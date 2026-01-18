package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dlorenc/multiclaude/internal/logging"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/tmux"
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

	for repoName, repo := range d.state.Repos {
		// Check if tmux session exists
		hasSession, err := d.tmux.HasSession(repo.TmuxSession)
		if err != nil {
			d.logger.Error("Failed to check session %s: %v", repo.TmuxSession, err)
			continue
		}

		if !hasSession {
			d.logger.Warn("Tmux session %s not found for repo %s", repo.TmuxSession, repoName)
			// TODO: Mark repo for cleanup or recovery
			continue
		}

		// Check each agent
		for agentName, agent := range repo.Agents {
			// Check if window exists
			hasWindow, err := d.tmux.HasWindow(repo.TmuxSession, agent.TmuxWindow)
			if err != nil {
				d.logger.Error("Failed to check window %s: %v", agent.TmuxWindow, err)
				continue
			}

			if !hasWindow {
				d.logger.Warn("Agent %s window not found, marking for cleanup", agentName)
				// TODO: Mark agent for cleanup
				continue
			}

			// Check if process is alive (if we have a PID)
			if agent.PID > 0 {
				if !isProcessAlive(agent.PID) {
					d.logger.Warn("Agent %s process (PID %d) not running", agentName, agent.PID)
					// TODO: Mark agent for cleanup or restart
				}
			}
		}
	}
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
	// TODO: Implement message routing
	// 1. Walk messages directory
	// 2. Find pending messages
	// 3. Deliver via tmux send-keys
	// 4. Mark as delivered
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
	for repoName, repo := range d.state.Repos {
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

			if err := d.tmux.SendKeys(repo.TmuxSession, agent.TmuxWindow, message); err != nil {
				d.logger.Error("Failed to wake agent %s: %v", agentName, err)
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

	agent := state.Agent{
		Type:         state.AgentType(agentTypeStr),
		WorktreePath: worktreePath,
		TmuxWindow:   tmuxWindow,
		SessionID:    fmt.Sprintf("agent-%d", time.Now().UnixNano()),
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

// isProcessAlive checks if a process is running
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(os.Signal(nil))
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
