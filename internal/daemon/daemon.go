package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dlorenc/multiclaude/internal/logging"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/prompts"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/claude"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

// Daemon represents the main daemon process
type Daemon struct {
	paths        *config.Paths
	state        *state.State
	tmux         *tmux.Client
	logger       *logging.Logger
	server       *socket.Server
	pidFile      *PIDFile
	claudeRunner *claude.Runner

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new daemon instance
func New(paths *config.Paths) (*Daemon, error) {
	// Ensure directories exist
	if err := paths.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Initialize logger
	logger, err := logging.NewFile(paths.DaemonLog)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Load or create state
	st, err := state.Load(paths.StateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tmuxClient := tmux.NewClient()
	d := &Daemon{
		paths:        paths,
		state:        st,
		tmux:         tmuxClient,
		logger:       logger,
		pidFile:      NewPIDFile(paths.DaemonPID),
		claudeRunner: claude.NewRunner(claude.WithTerminal(tmuxClient)),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Create socket server
	d.server = socket.NewServer(paths.DaemonSock, socket.HandlerFunc(d.handleRequest))

	return d, nil
}

// Start starts the daemon
func (d *Daemon) Start() error {
	d.logger.Info("Starting daemon")

	// Check and claim PID file
	if err := d.pidFile.CheckAndClaim(); err != nil {
		return err
	}

	// Start socket server
	if err := d.server.Start(); err != nil {
		return fmt.Errorf("failed to start socket server: %w", err)
	}

	d.logger.Info("Socket server started at %s", d.paths.DaemonSock)

	d.logger.Info("Daemon started successfully")

	// Restore agents for tracked repos BEFORE starting health checks
	// This prevents race conditions where health check cleans up agents being restored
	d.restoreTrackedRepos()

	// Start core loops after restore completes
	d.wg.Add(4)
	go d.healthCheckLoop()
	go d.messageRouterLoop()
	go d.wakeLoop()
	go d.serverLoop()

	return nil
}

// Wait waits for the daemon to shut down
func (d *Daemon) Wait() {
	d.wg.Wait()
}

// GetState returns the daemon's state (for testing)
func (d *Daemon) GetState() *state.State {
	return d.state
}

// GetPaths returns the daemon's paths (for testing)
func (d *Daemon) GetPaths() *config.Paths {
	return d.paths
}

// TriggerHealthCheck triggers an immediate health check (for testing)
func (d *Daemon) TriggerHealthCheck() {
	d.checkAgentHealth()
}

// TriggerMessageRouting triggers an immediate message routing (for testing)
func (d *Daemon) TriggerMessageRouting() {
	d.routeMessages()
}

// TriggerWake triggers an immediate wake cycle (for testing)
func (d *Daemon) TriggerWake() {
	d.wakeAgents()
}

// Stop stops the daemon
func (d *Daemon) Stop() error {
	d.logger.Info("Stopping daemon")

	// Cancel context to stop all loops
	d.cancel()

	// Wait for all goroutines to finish
	d.wg.Wait()

	// Stop socket server
	if err := d.server.Stop(); err != nil {
		d.logger.Error("Failed to stop socket server: %v", err)
	}

	// Save state
	if err := d.state.Save(); err != nil {
		d.logger.Error("Failed to save state: %v", err)
	}

	// Remove PID file
	if err := d.pidFile.Remove(); err != nil {
		d.logger.Error("Failed to remove PID file: %v", err)
	}

	d.logger.Info("Daemon stopped")
	return nil
}

// serverLoop handles socket connections
func (d *Daemon) serverLoop() {
	defer d.wg.Done()
	d.logger.Info("Starting server loop")

	// Run server in a goroutine so we can handle cancellation
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.server.Serve()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			d.logger.Error("Server error: %v", err)
		}
	case <-d.ctx.Done():
		d.logger.Info("Server loop stopped")
	}
}

// healthCheckLoop periodically checks agent health
func (d *Daemon) healthCheckLoop() {
	defer d.wg.Done()
	d.logger.Info("Starting health check loop")

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Run once immediately on startup
	d.checkAgentHealth()
	d.rotateLogsIfNeeded()
	d.cleanupMergedBranches()

	for {
		select {
		case <-ticker.C:
			d.checkAgentHealth()
			d.rotateLogsIfNeeded()
			d.cleanupMergedBranches()
		case <-d.ctx.Done():
			d.logger.Info("Health check loop stopped")
			return
		}
	}
}

// checkAgentHealth checks if agents are still alive
func (d *Daemon) checkAgentHealth() {
	d.logger.Debug("Checking agent health")

	deadAgents := make(map[string][]string) // repo -> []agent names

	// Get a snapshot of repos to avoid concurrent map access
	repos := d.state.GetAllRepos()
	for repoName, repo := range repos {
		// Check if tmux session exists
		hasSession, err := d.tmux.HasSession(repo.TmuxSession)
		if err != nil {
			d.logger.Error("Failed to check session %s: %v", repo.TmuxSession, err)
			continue
		}

		if !hasSession {
			d.logger.Warn("Tmux session %s not found for repo %s", repo.TmuxSession, repoName)
			// Mark all agents in this repo for cleanup
			for agentName := range repo.Agents {
				if deadAgents[repoName] == nil {
					deadAgents[repoName] = []string{}
				}
				deadAgents[repoName] = append(deadAgents[repoName], agentName)
			}
			continue
		}

		// Check each agent
		for agentName, agent := range repo.Agents {
			// Check if agent is marked as ready for cleanup
			if agent.ReadyForCleanup {
				d.logger.Info("Agent %s is ready for cleanup", agentName)
				if deadAgents[repoName] == nil {
					deadAgents[repoName] = []string{}
				}
				deadAgents[repoName] = append(deadAgents[repoName], agentName)
				continue
			}

			// Check if window exists
			hasWindow, err := d.tmux.HasWindow(repo.TmuxSession, agent.TmuxWindow)
			if err != nil {
				d.logger.Error("Failed to check window %s: %v", agent.TmuxWindow, err)
				continue
			}

			if !hasWindow {
				d.logger.Warn("Agent %s window not found, marking for cleanup", agentName)
				if deadAgents[repoName] == nil {
					deadAgents[repoName] = []string{}
				}
				deadAgents[repoName] = append(deadAgents[repoName], agentName)
				continue
			}

			// Check if process is alive (if we have a PID)
			if agent.PID > 0 {
				if !isProcessAlive(agent.PID) {
					d.logger.Warn("Agent %s process (PID %d) not running", agentName, agent.PID)

					// For managed agents (supervisor, merge-queue), attempt auto-restart
					if agent.Type == state.AgentTypeSupervisor || agent.Type == state.AgentTypeMergeQueue {
						d.logger.Info("Attempting to auto-restart managed agent %s", agentName)
						if err := d.restartManagedAgent(repoName, agentName, agent, repo); err != nil {
							d.logger.Error("Failed to restart agent %s: %v", agentName, err)
						} else {
							d.logger.Info("Successfully restarted agent %s", agentName)
						}
					}
					// For other agents (workers, workspaces), don't clean up - user might restart manually
				}
			}
		}
	}

	// Clean up dead agents
	if len(deadAgents) > 0 {
		d.cleanupDeadAgents(deadAgents)
	}

	// Clean up orphaned worktrees
	d.cleanupOrphanedWorktrees()
}

// messageRouterLoop watches for new messages and delivers them
func (d *Daemon) messageRouterLoop() {
	defer d.wg.Done()
	d.logger.Info("Starting message router loop")

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.routeMessages()
		case <-d.ctx.Done():
			d.logger.Info("Message router loop stopped")
			return
		}
	}
}

