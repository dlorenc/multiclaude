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

	"github.com/dlorenc/multiclaude/internal/agents"
	"github.com/dlorenc/multiclaude/internal/hooks"
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
	d.wg.Add(5)
	go d.healthCheckLoop()
	go d.messageRouterLoop()
	go d.wakeLoop()
	go d.serverLoop()
	go d.worktreeRefreshLoop()

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

// getRequiredStringArg extracts a required string argument from request Args.
// Returns the value and true if present, or an error response and false if missing.
func getRequiredStringArg(args map[string]interface{}, key, description string) (string, socket.Response, bool) {
	val, ok := args[key].(string)
	if !ok || val == "" {
		return "", socket.Response{
			Success: false,
			Error:   fmt.Sprintf("missing '%s': %s", key, description),
		}, false
	}
	return val, socket.Response{}, true
}

// periodicLoop runs a function periodically at the specified interval.
// If onStartup is provided, it's called immediately before entering the loop.
// The onTick function is called on each timer tick.
func (d *Daemon) periodicLoop(name string, interval time.Duration, onStartup, onTick func()) {
	defer d.wg.Done()
	d.logger.Info("Starting %s loop", name)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run startup tasks if provided
	if onStartup != nil {
		onStartup()
	}

	for {
		select {
		case <-ticker.C:
			onTick()
		case <-d.ctx.Done():
			d.logger.Info("%s loop stopped", name)
			return
		}
	}
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
	startup := func() {
		d.checkAgentHealth()
		d.rotateLogsIfNeeded()
		d.cleanupMergedBranches()
	}
	d.periodicLoop("health check", 2*time.Minute, startup, startup)
}

