package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dlorenc/multiclaude/internal/logging"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/tmux"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/config"
)

// Daemon represents the main daemon process
type Daemon struct {
	paths   *config.Paths
	state   *state.State
	tmux    *tmux.Client
	logger  *logging.Logger
	server  *socket.Server
	pidFile *PIDFile

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

	d := &Daemon{
		paths:   paths,
		state:   st,
		tmux:    tmux.NewClient(),
		logger:  logger,
		pidFile: NewPIDFile(paths.DaemonPID),
		ctx:     ctx,
		cancel:  cancel,
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

	// Start core loops
	d.wg.Add(4)
	go d.healthCheckLoop()
	go d.messageRouterLoop()
	go d.wakeLoop()
	go d.serverLoop()

	d.logger.Info("Daemon started successfully")

	return nil
}

// Wait waits for the daemon to shut down
func (d *Daemon) Wait() {
	d.wg.Wait()
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

	for {
		select {
		case <-ticker.C:
			d.checkAgentHealth()
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
					// Don't clean up just because process died - window might still be active
					// User might have restarted Claude manually
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

				// Send via tmux using literal mode to avoid escaping issues
				// First send the text literally, then send Enter
				if err := d.tmux.SendKeysLiteral(repo.TmuxSession, agent.TmuxWindow, messageText); err != nil {
					d.logger.Error("Failed to deliver message text %s to %s/%s: %v", msg.ID, repoName, agentName, err)
					continue
				}

				// Send Enter key to submit the message
				if err := d.tmux.SendEnter(repo.TmuxSession, agent.TmuxWindow); err != nil {
					d.logger.Error("Failed to send Enter for message %s to %s/%s: %v", msg.ID, repoName, agentName, err)
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
			}

			// Send message using literal mode to avoid escaping issues
			if err := d.tmux.SendKeysLiteral(repo.TmuxSession, agent.TmuxWindow, message); err != nil {
				d.logger.Error("Failed to send wake message to agent %s: %v", agentName, err)
				continue
			}
			if err := d.tmux.SendEnter(repo.TmuxSession, agent.TmuxWindow); err != nil {
				d.logger.Error("Failed to send Enter for wake message to agent %s: %v", agentName, err)
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

	default:
		return socket.Response{
			Success: false,
			Error:   fmt.Sprintf("unknown command: %s", req.Command),
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

// handleListRepos lists all repositories
func (d *Daemon) handleListRepos(req socket.Request) socket.Response {
	repos := d.state.ListRepos()
	return socket.Response{Success: true, Data: repos}
}

// handleAddRepo adds a new repository
func (d *Daemon) handleAddRepo(req socket.Request) socket.Response {
	name, ok := req.Args["name"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'name' argument"}
	}

	githubURL, ok := req.Args["github_url"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'github_url' argument"}
	}

	tmuxSession, ok := req.Args["tmux_session"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'tmux_session' argument"}
	}

	repo := &state.Repository{
		GithubURL:   githubURL,
		TmuxSession: tmuxSession,
		Agents:      make(map[string]state.Agent),
	}

	if err := d.state.AddRepo(name, repo); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Added repository: %s", name)
	return socket.Response{Success: true}
}

// handleAddAgent adds a new agent
func (d *Daemon) handleAddAgent(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'repo' argument"}
	}

	agentName, ok := req.Args["agent"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'agent' argument"}
	}

	agentTypeStr, ok := req.Args["type"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'type' argument"}
	}

	worktreePath, ok := req.Args["worktree_path"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'worktree_path' argument"}
	}

	tmuxWindow, ok := req.Args["tmux_window"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'tmux_window' argument"}
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
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'repo' argument"}
	}

	agentName, ok := req.Args["agent"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'agent' argument"}
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
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'repo' argument"}
	}

	agents, err := d.state.ListAgents(repoName)
	if err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	// Get full agent details
	agentDetails := make([]map[string]interface{}, 0, len(agents))
	for _, agentName := range agents {
		agent, exists := d.state.GetAgent(repoName, agentName)
		if !exists {
			continue
		}

		agentDetails = append(agentDetails, map[string]interface{}{
			"name":          agentName,
			"type":          agent.Type,
			"worktree_path": agent.WorktreePath,
			"tmux_window":   agent.TmuxWindow,
			"task":          agent.Task,
			"created_at":    agent.CreatedAt,
		})
	}

	return socket.Response{Success: true, Data: agentDetails}
}

// handleCompleteAgent marks an agent as ready for cleanup
func (d *Daemon) handleCompleteAgent(req socket.Request) socket.Response {
	repoName, ok := req.Args["repo"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'repo' argument"}
	}

	agentName, ok := req.Args["agent"].(string)
	if !ok {
		return socket.Response{Success: false, Error: "missing or invalid 'agent' argument"}
	}

	agent, exists := d.state.GetAgent(repoName, agentName)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("agent %s not found in repo %s", agentName, repoName)}
	}

	// Mark as ready for cleanup
	agent.ReadyForCleanup = true
	if err := d.state.UpdateAgent(repoName, agentName, agent); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Agent %s/%s marked as ready for cleanup", repoName, agentName)

	// Notify supervisor and merge-queue that worker completed
	if agent.Type == state.AgentTypeWorker {
		msgMgr := d.getMessageManager()
		task := agent.Task
		if task == "" {
			task = "unknown task"
		}

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

			// Check if worktree exists (for workers)
			if agent.Type == state.AgentTypeWorker && agent.WorktreePath != "" {
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

			// Clean up worktree if it exists
			if agent.WorktreePath != "" && agent.Type == state.AgentTypeWorker {
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