// routeMessages checks for pending messages and delivers them
func (d *Daemon) routeMessages() {
	d.logger.Debug("Routing messages")

	// Get messages manager
	msgMgr := d.getMessageManager()

	// Get a snapshot of repos to avoid concurrent map access
	repos := d.state.GetAllRepos()

	// Check each repository
	for repoName, repo := range repos {
		// Check each agent for messages
		for agentName, agent := range repo.Agents {
			// Skip workspace agent - it should only receive direct user input
			if agent.Type == state.AgentTypeWorkspace {
				continue
			}

			// Get unread messages (pending or delivered but not yet read)
			unreadMsgs, err := msgMgr.ListUnread(repoName, agentName)
			if err != nil {
				d.logger.Error("Failed to list messages for %s/%s: %v", repoName, agentName, err)
				continue
			}

			// Deliver each pending message
			for _, msg := range unreadMsgs {
				if msg.Status != messages.StatusPending {
					// Already delivered, skip
					continue
				}

				// Format message for delivery
				messageText := fmt.Sprintf("📨 Message from %s: %s", msg.From, msg.Body)

				// Send via tmux using atomic method to avoid race conditions
				// where Enter might be lost between separate exec calls (issue #63)
				if err := d.tmux.SendKeysLiteralWithEnter(repo.TmuxSession, agent.TmuxWindow, messageText); err != nil {
					d.logger.Error("Failed to deliver message %s to %s/%s: %v", msg.ID, repoName, agentName, err)
					continue
				}

				// Mark as delivered
				if err := msgMgr.UpdateStatus(repoName, agentName, msg.ID, messages.StatusDelivered); err != nil {
					d.logger.Error("Failed to update message %s status: %v", msg.ID, err)
					continue
				}

				d.logger.Info("Delivered message %s from %s to %s/%s", msg.ID, msg.From, repoName, agentName)
			}
		}
	}
}

// getMessageManager returns a message manager instance
func (d *Daemon) getMessageManager() *messages.Manager {
	return messages.NewManager(d.paths.MessagesDir)
}

// wakeLoop periodically wakes agents with status checks
func (d *Daemon) wakeLoop() {
	defer d.wg.Done()
	d.logger.Info("Starting wake loop")

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.wakeAgents()
		case <-d.ctx.Done():
			d.logger.Info("Wake loop stopped")
			return
		}
	}
}

// wakeAgents sends periodic nudges to agents
func (d *Daemon) wakeAgents() {
	d.logger.Debug("Waking agents")

	now := time.Now()

	// Get a snapshot of repos to avoid concurrent map access
	repos := d.state.GetAllRepos()
	for repoName, repo := range repos {
		for agentName, agent := range repo.Agents {
			// Skip workspace agent - it should only receive direct user input
			if agent.Type == state.AgentTypeWorkspace {
				continue
			}

			// Skip if nudged recently (within last 2 minutes)
			if !agent.LastNudge.IsZero() && now.Sub(agent.LastNudge) < 2*time.Minute {
				continue
			}

			// Send wake message based on agent type
			var message string
			switch agent.Type {
			case state.AgentTypeSupervisor:
				message = "Status check: Review worker progress and check merge queue."
			case state.AgentTypeMergeQueue:
				message = "Status check: Review open PRs and check CI status."
			case state.AgentTypeWorker:
				message = "Status check: Update on your progress?"
			case state.AgentTypeReview:
				message = "Status check: Update on your review progress?"
			}

			// Send message using atomic method to avoid race conditions (issue #63)
			if err := d.tmux.SendKeysLiteralWithEnter(repo.TmuxSession, agent.TmuxWindow, message); err != nil {
				d.logger.Error("Failed to send wake message to agent %s: %v", agentName, err)
				continue
			}

			// Update last nudge time
			agent.LastNudge = now
			if err := d.state.UpdateAgent(repoName, agentName, agent); err != nil {
				d.logger.Error("Failed to update agent %s last nudge: %v", agentName, err)
			}

			d.logger.Debug("Woke agent %s in repo %s", agentName, repoName)
		}
	}
}

// handleRequest handles incoming socket requests
func (d *Daemon) handleRequest(req socket.Request) socket.Response {
	d.logger.Debug("Handling request: %s", req.Command)

	switch req.Command {
	case "ping":
		return socket.Response{Success: true, Data: "pong"}

	case "status":
		return d.handleStatus(req)

	case "stop":
		go func() {
			time.Sleep(100 * time.Millisecond)
			d.Stop()
		}()
		return socket.Response{Success: true, Data: "Daemon stopping"}

	case "list_repos":
		return d.handleListRepos(req)

	case "add_repo":
		return d.handleAddRepo(req)

	case "remove_repo":
		return d.handleRemoveRepo(req)

	case "add_agent":
		return d.handleAddAgent(req)

	case "remove_agent":
		return d.handleRemoveAgent(req)

	case "list_agents":
		return d.handleListAgents(req)

	case "complete_agent":
		return d.handleCompleteAgent(req)

	case "trigger_cleanup":
		return d.handleTriggerCleanup(req)

	case "repair_state":
		return d.handleRepairState(req)

	case "get_repo_config":
		return d.handleGetRepoConfig(req)

	case "update_repo_config":
		return d.handleUpdateRepoConfig(req)

	case "set_current_repo":
		return d.handleSetCurrentRepo(req)

	case "get_current_repo":
		return d.handleGetCurrentRepo(req)

	case "clear_current_repo":
		return d.handleClearCurrentRepo(req)

	case "route_messages":
		go d.routeMessages()
		return socket.Response{Success: true, Data: "Message routing triggered"}

	case "task_history":
		return d.handleTaskHistory(req)

	default:
		return socket.Response{
			Success: false,
			Error:   fmt.Sprintf("unknown command: %q. Run 'multiclaude --help' for available commands", req.Command),
		}
	}
}