// checkAgentHealth checks if agents are still alive
func (d *Daemon) checkAgentHealth() {
	d.logger.Debug("Checking agent health")

	deadAgents := make(map[string][]string) // repo -> []agent names

	// Get a snapshot of repos to avoid concurrent map access
	repos := d.state.GetAllRepos()
	for repoName, repo := range repos {
		// Check if tmux session exists
		hasSession, err := d.tmux.HasSession(d.ctx, repo.TmuxSession)
		if err != nil {
			d.logger.Error("Failed to check session %s: %v", repo.TmuxSession, err)
			continue
		}

		if !hasSession {
			d.logger.Warn("Tmux session %s not found for repo %s, attempting restoration", repo.TmuxSession, repoName)
			// Try to restore the session and agents instead of cleaning up
			if err := d.restoreRepoAgents(repoName, repo); err != nil {
				d.logger.Error("Failed to restore repo %s: %v, marking all agents for cleanup", repoName, err)
				// Only mark for cleanup if restoration failed
				for agentName := range repo.Agents {
					appendToSliceMap(deadAgents, repoName, agentName)
				}
			} else {
				d.logger.Info("Successfully restored tmux session and agents for repo %s", repoName)
			}
			continue
		}

		// Check each agent
		for agentName, agent := range repo.Agents {
			// Check if agent is marked as ready for cleanup
			if agent.ReadyForCleanup {
				d.logger.Info("Agent %s is ready for cleanup", agentName)
				appendToSliceMap(deadAgents, repoName, agentName)
				continue
			}

			// Check if window exists
			hasWindow, err := d.tmux.HasWindow(d.ctx, repo.TmuxSession, agent.TmuxWindow)
			if err != nil {
				d.logger.Error("Failed to check window %s: %v", agent.TmuxWindow, err)
				continue
			}

			if !hasWindow {
				d.logger.Warn("Agent %s window not found, marking for cleanup", agentName)
				appendToSliceMap(deadAgents, repoName, agentName)
				continue
			}

			// Check if process is alive (if we have a PID)
			if agent.PID > 0 {
				if !isProcessAlive(agent.PID) {
					d.logger.Warn("Agent %s process (PID %d) not running", agentName, agent.PID)

					// For persistent agents, attempt auto-restart
					if agent.Type.IsPersistent() {
						d.logger.Info("Attempting to auto-restart agent %s", agentName)
						if err := d.restartAgent(repoName, agentName, agent, repo); err != nil {
							d.logger.Error("Failed to restart agent %s: %v", agentName, err)
						} else {
							d.logger.Info("Successfully restarted agent %s", agentName)
						}
					}
					// For transient agents (workers, review), don't auto-restart - they complete and clean up
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
	d.periodicLoop("message router", 2*time.Minute, nil, d.routeMessages)
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
				if err := d.tmux.SendKeysLiteralWithEnter(d.ctx, repo.TmuxSession, agent.TmuxWindow, messageText); err != nil {
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
	d.periodicLoop("wake", 2*time.Minute, nil, d.wakeAgents)
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
			case state.AgentTypePRShepherd:
				message = "Status check: Review PRs on upstream, check CI status, and rebase branches if needed."
			case state.AgentTypeWorker:
				message = "Status check: Update on your progress?"
			case state.AgentTypeReview:
				message = "Status check: Update on your review progress?"
			case state.AgentTypeGenericPersistent:
				message = "Status check: Update on your progress?"
			}

			// Send message using atomic method to avoid race conditions (issue #63)
			if err := d.tmux.SendKeysLiteralWithEnter(d.ctx, repo.TmuxSession, agent.TmuxWindow, message); err != nil {
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

// worktreeRefreshLoop periodically syncs worker worktrees with main branch
func (d *Daemon) worktreeRefreshLoop() {
	defer d.wg.Done()
	d.logger.Info("Starting worktree refresh loop")

	// Run every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run once after a short delay on startup (respecting context cancellation)
	select {
	case <-time.After(30 * time.Second):
		d.refreshWorktrees()
	case <-d.ctx.Done():
		d.logger.Info("Worktree refresh loop stopped")
		return
	}

	for {
		select {
		case <-ticker.C:
			d.refreshWorktrees()
		case <-d.ctx.Done():
			d.logger.Info("Worktree refresh loop stopped")
			return
		}
	}
}

// refreshWorktrees syncs worker worktrees that are behind main
func (d *Daemon) refreshWorktrees() {
	d.logger.Debug("Checking worker worktrees for refresh")

	repos := d.state.GetAllRepos()
	for repoName, repo := range repos {
		repoPath := d.paths.RepoDir(repoName)

		// Check if repo path exists
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			continue
		}

		wt := worktree.NewManager(repoPath)

		// Get the upstream remote and default branch
		remote, err := wt.GetUpstreamRemote()
		if err != nil {
			d.logger.Debug("Could not get remote for %s: %v", repoName, err)
			continue
		}

		mainBranch, err := wt.GetDefaultBranch(remote)
		if err != nil {
			d.logger.Debug("Could not get default branch for %s: %v", repoName, err)
			continue
		}

		// Fetch from remote to have latest state
		if err := wt.FetchRemote(remote); err != nil {
			d.logger.Debug("Could not fetch from remote for %s: %v", repoName, err)
			continue
		}

		// Check each worker agent's worktree
		for agentName, agent := range repo.Agents {
			// Only refresh worker worktrees
			if agent.Type != state.AgentTypeWorker {
				continue
			}

			// Skip if worktree path is empty
			if agent.WorktreePath == "" {
				continue
			}

			// Check if worktree exists
			if _, err := os.Stat(agent.WorktreePath); os.IsNotExist(err) {
				continue
			}

			// Check worktree state
			wtState, err := worktree.GetWorktreeState(agent.WorktreePath, remote, mainBranch)
			if err != nil {
				d.logger.Debug("Could not get worktree state for %s/%s: %v", repoName, agentName, err)
				continue
			}

			// Skip if can't refresh (detached HEAD, mid-rebase, mid-merge, on main, or up to date)
			if !wtState.CanRefresh {
				d.logger.Debug("Skipping refresh for %s/%s: %s", repoName, agentName, wtState.RefreshReason)
				continue
			}

			// Refresh the worktree
			d.logger.Info("Refreshing worktree for %s/%s (%d commits behind)", repoName, agentName, wtState.CommitsBehind)
			result := worktree.RefreshWorktree(agent.WorktreePath, remote, mainBranch)

			if result.Error != nil {
				if result.HasConflicts {
					d.logger.Warn("Worktree refresh for %s/%s has conflicts in: %v", repoName, agentName, result.ConflictFiles)
				} else {
					d.logger.Error("Failed to refresh worktree for %s/%s: %v", repoName, agentName, result.Error)
				}
			} else if result.Skipped {
				d.logger.Debug("Worktree refresh for %s/%s skipped: %s", repoName, agentName, result.SkipReason)
			} else {
				d.logger.Info("Refreshed worktree for %s/%s: rebased %d commits", repoName, agentName, result.CommitsRebased)

				// Notify the agent that their worktree was refreshed
				msgMgr := d.getMessageManager()
				msg := fmt.Sprintf("Your worktree has been automatically synced with main (rebased %d commits). Run 'git log --oneline -5' to see recent changes.", result.CommitsRebased)
				if _, err := msgMgr.Send(repoName, "daemon", agentName, msg); err != nil {
					d.logger.Debug("Could not send refresh notification to %s/%s: %v", repoName, agentName, err)
				}
			}
		}
	}
}

// TriggerWorktreeRefresh triggers an immediate worktree refresh (for testing)
func (d *Daemon) TriggerWorktreeRefresh() {
	d.refreshWorktrees()
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

	case "restart_agent":
		return d.handleRestartAgent(req)

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

	case "spawn_agent":
		return d.handleSpawnAgent(req)

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
		if hasSession, err := d.tmux.HasSession(d.ctx, repo.TmuxSession); err == nil {
			sessionHealthy = hasSession
		}

		// Determine PR management mode
		prManagementMode := "merge-queue"
		if repo.ForkConfig.IsFork {
			prManagementMode = "pr-shepherd"
		}

		repoDetails = append(repoDetails, map[string]interface{}{
			"name":               repoName,
			"github_url":         repo.GithubURL,
			"tmux_session":       repo.TmuxSession,
			"total_agents":       totalAgents,
			"worker_count":       workerCount,
			"session_healthy":    sessionHealthy,
			"is_fork":            repo.ForkConfig.IsFork,
			"upstream_owner":     repo.ForkConfig.UpstreamOwner,
			"upstream_repo":      repo.ForkConfig.UpstreamRepo,
			"pr_management_mode": prManagementMode,
		})
	}

	return socket.Response{Success: true, Data: repoDetails}
}

// handleAddRepo adds a new repository
func (d *Daemon) handleAddRepo(req socket.Request) socket.Response {
	name, errResp, ok := getRequiredStringArg(req.Args, "name", "repository name is required (e.g., 'my-project')")
	if !ok {
		return errResp
	}

	githubURL, errResp, ok := getRequiredStringArg(req.Args, "github_url", "GitHub repository URL is required (e.g., 'https://github.com/owner/repo')")
	if !ok {
		return errResp
	}

	tmuxSession, errResp, ok := getRequiredStringArg(req.Args, "tmux_session", "tmux session name is required")
	if !ok {
		return errResp
	}

	// Parse merge queue configuration (optional, defaults to enabled with "all" tracking)
	mqConfig := state.DefaultMergeQueueConfig()
	if mqEnabled, ok := req.Args["mq_enabled"].(bool); ok {
		mqConfig.Enabled = mqEnabled
	}
	if mqTrackMode, ok := req.Args["mq_track_mode"].(string); ok {
		mode, err := state.ParseTrackMode(mqTrackMode)
		if err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		mqConfig.TrackMode = mode
	}

	// Parse fork configuration (optional)
	var forkConfig state.ForkConfig
	if isFork, ok := req.Args["is_fork"].(bool); ok {
		forkConfig.IsFork = isFork
	}
	if upstreamURL, ok := req.Args["upstream_url"].(string); ok {
		forkConfig.UpstreamURL = upstreamURL
	}
	if upstreamOwner, ok := req.Args["upstream_owner"].(string); ok {
		forkConfig.UpstreamOwner = upstreamOwner
	}
	if upstreamRepo, ok := req.Args["upstream_repo"].(string); ok {
		forkConfig.UpstreamRepo = upstreamRepo
	}

	// Parse PR shepherd configuration (optional, defaults for fork mode)
	psConfig := state.DefaultPRShepherdConfig()
	if psEnabled, ok := req.Args["ps_enabled"].(bool); ok {
		psConfig.Enabled = psEnabled
	}
	if psTrackMode, ok := req.Args["ps_track_mode"].(string); ok {
		mode, err := state.ParseTrackMode(psTrackMode)
		if err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		psConfig.TrackMode = mode
	}

	// If in fork mode, disable merge-queue and enable pr-shepherd by default
	if forkConfig.IsFork {
		mqConfig.Enabled = false
		psConfig.Enabled = true
	}

	repo := &state.Repository{
		GithubURL:        githubURL,
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: mqConfig,
		PRShepherdConfig: psConfig,
		ForkConfig:       forkConfig,
	}

	if err := d.state.AddRepo(name, repo); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	if forkConfig.IsFork {
		d.logger.Info("Added repository: %s (fork of %s/%s, pr-shepherd: enabled=%v)", name, forkConfig.UpstreamOwner, forkConfig.UpstreamRepo, psConfig.Enabled)
	} else {
		d.logger.Info("Added repository: %s (merge queue: enabled=%v, track=%s)", name, mqConfig.Enabled, mqConfig.TrackMode)
	}
	return socket.Response{Success: true}
}

// handleRemoveRepo removes a repository from state
func (d *Daemon) handleRemoveRepo(req socket.Request) socket.Response {
	name, errResp, ok := getRequiredStringArg(req.Args, "name", "repository name is required")
	if !ok {
		return errResp
	}

	if err := d.state.RemoveRepo(name); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Removed repository: %s", name)
	return socket.Response{Success: true}
}

// handleAddAgent adds a new agent
func (d *Daemon) handleAddAgent(req socket.Request) socket.Response {
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
	}

	agentName, errResp, ok := getRequiredStringArg(req.Args, "agent", "agent name is required")
	if !ok {
		return errResp
	}

	agentTypeStr, errResp, ok := getRequiredStringArg(req.Args, "type", "agent type is required (supervisor, worker, merge-queue, or reviewer)")
	if !ok {
		return errResp
	}

	worktreePath, errResp, ok := getRequiredStringArg(req.Args, "worktree_path", "path to the agent's git worktree is required")
	if !ok {
		return errResp
	}

	tmuxWindow, errResp, ok := getRequiredStringArg(req.Args, "tmux_window", "tmux window name is required")
	if !ok {
		return errResp
	}

	// Get session ID from args or generate one
	sessionID, _, ok := getRequiredStringArg(req.Args, "session_id", "")
	if !ok {
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
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
	}

	agentName, errResp, ok := getRequiredStringArg(req.Args, "agent", "agent name is required")
	if !ok {
		return errResp
	}

	if err := d.state.RemoveAgent(repoName, agentName); err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	d.logger.Info("Removed agent %s from repo %s", agentName, repoName)
	return socket.Response{Success: true}
}

// handleListAgents lists agents for a repository
func (d *Daemon) handleListAgents(req socket.Request) socket.Response {
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
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
				hasWindow, err := d.tmux.HasWindow(d.ctx, repo.TmuxSession, agent.TmuxWindow)
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
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
	}

	agentName, errResp, ok := getRequiredStringArg(req.Args, "agent", "agent name is required")
	if !ok {
		return errResp
	}

	agent, exists := d.state.GetAgent(repoName, agentName)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("agent '%s' not found in repository '%s' - check available agents with: multiclaude worker list --repo %s", agentName, repoName, repoName)}
	}

	// Mark as ready for cleanup
	agent.ReadyForCleanup = true

	// Optional: capture summary and failure reason for task history
	if summary, ok := req.Args["summary"].(string); ok && summary != "" {
		agent.Summary = summary
	}
	if failureReason, ok := req.Args["failure_reason"].(string); ok && failureReason != "" {
		agent.FailureReason = failureReason
	}

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

// handleRestartAgent restarts an agent that has crashed or exited
func (d *Daemon) handleRestartAgent(req socket.Request) socket.Response {
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
	}

	agentName, errResp, ok := getRequiredStringArg(req.Args, "agent", "agent name is required")
	if !ok {
		return errResp
	}

	force, _ := req.Args["force"].(bool)

	agent, exists := d.state.GetAgent(repoName, agentName)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("agent '%s' not found in repository '%s' - check available agents with: multiclaude worker list --repo %s", agentName, repoName, repoName)}
	}

	// Check if agent is marked for cleanup (completed)
	if agent.ReadyForCleanup {
		return socket.Response{Success: false, Error: fmt.Sprintf("agent '%s' is marked as complete and pending cleanup - cannot restart a completed agent", agentName)}
	}

	// Check if tmux window exists
	repo, exists := d.state.GetRepo(repoName)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("repository '%s' not found in state", repoName)}
	}

	hasWindow, err := d.tmux.HasWindow(d.ctx, repo.TmuxSession, agentName)
	if err != nil {
		return socket.Response{Success: false, Error: fmt.Sprintf("failed to check tmux window: %v", err)}
	}
	if !hasWindow {
		return socket.Response{Success: false, Error: fmt.Sprintf("tmux window '%s' does not exist - the agent may need to be recreated", agentName)}
	}

	// Check if agent is already running
	if agent.PID > 0 && isProcessAlive(agent.PID) {
		if !force {
			return socket.Response{Success: false, Error: fmt.Sprintf("agent '%s' is already running with PID %d - use --force to restart anyway", agentName, agent.PID)}
		}
		d.logger.Info("Force restarting agent %s (PID %d was still running)", agentName, agent.PID)
	}

	// Restart the agent
	if err := d.restartAgent(repoName, agentName, agent, repo); err != nil {
		return socket.Response{Success: false, Error: fmt.Sprintf("failed to restart agent: %v", err)}
	}

	// Get updated PID from state
	updatedAgent, _ := d.state.GetAgent(repoName, agentName)
	return socket.Response{
		Success: true,
		Data: map[string]interface{}{
			"agent":   agentName,
			"repo":    repoName,
			"pid":     updatedAgent.PID,
			"message": fmt.Sprintf("Agent '%s' restarted successfully", agentName),
		},
	}
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
		hasSession, err := d.tmux.HasSession(d.ctx, repo.TmuxSession)
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
			hasWindow, _ := d.tmux.HasWindow(d.ctx, repo.TmuxSession, agent.TmuxWindow)
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
	name, errResp, ok := getRequiredStringArg(req.Args, "name", "repository name is required")
	if !ok {
		return errResp
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

	// Get PR shepherd config (use default if not set)
	psConfig := repo.PRShepherdConfig
	if psConfig.TrackMode == "" {
		psConfig = state.DefaultPRShepherdConfig()
	}

	// Get fork config
	forkConfig := repo.ForkConfig

	return socket.Response{
		Success: true,
		Data: map[string]interface{}{
			"mq_enabled":      mqConfig.Enabled,
			"mq_track_mode":   string(mqConfig.TrackMode),
			"ps_enabled":      psConfig.Enabled,
			"ps_track_mode":   string(psConfig.TrackMode),
			"is_fork":         forkConfig.IsFork,
			"upstream_url":    forkConfig.UpstreamURL,
			"upstream_owner":  forkConfig.UpstreamOwner,
			"upstream_repo":   forkConfig.UpstreamRepo,
			"force_fork_mode": forkConfig.ForceForkMode,
		},
	}
}

// handleUpdateRepoConfig updates the configuration for a repository
func (d *Daemon) handleUpdateRepoConfig(req socket.Request) socket.Response {
	name, errResp, ok := getRequiredStringArg(req.Args, "name", "repository name is required")
	if !ok {
		return errResp
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
		mode, err := state.ParseTrackMode(mqTrackMode)
		if err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		currentMQConfig.TrackMode = mode
		mqUpdated = true
	}

	if mqUpdated {
		if err := d.state.UpdateMergeQueueConfig(name, currentMQConfig); err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		d.logger.Info("Updated merge queue config for repo %s: enabled=%v, track=%s", name, currentMQConfig.Enabled, currentMQConfig.TrackMode)
	}

	// Get current PR shepherd config
	currentPSConfig, err := d.state.GetPRShepherdConfig(name)
	if err != nil {
		return socket.Response{Success: false, Error: err.Error()}
	}

	// Update PR shepherd config with provided values
	psUpdated := false
	if psEnabled, ok := req.Args["ps_enabled"].(bool); ok {
		currentPSConfig.Enabled = psEnabled
		psUpdated = true
	}
	if psTrackMode, ok := req.Args["ps_track_mode"].(string); ok {
		mode, err := state.ParseTrackMode(psTrackMode)
		if err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		currentPSConfig.TrackMode = mode
		psUpdated = true
	}

	if psUpdated {
		if err := d.state.UpdatePRShepherdConfig(name, currentPSConfig); err != nil {
			return socket.Response{Success: false, Error: err.Error()}
		}
		d.logger.Info("Updated PR shepherd config for repo %s: enabled=%v, track=%s", name, currentPSConfig.Enabled, currentPSConfig.TrackMode)
	}

	return socket.Response{Success: true}
}