// handleStatus returns daemon status
func (d *Daemon) handleStatus(req socket.Request) socket.Response {
	repos := d.state.ListRepos()
	agentCount := 0
	for _, repo := range repos {
		agents, _ := d.state.ListAgents(repo)
		agentCount += len(agents)
	}

	return socket.Response{
		Success: true,
		Data: map[string]interface{}{
			"running":     true,
			"pid":         os.Getpid(),
			"repos":       len(repos),
			"agents":      agentCount,
			"socket_path": d.paths.DaemonSock,
		},
	}
}

// handleListRepos lists all repositories with detailed status
func (d *Daemon) handleListRepos(req socket.Request) socket.Response {
	repos := d.state.GetAllRepos()

	// Check if rich format is requested
	rich, _ := req.Args["rich"].(bool)
	if !rich {
		// Return simple list for backward compatibility
		repoNames := make([]string, 0, len(repos))
		for name := range repos {
			repoNames = append(repoNames, name)
		}
		return socket.Response{Success: true, Data: repoNames}
	}

	// Return detailed repo info
	repoDetails := make([]map[string]interface{}, 0, len(repos))
	for repoName, repo := range repos {
		// Count agents by type
		workerCount := 0
		totalAgents := len(repo.Agents)
		for _, agent := range repo.Agents {
			if agent.Type == state.AgentTypeWorker {
				workerCount++
			}
		}

		// Check session health
		sessionHealthy := false
		if hasSession, err := d.tmux.HasSession(repo.TmuxSession); err == nil {
			sessionHealthy = hasSession
		}

		repoDetails = append(repoDetails, map[string]interface{}{
			"name":            repoName,
			"github_url":      repo.GithubURL,
			"tmux_session":    repo.TmuxSession,
			"total_agents":    totalAgents,
			"worker_count":    workerCount,
			"session_healthy": sessionHealthy,
		})
	}

	return socket.Response{Success: true, Data: repoDetails}
}

// handleAddRepo adds a new repository
func (d *Daemon) handleAddRepo(req socket.Request) socket.Response {
	name, ok := req.Args["name"].(string)
	if !ok || name == "" {
		return socket.Response{Success: false, Error: "missing 'name': repository name is required (e.g., 'my-project')"}
	}

	githubURL, ok := req.Args["github_url"].(string)
	if !ok || githubURL == "" {
		return socket.Response{Success: false, Error: "missing 'github_url': GitHub repository URL is required (e.g., 'https://github.com/owner/repo')"}
	}

	tmuxSession, ok := req.Args["tmux_session"].(string)
	if !ok || tmuxSession == "" {
		return socket.Response{Success: false, Error: "missing 'tmux_session': tmux session name is required"}
	}

	// Parse merge queue configuration (optional, defaults to enabled with "all" tracking)
	mqConfig := state.DefaultMergeQueueConfig()
	if mqEnabled, ok := req.Args["mq_enabled"].(bool); ok {
		mqConfig.Enabled = mqEnabled
	}
	if mqTrackMode, ok := req.Args["mq_track_mode"].(string); ok {
		switch mqTrackMode {
		case "all":
			mqConfig.TrackMode = state.TrackModeAll
		case "author":
			mqConfig.TrackMode = state.TrackModeAuthor
		case "assigned":
			mqConfig.TrackMode = state.TrackModeAssigned
		}
	}

	repo := &state.Repository{
		GithubURL:        githubURL,
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: mqConfig,
	}

	if err := d.state.AddRepo(name, repo); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Added repository: %s (merge queue: enabled=%v, track=%s)", name, mqConfig.Enabled, mqConfig.TrackMode)
	return socket.Response{Success: true}
}

// handleRemoveRepo removes a repository from state
func (d *Daemon) handleRemoveRepo(req socket.Request) socket.Response {
	name, ok := req.Args["name"].(string)
	if !ok || name == "" {
		return socket.Response{Success: false, Error: "missing 'name': repository name is required"}
	}

	if err := d.state.RemoveRepo(name); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Removed repository: %s", name)
	return socket.Response{Success: true}
}

// handleAddAgent adds a new agent
func (d *Daemon) handleAddAgent(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok || repoName == "" {
		return socket.Response{Success: false, Error: "missing 'repo': repository name is required"}
	}

	agentName, ok := req.Args["agent"].(string)
	if !ok || agentName == "" {
		return socket.Response{Success: false, Error: "missing 'agent': agent name is required"}
	}

	agentTypeStr, ok := req.Args["type"].(string)
	if !ok || agentTypeStr == "" {
		return socket.Response{Success: false, Error: "missing 'type': agent type is required (supervisor, worker, merge-queue, or reviewer)"}
	}

	worktreePath, ok := req.Args["worktree_path"].(string)
	if !ok || worktreePath == "" {
		return socket.Response{Success: false, Error: "missing 'worktree_path': path to the agent's git worktree is required"}
	}

	tmuxWindow, ok := req.Args["tmux_window"].(string)
	if !ok || tmuxWindow == "" {
		return socket.Response{Success: false, Error: "missing 'tmux_window': tmux window name is required"}
	}

	// Get session ID from args or generate one
	sessionID, ok := req.Args["session_id"].(string)
	if !ok || sessionID == "" {
		sessionID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}

	// Get PID from args (optional)
	var pid int
	if pidFloat, ok := req.Args["pid"].(float64); ok {
		pid = int(pidFloat)
	} else if pidInt, ok := req.Args["pid"].(int); ok {
		pid = pidInt
	}

	agent := state.Agent{
		Type:         state.AgentType(agentTypeStr),
		WorktreePath: worktreePath,
		TmuxWindow:   tmuxWindow,
		SessionID:    sessionID,
		PID:          pid,
		CreatedAt:    time.Now(),
	}

	// Optional task field for workers
	if task, ok := req.Args["task"].(string); ok {
		agent.Task = task
	}

	if err := d.state.AddAgent(repoName, agentName, agent); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Added agent %s to repo %s", agentName, repoName)
	return socket.Response{Success: true}
}

// handleRemoveAgent removes an agent
func (d *Daemon) handleRemoveAgent(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok || repoName == "" {
		return socket.Response{Success: false, Error: "missing 'repo': repository name is required"}
	}

	agentName, ok := req.Args["agent"].(string)
	if !ok || agentName == "" {
		return socket.Response{Success: false, Error: "missing 'agent': agent name is required"}
	}

	if err := d.state.RemoveAgent(repoName, agentName); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Removed agent %s from repo %s", agentName, repoName)
	return socket.Response{Success: true}
}

// handleListAgents lists agents for a repository
func (d *Daemon) handleListAgents(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok || repoName == "" {
		return socket.Response{Success: false, Error: "missing 'repo': repository name is required"}
	}

	agents, err := d.state.ListAgents(repoName)
	if err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	// Check if rich format is requested
	rich, _ := req.Args["rich"].(bool)

	// Get repository to check session
	repo, repoExists := d.state.GetRepo(repoName)

	// Get full agent details
	agentDetails := make([]map[string]interface{}, 0, len(agents))
	for _, agentName := range agents {
		agent, exists := d.state.GetAgent(repoName, agentName)
		if !exists {
			continue
		}

		detail := map[string]interface{}{
			"name":          agentName,
			"type":          agent.Type,
			"worktree_path": agent.WorktreePath,
			"tmux_window":   agent.TmuxWindow,
			"task":          agent.Task,
			"created_at":    agent.CreatedAt,
		}

		// Add rich status information if requested
		if rich {
			// Determine agent status
			status := "unknown"
			if agent.ReadyForCleanup {
				status = "completed"
			} else if repoExists {
				// Check if window exists (means agent is running)
				hasWindow, err := d.tmux.HasWindow(repo.TmuxSession, agent.TmuxWindow)
				if err == nil && hasWindow {
					status = "running"
				} else {
					status = "stopped"
				}
			}
			detail["status"] = status

			// Get current branch from worktree
			branch := ""
			if agent.WorktreePath != "" {
				if b, err := worktree.GetCurrentBranch(agent.WorktreePath); err == nil {
					branch = b
				}
			}
			detail["branch"] = branch

			// Get message counts
			msgManager := messages.NewManager(d.paths.MessagesDir)
			allMsgs, _ := msgManager.List(repoName, agentName)
			pendingCount := 0
			for _, msg := range allMsgs {
				if msg.Status == messages.StatusPending || msg.Status == messages.StatusDelivered {
					pendingCount++
				}
			}
			detail["messages_total"] = len(allMsgs)
			detail["messages_pending"] = pendingCount
		}

		agentDetails = append(agentDetails, detail)
	}

	return socket.Response{Success: true, Data: agentDetails}
}

// handleCompleteAgent marks an agent as ready for cleanup
func (d *Daemon) handleCompleteAgent(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok || repoName == "" {
		return socket.Response{Success: false, Error: "missing 'repo': repository name is required"}
	}

	agentName, ok := req.Args["agent"].(string)
	if !ok || agentName == "" {
		return socket.Response{Success: false, Error: "missing 'agent': agent name is required"}
	}

	agent, exists := d.state.GetAgent(repoName, agentName)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("agent '%s' not found in repository '%s' - check available agents with: multiclaude work list --repo %s", agentName, repoName, repoName)}
	}

	// Mark as ready for cleanup
	agent.ReadyForCleanup = true
	if err := d.state.UpdateAgent(repoName, agentName, agent); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Agent %s/%s marked as ready for cleanup", repoName, agentName)

	// Notify supervisor and merge-queue that worker or review agent completed
	if agent.Type == state.AgentTypeWorker || agent.Type == state.AgentTypeReview {
		msgMgr := d.getMessageManager()
		task := agent.Task
		if task == "" {
			task = "unknown task"
		}

		if agent.Type == state.AgentTypeWorker {
			// Notify supervisor
			supervisorMessage := fmt.Sprintf("Worker '%s' has completed its task: %s", agentName, task)
			if _, err := msgMgr.Send(repoName, agentName, "supervisor", supervisorMessage); err != nil {
				d.logger.Error("Failed to send completion message to supervisor: %v", err)
			} else {
				d.logger.Info("Sent completion notification to supervisor for worker %s", agentName)
			}

			// Notify merge-queue so it can process any new PRs immediately
			mergeQueueMessage := fmt.Sprintf("Worker '%s' has completed and may have created a PR. Task: %s. Please check for new PRs to process.", agentName, task)
			if _, err := msgMgr.Send(repoName, agentName, "merge-queue", mergeQueueMessage); err != nil {
				d.logger.Error("Failed to send completion message to merge-queue: %v", err)
			} else {
				d.logger.Info("Sent completion notification to merge-queue for worker %s", agentName)
			}
		} else if agent.Type == state.AgentTypeReview {
			// Review agent completed - notify merge-queue to process the review results
			mergeQueueMessage := fmt.Sprintf("Review agent '%s' has completed its review. Task: %s. Please check the review summary and decide on next steps.", agentName, task)
			if _, err := msgMgr.Send(repoName, agentName, "merge-queue", mergeQueueMessage); err != nil {
				d.logger.Error("Failed to send completion message to merge-queue: %v", err)
			} else {
				d.logger.Info("Sent completion notification to merge-queue for review agent %s", agentName)
			}
		}

		// Trigger immediate message delivery
		go d.routeMessages()
	}

	// Trigger immediate cleanup check
	go d.checkAgentHealth()

	return socket.Response{Success: true}
}

// handleTriggerCleanup manually triggers cleanup operations
func (d *Daemon) handleTriggerCleanup(req socket.Request) socket.Response {
	d.logger.Info("Manual cleanup triggered")

	// Run health check to find dead agents
	d.checkAgentHealth()

	return socket.Response{
		Success: true,
		Data:    "Cleanup triggered",
	}
}

// handleRepairState repairs state inconsistencies
func (d *Daemon) handleRepairState(req socket.Request) socket.Response {
	d.logger.Info("State repair triggered")

	agentsRemoved := 0
	issuesFixed := 0

	// Get a snapshot of repos to avoid concurrent map access
	repos := d.state.GetAllRepos()

	// Check all agents and verify resources exist
	for repoName, repo := range repos {
		// Check tmux session
		hasSession, err := d.tmux.HasSession(repo.TmuxSession)
		if err != nil {
			d.logger.Error("Failed to check session %s: %v", repo.TmuxSession, err)
			continue
		}

		if !hasSession {
			d.logger.Warn("Tmux session %s not found, removing all agents for repo %s", repo.TmuxSession, repoName)
			// Remove all agents for this repo
			for agentName := range repo.Agents {
				if err := d.state.RemoveAgent(repoName, agentName); err == nil {
					agentsRemoved++
				}
			}
			issuesFixed++
			continue
		}

		// Check each agent's resources
		for agentName, agent := range repo.Agents {
			hasWindow, _ := d.tmux.HasWindow(repo.TmuxSession, agent.TmuxWindow)
			if !hasWindow {
				d.logger.Info("Removing agent %s (window not found)", agentName)
				if err := d.state.RemoveAgent(repoName, agentName); err == nil {
					agentsRemoved++
					issuesFixed++
				}
				continue
			}

			// Check if worktree exists (for workers and review agents)
			if (agent.Type == state.AgentTypeWorker || agent.Type == state.AgentTypeReview) && agent.WorktreePath != "" {
				if _, err := os.Stat(agent.WorktreePath); os.IsNotExist(err) {
					d.logger.Warn("Worktree missing for agent %s, but window exists - keeping agent", agentName)
					// Don't remove - user might have manually deleted worktree
				}
			}
		}
	}

	// Clean up orphaned worktrees
	d.cleanupOrphanedWorktrees()

	// Clean up orphaned message directories
	msgMgr := d.getMessageManager()
	repoNames := d.state.ListRepos()
	for _, repoName := range repoNames {
		validAgents, _ := d.state.ListAgents(repoName)
		if count, err := msgMgr.CleanupOrphaned(repoName, validAgents); err == nil && count > 0 {
			issuesFixed += count
		}
	}

	d.logger.Info("State repair completed: %d agents removed, %d issues fixed", agentsRemoved, issuesFixed)

	return socket.Response{
		Success: true,
		Data: map[string]interface{}{
			"agents_removed": agentsRemoved,
			"issues_fixed":   issuesFixed,
		},
	}
}