// handleSetCurrentRepo sets the current/default repository
func (d *Daemon) handleSetCurrentRepo(req socket.Request) socket.Response {
	name, errResp, ok := getRequiredStringArg(req.Args, "name", "repository name is required")
	if !ok {
		return errResp
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
			if err := d.tmux.KillWindow(d.ctx, repo.TmuxSession, agent.TmuxWindow); err != nil {
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

	// Determine initial status
	status := state.TaskStatusUnknown
	if agent.FailureReason != "" {
		status = state.TaskStatusFailed
	}

	entry := state.TaskHistoryEntry{
		Name:          agentName,
		Task:          agent.Task,
		Branch:        branch,
		Status:        status, // Will be updated when displaying if a PR exists
		Summary:       agent.Summary,
		FailureReason: agent.FailureReason,
		CreatedAt:     agent.CreatedAt,
		CompletedAt:   time.Now(),
	}

	if err := d.state.AddTaskHistory(repoName, entry); err != nil {
		d.logger.Warn("Failed to record task history for %s: %v", agentName, err)
	} else {
		d.logger.Info("Recorded task history for %s (branch: %s, summary: %q)", agentName, branch, agent.Summary)
	}
}

// handleTaskHistory returns the task history for a repository
func (d *Daemon) handleTaskHistory(req socket.Request) socket.Response {
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
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
			"name":           entry.Name,
			"task":           entry.Task,
			"branch":         entry.Branch,
			"pr_url":         entry.PRURL,
			"pr_number":      entry.PRNumber,
			"status":         string(entry.Status),
			"summary":        entry.Summary,
			"failure_reason": entry.FailureReason,
			"created_at":     entry.CreatedAt,
			"completed_at":   entry.CompletedAt,
		}
	}

	return socket.Response{Success: true, Data: result}
}