// handleGetRepoConfig returns the configuration for a repository
func (d *Daemon) handleGetRepoConfig(req socket.Request) socket.Response {
	name, ok := req.Args["name"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'name' argument"}
	}

	repo, exists := d.state.GetRepo(name)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("repository %q not found", name)}
	}

	// Get merge queue config (use default if not set for backward compatibility)
	mqConfig := repo.MergeQueueConfig
	if mqConfig.TrackMode == "" {
		mqConfig = state.DefaultMergeQueueConfig()
	}

	return socket.Response{
		Success: true,
		Data: map[string]interface{}{
			"mq_enabled":    mqConfig.Enabled,
			"mq_track_mode": string(mqConfig.TrackMode),
		},
	}
}

// handleUpdateRepoConfig updates the configuration for a repository
func (d *Daemon) handleUpdateRepoConfig(req socket.Request) socket.Response {
	name, ok := req.Args["name"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'name' argument"}
	}

	// Get current merge queue config
	currentMQConfig, err := d.state.GetMergeQueueConfig(name)
	if err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	// Update merge queue config with provided values
	mqUpdated := false
	if mqEnabled, ok := req.Args["mq_enabled"].(bool); ok {
		currentMQConfig.Enabled = mqEnabled
		mqUpdated = true
	}
	if mqTrackMode, ok := req.Args["mq_track_mode"].(string); ok {
		switch mqTrackMode {
		case "all":
			currentMQConfig.TrackMode = state.TrackModeAll
		case "author":
			currentMQConfig.TrackMode = state.TrackModeAuthor
		case "assigned":
			currentMQConfig.TrackMode = state.TrackModeAssigned
		default:
			return socket.Response{Success: false, Error: fmt.Sprintf("invalid track mode: %s", mqTrackMode)}
		}
		mqUpdated = true
	}

	if mqUpdated {
		if err := d.state.UpdateMergeQueueConfig(name, currentMQConfig); err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		d.logger.Info("Updated merge queue config for repo %s: enabled=%v, track=%s", name, currentMQConfig.Enabled, currentMQConfig.TrackMode)
	}

	return socket.Response{Success: true}
}

// handleSetCurrentRepo sets the current/default repository
func (d *Daemon) handleSetCurrentRepo(req socket.Request) socket.Response {
	name, ok := req.Args["name"].(string)
	if !ok || name == "" {
		return socket.Response{Success: false, Error: "missing 'name': repository name is required"}
	}

	if err := d.state.SetCurrentRepo(name); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Set current repository to: %s", name)
	return socket.Response{Success: true, Data: name}
}

// handleGetCurrentRepo returns the current/default repository
func (d *Daemon) handleGetCurrentRepo(req socket.Request) socket.Response {
	currentRepo := d.state.GetCurrentRepo()
	if currentRepo == "" {
		return socket.Response{Success: false, Error: "no current repository set"}
	}
	return socket.Response{Success: true, Data: currentRepo}
}

// handleClearCurrentRepo clears the current/default repository
func (d *Daemon) handleClearCurrentRepo(req socket.Request) socket.Response {
	if err := d.state.ClearCurrentRepo(); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Cleared current repository")
	return socket.Response{Success: true}
}

// cleanupDeadAgents removes dead agents from state
func (d *Daemon) cleanupDeadAgents(deadAgents map[string][]string) {
	for repoName, agentNames := range deadAgents {
		for _, agentName := range agentNames {
			d.logger.Info("Cleaning up dead agent %s/%s", repoName, agentName)

			agent, exists := d.state.GetAgent(repoName, agentName)
			if !exists {
				continue
			}

			// Get repo info for tmux session
			repo, exists := d.state.GetRepo(repoName)
			if !exists {
				d.logger.Error("Failed to get repo %s for cleanup", repoName)
				continue
			}

			// Record task history for workers before cleanup
			if agent.Type == state.AgentTypeWorker {
				d.recordTaskHistory(repoName, agentName, agent)
			}

			// Kill tmux window
			if err := d.tmux.KillWindow(repo.TmuxSession, agent.TmuxWindow); err != nil {
				d.logger.Warn("Failed to kill tmux window %s: %v", agent.TmuxWindow, err)
			} else {
				d.logger.Info("Killed tmux window for agent %s: %s", agentName, agent.TmuxWindow)
			}

			// Remove from state
			if err := d.state.RemoveAgent(repoName, agentName); err != nil {
				d.logger.Error("Failed to remove agent %s/%s from state: %v", repoName, agentName, err)
			}

			// Clean up worktree if it exists (workers and review agents have worktrees)
			if agent.WorktreePath != "" && (agent.Type == state.AgentTypeWorker || agent.Type == state.AgentTypeReview) {
				repoPath := d.paths.RepoDir(repoName)
				wt := worktree.NewManager(repoPath)
				if err := wt.Remove(agent.WorktreePath, true); err != nil {
					d.logger.Warn("Failed to remove worktree %s: %v", agent.WorktreePath, err)
				} else {
					d.logger.Info("Removed worktree for dead agent: %s", agent.WorktreePath)
				}
			}

			// Clean up message directory
			msgMgr := d.getMessageManager()
			validAgents, _ := d.state.ListAgents(repoName)
			if _, err := msgMgr.CleanupOrphaned(repoName, validAgents); err != nil {
				d.logger.Warn("Failed to cleanup orphaned messages for %s: %v", repoName, err)
			}
		}
	}
}

// recordTaskHistory saves a worker's task to the history before cleanup
func (d *Daemon) recordTaskHistory(repoName, agentName string, agent state.Agent) {
	// Get the branch name from the worktree if it exists
	branch := ""
	if agent.WorktreePath != "" {
		if b, err := worktree.GetCurrentBranch(agent.WorktreePath); err == nil {
			branch = b
		} else {
			// Fallback: construct expected branch name
			branch = "work/" + agentName
		}
	}

	entry := state.TaskHistoryEntry{
		Name:        agentName,
		Task:        agent.Task,
		Branch:      branch,
		Status:      state.TaskStatusUnknown, // Will be updated when displaying
		CreatedAt:   agent.CreatedAt,
		CompletedAt: time.Now(),
	}

	if err := d.state.AddTaskHistory(repoName, entry); err != nil {
		d.logger.Warn("Failed to record task history for %s: %v", agentName, err)
	} else {
		d.logger.Info("Recorded task history for %s (branch: %s)", agentName, branch)
	}
}

// handleTaskHistory returns the task history for a repository
func (d *Daemon) handleTaskHistory(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok || repoName == "" {
		return socket.Response{Success: false, Error: "missing 'repo': repository name is required"}
	}

	// Get optional limit
	limit := 10 // default
	if l, ok := req.Args["limit"].(float64); ok {
		limit = int(l)
	}

	history, err := d.state.GetTaskHistory(repoName, limit)
	if err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	// Convert to interface slice for JSON serialization
	result := make([]map[string]interface{}, len(history))
	for i, entry := range history {
		result[i] = map[string]interface{}{
			"name":         entry.Name,
			"task":         entry.Task,
			"branch":       entry.Branch,
			"pr_url":       entry.PRURL,
			"pr_number":    entry.PRNumber,
			"status":       string(entry.Status),
			"created_at":   entry.CreatedAt,
			"completed_at": entry.CompletedAt,
		}
	}

	return socket.Response{Success: true, Data: result}
}

// cleanupOrphanedWorktrees removes worktree directories without git tracking
func (d *Daemon) cleanupOrphanedWorktrees() {
	repoNames := d.state.ListRepos()
	for _, repoName := range repoNames {
		repoPath := d.paths.RepoDir(repoName)
		wtRootDir := d.paths.WorktreeDir(repoName)

		// Check if worktree directory exists
		if _, err := os.Stat(wtRootDir); os.IsNotExist(err) {
			continue
		}

		wt := worktree.NewManager(repoPath)
		removed, err := worktree.CleanupOrphaned(wtRootDir, wt)
		if err != nil {
			d.logger.Error("Failed to cleanup orphaned worktrees for %s: %v", repoName, err)
			continue
		}

		if len(removed) > 0 {
			d.logger.Info("Cleaned up %d orphaned worktree(s) for %s", len(removed), repoName)
			for _, path := range removed {
				d.logger.Debug("Removed orphaned worktree: %s", path)
			}
		}

		// Also prune git worktree references
		if err := wt.Prune(); err != nil {
			d.logger.Warn("Failed to prune worktrees for %s: %v", repoName, err)
		}
	}
}

// cleanupMergedBranches cleans up branches that have been merged upstream
func (d *Daemon) cleanupMergedBranches() {
	d.logger.Debug("Checking for merged branches to cleanup")

	repoNames := d.state.ListRepos()
	for _, repoName := range repoNames {
		repoPath := d.paths.RepoDir(repoName)

		// Check if repo path exists
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			continue
		}

		wt := worktree.NewManager(repoPath)

		// Clean up merged branches with common multiclaude prefixes
		for _, prefix := range []string{"multiclaude/", "work/"} {
			deleted, err := wt.CleanupMergedBranches(prefix, true)
			if err != nil {
				d.logger.Debug("Failed to cleanup merged branches with prefix %s for %s: %v", prefix, repoName, err)
				continue
			}

			if len(deleted) > 0 {
				d.logger.Info("Cleaned up %d merged branch(es) for %s", len(deleted), repoName)
				for _, branch := range deleted {
					d.logger.Info("Deleted merged branch: %s", branch)
				}
			}
		}
	}
}

// restoreTrackedRepos restores agents for tracked repos that are missing their tmux sessions
func (d *Daemon) restoreTrackedRepos() {
	d.logger.Info("Checking tracked repos for restoration")

	repos := d.state.GetAllRepos()
	for repoName, repo := range repos {
		// Check if tmux session exists
		hasSession, err := d.tmux.HasSession(repo.TmuxSession)
		if err != nil {
			d.logger.Error("Failed to check session %s: %v", repo.TmuxSession, err)
			continue
		}

		if hasSession {
			d.logger.Debug("Tmux session %s exists for repo %s", repo.TmuxSession, repoName)
			continue
		}

		// Session doesn't exist - restore it
		d.logger.Info("Restoring agents for repo %s (tmux session %s was missing)", repoName, repo.TmuxSession)
		if err := d.restoreRepoAgents(repoName, repo); err != nil {
			d.logger.Error("Failed to restore agents for repo %s: %v", repoName, err)
		}
	}
}

// restoreRepoAgents restores the tmux session and agents for a tracked repo
func (d *Daemon) restoreRepoAgents(repoName string, repo *state.Repository) error {
	repoPath := d.paths.RepoDir(repoName)

	// Verify the repo still exists on disk
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository path does not exist: %s", repoPath)
	}

	// Clear any stale agents from state (their tmux session is gone)
	for agentName := range repo.Agents {
		d.logger.Debug("Removing stale agent %s/%s from state", repoName, agentName)
		if err := d.state.RemoveAgent(repoName, agentName); err != nil {
			d.logger.Warn("Failed to remove stale agent %s/%s: %v", repoName, agentName, err)
		}
	}

	// Create tmux session with supervisor window
	d.logger.Info("Creating tmux session %s for repo %s", repo.TmuxSession, repoName)
	cmd := exec.Command("tmux", "new-session", "-d", "-s", repo.TmuxSession, "-n", "supervisor", "-c", repoPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Get merge queue config (use default if not set for backward compatibility)
	mqConfig := repo.MergeQueueConfig
	if mqConfig.TrackMode == "" {
		mqConfig = state.DefaultMergeQueueConfig()
	}

	// Create merge-queue window only if enabled
	if mqConfig.Enabled {
		cmd = exec.Command("tmux", "new-window", "-d", "-t", repo.TmuxSession, "-n", "merge-queue", "-c", repoPath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create merge-queue window: %w", err)
		}
	}

	// Start supervisor agent
	if err := d.startAgent(repoName, repo, "supervisor", prompts.TypeSupervisor, repoPath); err != nil {
		d.logger.Error("Failed to start supervisor for %s: %v", repoName, err)
	}

	// Start merge-queue agent only if enabled
	if mqConfig.Enabled {
		if err := d.startMergeQueueAgent(repoName, repo, repoPath, mqConfig); err != nil {
			d.logger.Error("Failed to start merge-queue for %s: %v", repoName, err)
		}
	} else {
		d.logger.Info("Merge queue is disabled for repo %s, skipping merge-queue agent", repoName)
	}

	// Create and restore workspace
	workspacePath := d.paths.AgentWorktree(repoName, "workspace")
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		// Workspace worktree doesn't exist, create it
		d.logger.Info("Creating workspace worktree for %s", repoName)
		wt := worktree.NewManager(repoPath)

		// Check for and migrate legacy "workspace" branch to "workspace/default"
		migrated, migrateErr := wt.MigrateLegacyWorkspaceBranch()
		if migrateErr != nil {
			d.logger.Warn("Failed to migrate legacy workspace branch for %s: %v", repoName, migrateErr)
		} else if migrated {
			d.logger.Info("Migrated legacy 'workspace' branch to 'workspace/default' for %s", repoName)
		}

		// Try to create with existing branch using new naming convention first
		if err := wt.Create(workspacePath, "workspace/default"); err != nil {
			// Branch doesn't exist, create with new branch
			if err := wt.CreateNewBranch(workspacePath, "workspace/default", "HEAD"); err != nil {
				d.logger.Error("Failed to create workspace worktree for %s: %v", repoName, err)
			}
		}
	}

	// Now start the workspace agent if worktree exists
	if _, err := os.Stat(workspacePath); err == nil {
		cmd = exec.Command("tmux", "new-window", "-d", "-t", repo.TmuxSession, "-n", "workspace", "-c", workspacePath)
		if err := cmd.Run(); err != nil {
			d.logger.Error("Failed to create workspace window: %v", err)
		} else {
			if err := d.startAgent(repoName, repo, "workspace", prompts.TypeWorkspace, workspacePath); err != nil {
				d.logger.Error("Failed to start workspace for %s: %v", repoName, err)
			}
		}
	}

	return nil
}