// handleSpawnAgent spawns a new agent with an inline prompt (no hardcoded type).
// This is used by the supervisor to spawn agents based on markdown definitions.
// Args:
//   - repo: repository name
//   - name: agent name (used for tmux window and worktree)
//   - class: "persistent" or "ephemeral"
//   - prompt: full prompt text to use as system prompt
//   - task: optional task description (for ephemeral/worker agents)
func (d *Daemon) handleSpawnAgent(req socket.Request) socket.Response {
	repoName, errResp, ok := getRequiredStringArg(req.Args, "repo", "repository name is required")
	if !ok {
		return errResp
	}

	agentName, errResp, ok := getRequiredStringArg(req.Args, "name", "agent name is required")
	if !ok {
		return errResp
	}

	agentClass, errResp, ok := getRequiredStringArg(req.Args, "class", "agent class is required (persistent or ephemeral)")
	if !ok {
		return errResp
	}

	promptText, errResp, ok := getRequiredStringArg(req.Args, "prompt", "prompt text is required")
	if !ok {
		return errResp
	}

	// Validate class
	if agentClass != "persistent" && agentClass != "ephemeral" {
		return socket.Response{
			Success: false,
			Error:   fmt.Sprintf("invalid agent class %q: must be 'persistent' or 'ephemeral'", agentClass),
		}
	}

	// Get optional task
	task, _ := req.Args["task"].(string)

	// Get repository
	repo, exists := d.state.GetRepo(repoName)
	if !exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("repository %q not found", repoName)}
	}

	// Check if agent already exists
	if _, exists := d.state.GetAgent(repoName, agentName); exists {
		return socket.Response{Success: false, Error: fmt.Sprintf("agent %q already exists in repository %q", agentName, repoName)}
	}

	// Determine agent type based on class
	var agentType state.AgentType
	if agentClass == "persistent" {
		// For persistent agents, use specific type if known or generic persistent
		switch agentName {
		case "merge-queue":
			agentType = state.AgentTypeMergeQueue
		case "pr-shepherd":
			agentType = state.AgentTypePRShepherd
		default:
			agentType = state.AgentTypeGenericPersistent
		}
	} else {
		// Ephemeral agents are workers or reviewers
		if strings.Contains(strings.ToLower(agentName), "review") {
			agentType = state.AgentTypeReview
		} else {
			agentType = state.AgentTypeWorker
		}
	}

	// Create worktree for the agent
	repoPath := d.paths.RepoDir(repoName)
	worktreePath := d.paths.AgentWorktree(repoName, agentName)

	wt := worktree.NewManager(repoPath)

	// Create worktree - persistent agents use repo dir, ephemeral get their own branch
	if agentClass == "persistent" {
		// Persistent agents work directly in the repo directory
		worktreePath = repoPath
	} else {
		// Ephemeral agents get their own worktree with a new branch
		branchName := fmt.Sprintf("work/%s", agentName)
		if err := wt.CreateNewBranch(worktreePath, branchName, "HEAD"); err != nil {
			return socket.Response{Success: false, Error: fmt.Sprintf("failed to create worktree: %v", err)}
		}
	}

	// Create tmux window with working directory
	cmd := exec.Command("tmux", "new-window", "-d", "-t", repo.TmuxSession, "-n", agentName, "-c", worktreePath)
	if err := cmd.Run(); err != nil {
		// Clean up worktree on failure (only for ephemeral agents that have their own worktree)
		if agentClass != "persistent" {
			wt.Remove(worktreePath, true)
		}
		return socket.Response{Success: false, Error: fmt.Sprintf("failed to create tmux window: %v", err)}
	}

	// Write prompt to file
	promptDir := filepath.Join(d.paths.Root, "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		return socket.Response{Success: false, Error: fmt.Sprintf("failed to create prompt directory: %v", err)}
	}

	promptPath := filepath.Join(promptDir, fmt.Sprintf("%s.md", agentName))
	if err := os.WriteFile(promptPath, []byte(promptText), 0644); err != nil {
		return socket.Response{Success: false, Error: fmt.Sprintf("failed to write prompt file: %v", err)}
	}

	// Copy hooks config
	if err := hooks.CopyConfig(repoPath, worktreePath); err != nil {
		d.logger.Warn("Failed to copy hooks config: %v", err)
	}

	// Start Claude in the tmux window
	cfg := agentStartConfig{
		agentName:  agentName,
		agentType:  agentType,
		promptFile: promptPath,
		workDir:    worktreePath,
	}

	if err := d.startAgentWithConfig(repoName, repo, cfg); err != nil {
		// Clean up on failure
		d.tmux.KillWindow(d.ctx, repo.TmuxSession, agentName)
		if agentClass != "persistent" {
			wt.Remove(worktreePath, true)
		}
		return socket.Response{Success: false, Error: fmt.Sprintf("failed to start agent: %v", err)}
	}

	// Update task if provided
	if task != "" {
		agent, _ := d.state.GetAgent(repoName, agentName)
		agent.Task = task
		d.state.UpdateAgent(repoName, agentName, agent)
	}

	d.logger.Info("Spawned agent %s/%s (class=%s, type=%s)", repoName, agentName, agentClass, agentType)

	return socket.Response{
		Success: true,
		Data: map[string]interface{}{
			"name":          agentName,
			"class":         agentClass,
			"type":          string(agentType),
			"worktree_path": worktreePath,
		},
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
// or have dead Claude processes
func (d *Daemon) restoreTrackedRepos() {
	d.logger.Info("Checking tracked repos for restoration")

	repos := d.state.GetAllRepos()
	for repoName, repo := range repos {
		// Check if tmux session exists
		hasSession, err := d.tmux.HasSession(d.ctx, repo.TmuxSession)
		if err != nil {
			d.logger.Error("Failed to check session %s: %v", repo.TmuxSession, err)
			continue
		}

		if hasSession {
			d.logger.Debug("Tmux session %s exists for repo %s", repo.TmuxSession, repoName)
			// Session exists but agents might have dead processes - check and restart them
			d.restoreDeadAgents(repoName, repo)
			continue
		}

		// Session doesn't exist - restore it
		d.logger.Info("Restoring agents for repo %s (tmux session %s was missing)", repoName, repo.TmuxSession)
		if err := d.restoreRepoAgents(repoName, repo); err != nil {
			d.logger.Error("Failed to restore agents for repo %s: %v", repoName, err)
		}
	}
}

// restoreDeadAgents restarts agents that have dead Claude processes but existing tmux windows.
// This is called on daemon startup when the tmux session exists but Claude processes may have died
// (e.g., after a system restart or Claude crash).
func (d *Daemon) restoreDeadAgents(repoName string, repo *state.Repository) {
	d.logger.Debug("Checking for dead agents in repo %s", repoName)

	for agentName, agent := range repo.Agents {
		// Skip agents without a PID (shouldn't happen, but be safe)
		if agent.PID <= 0 {
			d.logger.Debug("Agent %s has no PID, skipping", agentName)
			continue
		}

		// Check if the tmux window still exists
		hasWindow, err := d.tmux.HasWindow(d.ctx, repo.TmuxSession, agent.TmuxWindow)
		if err != nil {
			d.logger.Error("Failed to check window for agent %s: %v", agentName, err)
			continue
		}

		if !hasWindow {
			d.logger.Debug("Agent %s window not found, will be handled by health check", agentName)
			continue
		}

		// Check if the process is still alive
		if isProcessAlive(agent.PID) {
			d.logger.Debug("Agent %s process (PID %d) is alive", agentName, agent.PID)
			continue
		}

		// Process is dead but window exists - restart persistent agents with --resume
		d.logger.Info("Agent %s process (PID %d) is dead, attempting restart", agentName, agent.PID)

		// For persistent agents, auto-restart. For transient agents, they will be cleaned up by health check
		if agent.Type.IsPersistent() {
			if err := d.restartAgent(repoName, agentName, agent, repo); err != nil {
				d.logger.Error("Failed to restart agent %s: %v", agentName, err)
			} else {
				d.logger.Info("Successfully restarted agent %s with --resume", agentName)
			}
		} else {
			d.logger.Debug("Skipping transient agent %s (type %s) - will be cleaned up", agentName, agent.Type)
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

	// Start supervisor agent
	if err := d.startAgent(repoName, repo, "supervisor", state.AgentTypeSupervisor, repoPath); err != nil {
		d.logger.Error("Failed to start supervisor for %s: %v", repoName, err)
	}

	// Send agent definitions to supervisor (includes merge-queue config for supervisor to decide)
	if err := d.sendAgentDefinitionsToSupervisor(repoName, repoPath, mqConfig); err != nil {
		d.logger.Warn("Failed to send agent definitions to supervisor: %v", err)
	}

	// Create and restore workspace
	workspacePath := d.paths.AgentWorktree(repoName, "workspace")
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		// Workspace worktree doesn't exist, create it
		d.logger.Info("Creating workspace worktree for %s", repoName)
		wt := worktree.NewManager(repoPath)

		// Prune stale worktree references first - this handles the case where
		// worktree directories were deleted but git still has references to them
		if err := wt.Prune(); err != nil {
			d.logger.Warn("Failed to prune worktrees for %s: %v", repoName, err)
		}

		// Check for and migrate legacy "workspace" branch to "workspace/default"
		migrated, migrateErr := wt.MigrateLegacyWorkspaceBranch()
		if migrateErr != nil {
			d.logger.Warn("Failed to migrate legacy workspace branch for %s: %v", repoName, migrateErr)
		} else if migrated {
			d.logger.Info("Migrated legacy 'workspace' branch to 'workspace/default' for %s", repoName)
		}

		// Check if branch already exists to determine which creation method to use
		branchExists, err := wt.BranchExists("workspace/default")
		if err != nil {
			d.logger.Warn("Failed to check if workspace/default branch exists for %s: %v", repoName, err)
		}

		if branchExists {
			// Branch exists, create worktree using existing branch
			if err := wt.Create(workspacePath, "workspace/default"); err != nil {
				d.logger.Error("Failed to create workspace worktree with existing branch for %s: %v", repoName, err)
			}
		} else {
			// Branch doesn't exist, create worktree with new branch
			if err := wt.CreateNewBranch(workspacePath, "workspace/default", "HEAD"); err != nil {
				d.logger.Error("Failed to create workspace worktree with new branch for %s: %v", repoName, err)
			}
		}
	}

	// Now start the workspace agent if worktree exists
	if _, err := os.Stat(workspacePath); err == nil {
		cmd = exec.Command("tmux", "new-window", "-d", "-t", repo.TmuxSession, "-n", "workspace", "-c", workspacePath)
		if err := cmd.Run(); err != nil {
			d.logger.Error("Failed to create workspace window: %v", err)
		} else {
			if err := d.startAgent(repoName, repo, "workspace", state.AgentTypeWorkspace, workspacePath); err != nil {
				d.logger.Error("Failed to start workspace for %s: %v", repoName, err)
			}
		}
	}

	return nil
}

// sendAgentDefinitionsToSupervisor reads agent definitions and sends them to the supervisor.
// This allows the supervisor to know about available agents and spawn them as needed.
func (d *Daemon) sendAgentDefinitionsToSupervisor(repoName, repoPath string, mqConfig state.MergeQueueConfig) error {
	// Get repo to check fork config
	repo, exists := d.state.GetRepo(repoName)
	var forkConfig state.ForkConfig
	var psConfig state.PRShepherdConfig
	if exists {
		forkConfig = repo.ForkConfig
		psConfig = repo.PRShepherdConfig
	}

	// Create agent reader
	localAgentsDir := d.paths.RepoAgentsDir(repoName)
	reader := agents.NewReader(localAgentsDir, repoPath)

	// Read all definitions
	definitions, err := reader.ReadAllDefinitions()
	if err != nil {
		return fmt.Errorf("failed to read agent definitions: %w", err)
	}

	if len(definitions) == 0 {
		d.logger.Info("No agent definitions found for repo %s", repoName)
		return nil
	}

	// Build message with all definitions - send raw content for Claude to interpret
	var sb strings.Builder
	sb.WriteString("Agent definitions available for this repository:\n\n")

	// Include fork mode information if applicable
	isForkMode := forkConfig.IsFork || forkConfig.ForceForkMode
	if isForkMode {
		sb.WriteString("## Fork Mode (ACTIVE)\n")
		sb.WriteString(fmt.Sprintf("This repository is a fork of **%s/%s**.\n\n", forkConfig.UpstreamOwner, forkConfig.UpstreamRepo))
		sb.WriteString("**Key differences in fork mode:**\n")
		sb.WriteString("- Use `pr-shepherd` instead of `merge-queue`\n")
		sb.WriteString("- PRs target the upstream repository\n")
		sb.WriteString("- You cannot merge PRs - only prepare them for review\n\n")

		sb.WriteString("## PR Shepherd Configuration\n")
		if psConfig.Enabled {
			sb.WriteString("- Enabled: yes\n")
			sb.WriteString(fmt.Sprintf("- Track Mode: %s\n\n", psConfig.TrackMode))
		} else {
			sb.WriteString("- Enabled: no (do NOT spawn pr-shepherd agent)\n\n")
		}
	} else {
		// Include merge-queue configuration for non-fork mode
		sb.WriteString("## Merge Queue Configuration\n")
		if mqConfig.Enabled {
			sb.WriteString("- Enabled: yes\n")
			sb.WriteString(fmt.Sprintf("- Track Mode: %s\n\n", mqConfig.TrackMode))
		} else {
			sb.WriteString("- Enabled: no (do NOT spawn merge-queue agent)\n\n")
		}
	}

	for i, def := range definitions {
		// Skip merge-queue definition in fork mode
		if isForkMode && def.Name == "merge-queue" {
			continue
		}
		// Skip pr-shepherd definition in non-fork mode
		if !isForkMode && def.Name == "pr-shepherd" {
			continue
		}

		sb.WriteString(fmt.Sprintf("--- Agent Definition %d: %s (source: %s) ---\n", i+1, def.Name, def.Source))

		// For merge-queue, prepend the tracking mode configuration if enabled
		if def.Name == "merge-queue" && mqConfig.Enabled {
			trackModePrompt := prompts.GenerateTrackingModePrompt(string(mqConfig.TrackMode))
			sb.WriteString(trackModePrompt)
			sb.WriteString("\n\n")
		}

		// For pr-shepherd, prepend the tracking mode configuration if enabled
		if def.Name == "pr-shepherd" && psConfig.Enabled {
			trackModePrompt := prompts.GenerateTrackingModePrompt(string(psConfig.TrackMode))
			sb.WriteString(trackModePrompt)
			sb.WriteString("\n\n")
			// Also add fork workflow context
			forkPrompt := prompts.GenerateForkWorkflowPrompt(forkConfig.UpstreamOwner, forkConfig.UpstreamRepo, forkConfig.UpstreamOwner)
			sb.WriteString(forkPrompt)
			sb.WriteString("\n\n")
		}

		sb.WriteString(def.Content)
		sb.WriteString("\n--- End of Definition ---\n\n")
	}

	sb.WriteString("Review these definitions and determine which agents to spawn.\n")
	sb.WriteString("For each agent, decide:\n")
	sb.WriteString("- Class: Is it persistent (long-running, auto-restarts) or ephemeral (task-based, cleans up)?\n")
	sb.WriteString("- Spawn now: Should this agent start immediately on repository init?\n\n")
	sb.WriteString("To spawn an agent, save the prompt to a file and use:\n")
	sb.WriteString(fmt.Sprintf("  multiclaude agents spawn --repo %s --name <agent-name> --class <persistent|ephemeral> --prompt-file <file>\n", repoName))

	// Send message to supervisor
	msgMgr := d.getMessageManager()
	if _, err := msgMgr.Send(repoName, "daemon", "supervisor", sb.String()); err != nil {
		return fmt.Errorf("failed to send message to supervisor: %w", err)
	}

	d.logger.Info("Sent %d agent definition(s) to supervisor for repo %s", len(definitions), repoName)
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

// agentStartConfig holds configuration for starting an agent
type agentStartConfig struct {
	agentName  string
	agentType  state.AgentType
	promptFile string
	workDir    string
}

// startAgentWithConfig is the unified agent start function that handles all common logic
func (d *Daemon) startAgentWithConfig(repoName string, repo *state.Repository, cfg agentStartConfig) error {
	// Generate session ID
	sessionID, err := claude.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Copy hooks config if needed
	repoPath := d.paths.RepoDir(repoName)
	if err := hooks.CopyConfig(repoPath, cfg.workDir); err != nil {
		d.logger.Warn("Failed to copy hooks config: %v", err)
	}

	var pid int

	// Skip actual Claude startup in test mode
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		// Resolve claude binary path
		binaryPath, err := d.getClaudeBinaryPath()
		if err != nil {
			return fmt.Errorf("failed to resolve claude binary: %w", err)
		}

		// Build CLI command
		claudeCmd := fmt.Sprintf("%s --session-id %s --dangerously-skip-permissions --append-system-prompt-file %s",
			binaryPath, sessionID, cfg.promptFile)

		// Send command to tmux window
		target := fmt.Sprintf("%s:%s", repo.TmuxSession, cfg.agentName)
		cmd := exec.Command("tmux", "send-keys", "-t", target, claudeCmd, "C-m")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start Claude in tmux: %w", err)
		}

		// Wait a moment for Claude to start
		time.Sleep(500 * time.Millisecond)

		// Get PID
		pid, err = d.tmux.GetPanePID(d.ctx, repo.TmuxSession, cfg.agentName)
		if err != nil {
			return fmt.Errorf("failed to get Claude PID: %w", err)
		}
	}

	// Register agent with state
	agent := state.Agent{
		Type:         cfg.agentType,
		WorktreePath: cfg.workDir,
		TmuxWindow:   cfg.agentName,
		SessionID:    sessionID,
		PID:          pid,
		CreatedAt:    time.Now(),
	}

	if err := d.state.AddAgent(repoName, cfg.agentName, agent); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	d.logger.Info("Started and registered agent %s/%s", repoName, cfg.agentName)
	return nil
}

// startAgent starts a Claude agent in a tmux window and registers it with state
func (d *Daemon) startAgent(repoName string, repo *state.Repository, agentName string, agentType state.AgentType, workDir string) error {
	promptFile, err := d.writePromptFile(repoName, agentType, agentName)
	if err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	return d.startAgentWithConfig(repoName, repo, agentStartConfig{
		agentName:  agentName,
		agentType:  agentType,
		promptFile: promptFile,
		workDir:    workDir,
	})
}

// writePromptFileWithPrefix writes a prompt file with an optional prefix prepended to the content
func (d *Daemon) writePromptFileWithPrefix(repoName string, agentType state.AgentType, agentName, prefix string) (string, error) {
	repoPath := d.paths.RepoDir(repoName)

	// Get the base prompt (without CLI docs since we don't have them in daemon context)
	promptText, err := prompts.GetPrompt(repoPath, agentType, "")
	if err != nil {
		return "", fmt.Errorf("failed to get prompt: %w", err)
	}

	// Prepend prefix if provided
	if prefix != "" {
		promptText = prefix + "\n\n" + promptText
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

// restartAgent restarts an agent that has exited.
// It uses --resume to continue the existing session if history exists.
// This works for all agent types: supervisor, merge-queue, workspace, workers, and review agents.
func (d *Daemon) restartAgent(repoName, agentName string, agent state.Agent, repo *state.Repository) error {
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
	// Note: Slash commands are embedded in prompts, not via CLAUDE_CONFIG_DIR
	result, err := d.claudeRunner.Start(d.ctx, repo.TmuxSession, agentName, claude.Config{
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
func (d *Daemon) writePromptFile(repoName string, agentType state.AgentType, agentName string) (string, error) {
	return d.writePromptFileWithPrefix(repoName, agentType, agentName, "")
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

// appendToSliceMap appends a value to a slice in a map, initializing the slice if needed.
func appendToSliceMap(m map[string][]string, key, value string) {
	if m[key] == nil {
		m[key] = []string{}
	}
	m[key] = append(m[key], value)
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

// linkGlobalCredentials creates a symlink from the Claude config directory's .credentials.json
// to the global ~/.claude/.credentials.json. This ensures workers can access OAuth
// credentials without duplicating sensitive files.
//
// When CLAUDE_CONFIG_DIR is set, Claude looks for credentials there, not in the
// project's .claude directory or the global ~/.claude directory.
func (d *Daemon) linkGlobalCredentials(claudeConfigDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	globalCredFile := filepath.Join(home, ".claude", ".credentials.json")
	localCredFile := filepath.Join(claudeConfigDir, ".credentials.json")

	// Check if global credentials exist
	if _, err := os.Stat(globalCredFile); os.IsNotExist(err) {
		// No global credentials - user might be using API key
		return nil
	}

	// Ensure the config directory exists
	if err := os.MkdirAll(claudeConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if symlink already exists and is valid
	if linkTarget, err := os.Readlink(localCredFile); err == nil {
		if linkTarget == globalCredFile {
			// Already correctly linked
			return nil
		}
		// Invalid link, remove it
		os.Remove(localCredFile)
	} else if _, err := os.Stat(localCredFile); err == nil {
		// File exists but is not a symlink, remove it
		os.Remove(localCredFile)
	}

	// Create symlink
	if err := os.Symlink(globalCredFile, localCredFile); err != nil {
		return fmt.Errorf("failed to create credentials symlink: %w", err)
	}

	return nil
}

// repairCredentials fixes CLAUDE_CONFIG_DIR directories that are missing credential symlinks
func (d *Daemon) repairCredentials() (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("failed to get home directory: %w", err)
	}

	globalCredFile := filepath.Join(home, ".claude", ".credentials.json")

	// Check if global credentials exist
	if _, err := os.Stat(globalCredFile); os.IsNotExist(err) {
		// No global credentials - user might be using API key
		return 0, nil
	}

	fixed := 0
	for _, repoName := range d.state.ListRepos() {
		// Walk CLAUDE_CONFIG_DIR directories for this repo
		repoConfigDir := filepath.Join(d.paths.ClaudeConfigDir, repoName)

		// Skip if config directory doesn't exist
		if _, err := os.Stat(repoConfigDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(repoConfigDir)
		if err != nil {
			d.logger.Warn("Failed to read config dir for %s: %v", repoName, err)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			agentConfigDir := filepath.Join(repoConfigDir, entry.Name())
			localCredFile := filepath.Join(agentConfigDir, ".credentials.json")

			// Check if credentials already exist and are a valid symlink
			if linkTarget, err := os.Readlink(localCredFile); err == nil {
				if linkTarget == globalCredFile {
					// Already correctly linked
					continue
				}
				// Invalid symlink, will be recreated below
				os.Remove(localCredFile)
			} else if _, err := os.Stat(localCredFile); err == nil {
				// File exists but is not a symlink, remove it
				d.logger.Debug("Removing non-symlink credential file in %s/%s", repoName, entry.Name())
				os.Remove(localCredFile)
			} else if !os.IsNotExist(err) {
				// Some other error
				d.logger.Warn("Failed to check credentials in %s/%s: %v", repoName, entry.Name(), err)
				continue
			}

			// Create or recreate symlink
			if err := os.Symlink(globalCredFile, localCredFile); err != nil {
				d.logger.Warn("Failed to link credentials in %s/%s: %v", repoName, entry.Name(), err)
			} else {
				d.logger.Debug("Linked credentials in %s/%s", repoName, entry.Name())
				fixed++
			}
		}
	}

	return fixed, nil
}