// getClaudeBinaryPath resolves the claude CLI binary path
func (d *Daemon) getClaudeBinaryPath() (string, error) {
	binaryPath, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude binary not found in PATH: %w", err)
	}
	return binaryPath, nil
}

// startAgent starts a Claude agent in a tmux window and registers it with state
func (d *Daemon) startAgent(repoName string, repo *state.Repository, agentName string, agentType prompts.AgentType, workDir string) error {
	// Resolve claude binary path
	binaryPath, err := d.getClaudeBinaryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve claude binary: %w", err)
	}

	// Generate session ID
	sessionID, err := claude.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Write prompt file
	promptFile, err := d.writePromptFile(repoName, agentType, agentName)
	if err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Copy hooks config if needed
	repoPath := d.paths.RepoDir(repoName)
	if err := d.copyHooksConfig(repoPath, workDir); err != nil {
		d.logger.Warn("Failed to copy hooks config: %v", err)
	}

	// Build CLI command
	claudeCmd := fmt.Sprintf("%s --session-id %s --dangerously-skip-permissions --append-system-prompt-file %s",
		binaryPath, sessionID, promptFile)

	// Send command to tmux window
	target := fmt.Sprintf("%s:%s", repo.TmuxSession, agentName)
	cmd := exec.Command("tmux", "send-keys", "-t", target, claudeCmd, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Claude in tmux: %w", err)
	}

	// Wait a moment for Claude to start
	time.Sleep(500 * time.Millisecond)

	// Get PID
	pid, err := d.tmux.GetPanePID(repo.TmuxSession, agentName)
	if err != nil {
		return fmt.Errorf("failed to get Claude PID: %w", err)
	}

	// Register agent with state
	agent := state.Agent{
		Type:         state.AgentType(agentType),
		WorktreePath: workDir,
		TmuxWindow:   agentName,
		SessionID:    sessionID,
		PID:          pid,
		CreatedAt:    time.Now(),
	}

	if err := d.state.AddAgent(repoName, agentName, agent); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	d.logger.Info("Started and registered agent %s/%s", repoName, agentName)
	return nil
}

// startMergeQueueAgent starts a merge-queue agent with tracking mode configuration
func (d *Daemon) startMergeQueueAgent(repoName string, repo *state.Repository, workDir string, mqConfig state.MergeQueueConfig) error {
	// Resolve claude binary path
	binaryPath, err := d.getClaudeBinaryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve claude binary: %w", err)
	}

	// Generate session ID
	sessionID, err := claude.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Write prompt file with tracking mode configuration
	promptFile, err := d.writeMergeQueuePromptFile(repoName, "merge-queue", mqConfig)
	if err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Copy hooks config if needed
	repoPath := d.paths.RepoDir(repoName)
	if err := d.copyHooksConfig(repoPath, workDir); err != nil {
		d.logger.Warn("Failed to copy hooks config: %v", err)
	}

	// Build CLI command
	claudeCmd := fmt.Sprintf("%s --session-id %s --dangerously-skip-permissions --append-system-prompt-file %s",
		binaryPath, sessionID, promptFile)

	// Send command to tmux window
	target := fmt.Sprintf("%s:merge-queue", repo.TmuxSession)
	cmd := exec.Command("tmux", "send-keys", "-t", target, claudeCmd, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Claude in tmux: %w", err)
	}

	// Wait a moment for Claude to start
	time.Sleep(500 * time.Millisecond)

	// Get PID
	pid, err := d.tmux.GetPanePID(repo.TmuxSession, "merge-queue")
	if err != nil {
		return fmt.Errorf("failed to get Claude PID: %w", err)
	}

	// Register agent with state
	agent := state.Agent{
		Type:         state.AgentTypeMergeQueue,
		WorktreePath: workDir,
		TmuxWindow:   "merge-queue",
		SessionID:    sessionID,
		PID:          pid,
		CreatedAt:    time.Now(),
	}

	if err := d.state.AddAgent(repoName, "merge-queue", agent); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	d.logger.Info("Started and registered merge-queue agent %s/merge-queue (track mode: %s)", repoName, mqConfig.TrackMode)
	return nil
}

// writeMergeQueuePromptFile writes a merge-queue prompt file with tracking mode configuration
func (d *Daemon) writeMergeQueuePromptFile(repoName string, agentName string, mqConfig state.MergeQueueConfig) (string, error) {
	repoPath := d.paths.RepoDir(repoName)

	// Get the base prompt (without CLI docs since we don't have them in daemon context)
	promptText, err := prompts.GetPrompt(repoPath, prompts.TypeMergeQueue, "")
	if err != nil {
		return "", fmt.Errorf("failed to get prompt: %w", err)
	}

	// Add tracking mode configuration to the prompt
	trackingConfig := d.generateTrackingModePrompt(mqConfig.TrackMode)
	promptText = trackingConfig + "\n\n" + promptText

	// Create prompt file in prompts directory
	promptDir := filepath.Join(d.paths.Root, "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create prompt directory: %w", err)
	}

	promptPath := filepath.Join(promptDir, fmt.Sprintf("%s.md", agentName))
	if err := os.WriteFile(promptPath, []byte(promptText), 0644); err != nil {
		return "", fmt.Errorf("failed to write prompt file: %w", err)
	}

	return promptPath, nil
}

// generateTrackingModePrompt generates prompt text explaining which PRs to track based on tracking mode
func (d *Daemon) generateTrackingModePrompt(trackMode state.TrackMode) string {
	switch trackMode {
	case state.TrackModeAuthor:
		return `## PR Tracking Mode: Author Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are the author.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --author @me --label multiclaude
` + "```" + `

Do NOT process or attempt to merge PRs authored by others. Focus only on PRs created by multiclaude workers.`

	case state.TrackModeAssigned:
		return `## PR Tracking Mode: Assigned Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are assigned.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --assignee @me --label multiclaude
` + "```" + `

Do NOT process or attempt to merge PRs unless they are assigned to you. Focus only on PRs explicitly assigned to multiclaude.`

	default: // TrackModeAll
		return `## PR Tracking Mode: All PRs

This repository is configured to track all PRs with the multiclaude label.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --label multiclaude
` + "```" + `

Monitor and process all multiclaude-labeled PRs regardless of author or assignee.`
	}
}

// restartManagedAgent restarts a managed agent (supervisor, merge-queue) that has exited.
// It uses --resume to continue the existing session if history exists.
func (d *Daemon) restartManagedAgent(repoName, agentName string, agent state.Agent, repo *state.Repository) error {
	// Check if the session has history
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeProjectsDir := filepath.Join(home, ".claude", "projects")
	encodedPath := strings.ReplaceAll(agent.WorktreePath, "/", "-")
	sessionFile := filepath.Join(claudeProjectsDir, encodedPath, agent.SessionID+".jsonl")

	hasHistory := false
	if info, err := os.Stat(sessionFile); err == nil && info.Size() > 0 {
		hasHistory = true
	}

	// Get the existing prompt file path
	promptFile := filepath.Join(d.paths.Root, "prompts", agentName+".md")
	if _, err := os.Stat(promptFile); os.IsNotExist(err) {
		// Regenerate the prompt file if it doesn't exist
		promptFile, err = d.writePromptFile(repoName, prompts.AgentType(agent.Type), agentName)
		if err != nil {
			return fmt.Errorf("failed to regenerate prompt file: %w", err)
		}
	}

	// Restart Claude using the runner
	result, err := d.claudeRunner.Start(repo.TmuxSession, agentName, claude.Config{
		SessionID:        agent.SessionID,
		Resume:           hasHistory,
		SystemPromptFile: promptFile,
	})
	if err != nil {
		return fmt.Errorf("failed to restart Claude: %w", err)
	}

	// Update the agent's PID in state
	if err := d.state.UpdateAgentPID(repoName, agentName, result.PID); err != nil {
		d.logger.Warn("Failed to update agent PID: %v", err)
	}

	d.logger.Info("Restarted agent %s with PID %d (resumed=%v)", agentName, result.PID, hasHistory)
	return nil
}

// writePromptFile writes the agent prompt to a file and returns the path
func (d *Daemon) writePromptFile(repoName string, agentType prompts.AgentType, agentName string) (string, error) {
	repoPath := d.paths.RepoDir(repoName)

	// Get the prompt (without CLI docs since we don't have them in daemon context)
	promptText, err := prompts.GetPrompt(repoPath, agentType, "")
	if err != nil {
		return "", fmt.Errorf("failed to get prompt: %w", err)
	}

	// Create prompt file in prompts directory
	promptDir := filepath.Join(d.paths.Root, "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create prompt directory: %w", err)
	}

	promptPath := filepath.Join(promptDir, fmt.Sprintf("%s.md", agentName))
	if err := os.WriteFile(promptPath, []byte(promptText), 0644); err != nil {
		return "", fmt.Errorf("failed to write prompt file: %w", err)
	}

	return promptPath, nil
}

// copyHooksConfig copies hooks configuration from repo to workdir if it exists
func (d *Daemon) copyHooksConfig(repoPath, workDir string) error {
	hooksPath := filepath.Join(repoPath, ".multiclaude", "hooks.json")

	// Check if hooks.json exists
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		return nil // No hooks config
	} else if err != nil {
		return fmt.Errorf("failed to check hooks config: %w", err)
	}

	// Create .claude directory in workdir
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Copy hooks.json to .claude/settings.json
	hooksData, err := os.ReadFile(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to read hooks config: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, hooksData, 0644); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}

// isProcessAlive checks if a process is running
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists (doesn't actually signal, just checks)
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// Run runs the daemon in the foreground
func Run() error {
	paths, err := config.DefaultPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	d, err := New(paths)
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	if err := d.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Wait for shutdown
	d.Wait()

	return nil
}

// RunDetached starts the daemon in detached mode
func RunDetached() error {
	paths, err := config.DefaultPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	// Check if already running
	pidFile := NewPIDFile(paths.DaemonPID)
	if running, pid, _ := pidFile.IsRunning(); running {
		return fmt.Errorf("daemon already running (PID: %d)", pid)
	}

	// Ensure config directory exists
	if err := os.MkdirAll(paths.Root, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create log file for output
	logFile, err := os.OpenFile(paths.DaemonLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Prepare daemon command
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Fork and daemonize
	attr := &os.ProcAttr{
		Dir: filepath.Dir(paths.Root),
		Env: os.Environ(),
		Files: []*os.File{
			nil,     // stdin
			logFile, // stdout
			logFile, // stderr
		},
		Sys: nil,
	}

	// Start daemon process
	process, err := os.StartProcess(executable, []string{executable, "daemon", "_run"}, attr)
	if err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	// Detach from parent
	if err := process.Release(); err != nil {
		log.Printf("Warning: failed to release process: %v", err)
	}

	fmt.Printf("Daemon started (PID will be written to %s)\n", paths.DaemonPID)
	return nil
}

// MaxLogFileSize is the threshold for log rotation (10MB)
const MaxLogFileSize = 10 * 1024 * 1024

// rotateLogsIfNeeded checks log files and rotates any that exceed MaxLogFileSize
func (d *Daemon) rotateLogsIfNeeded() {
	d.logger.Debug("Checking for log rotation")

	err := filepath.Walk(d.paths.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}
		if !isLogFile(path) {
			return nil
		}

		if info.Size() > MaxLogFileSize {
			if err := d.rotateLog(path); err != nil {
				d.logger.Error("Failed to rotate log %s: %v", path, err)
			} else {
				d.logger.Info("Rotated log %s (was %d bytes)", path, info.Size())
			}
		}
		return nil
	})

	if err != nil {
		d.logger.Error("Failed to walk output directory for log rotation: %v", err)
	}
}

// rotateLog rotates a single log file by renaming it with a timestamp suffix
func (d *Daemon) rotateLog(logPath string) error {
	// Generate rotated filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := logPath + "." + timestamp

	// Rename the current log file
	if err := os.Rename(logPath, rotatedPath); err != nil {
		return fmt.Errorf("failed to rename log: %w", err)
	}

	// The tmux pipe-pane will create a new file automatically when it next writes
	// No need to recreate the file or restart the pipe

	return nil
}

// isLogFile checks if a file is a log file
func isLogFile(path string) bool {
	base := filepath.Base(path)
	// Only match .log files, not already-rotated files (which have timestamps)
	return len(base) > 4 && base[len(base)-4:] == ".log"
}
