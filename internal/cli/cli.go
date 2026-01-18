package cli

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/errors"
	"github.com/dlorenc/multiclaude/internal/format"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/names"
	"github.com/dlorenc/multiclaude/internal/prompts"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/tmux"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/config"
)

// Command represents a CLI command
type Command struct {
	Name        string
	Description string
	Usage       string
	Run         func(args []string) error
	Subcommands map[string]*Command
}

// CLI manages the command-line interface
type CLI struct {
	rootCmd          *Command
	paths            *config.Paths
	claudeBinaryPath string // Full path to claude binary to prevent version drift
	documentation    string // Auto-generated CLI documentation for prompts
}

// New creates a new CLI
func New() (*CLI, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}

	// Resolve the full path to the claude binary to prevent version drift
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return nil, errors.ClaudeNotFound(err)
	}

	cli := &CLI{
		paths:            paths,
		claudeBinaryPath: claudePath,
		rootCmd: &Command{
			Name:        "multiclaude",
			Description: "repo-centric orchestrator for Claude Code",
			Subcommands: make(map[string]*Command),
		},
	}

	cli.registerCommands()

	// Generate documentation after commands are registered
	cli.documentation = cli.GenerateDocumentation()

	return cli, nil
}

// NewWithPaths creates a CLI with custom paths (for testing)
// If claudePath is empty, it will be set to "claude" (useful when MULTICLAUDE_TEST_MODE=1)
func NewWithPaths(paths *config.Paths, claudePath string) *CLI {
	if claudePath == "" {
		claudePath = "claude"
	}

	cli := &CLI{
		paths:            paths,
		claudeBinaryPath: claudePath,
		rootCmd: &Command{
			Name:        "multiclaude",
			Description: "repo-centric orchestrator for Claude Code",
			Subcommands: make(map[string]*Command),
		},
	}

	cli.registerCommands()

	// Generate documentation after commands are registered
	cli.documentation = cli.GenerateDocumentation()

	return cli
}

// Execute executes the CLI with the given arguments
func (c *CLI) Execute(args []string) error {
	if len(args) == 0 {
		return c.showHelp()
	}

	return c.executeCommand(c.rootCmd, args)
}

// executeCommand recursively executes commands and subcommands
func (c *CLI) executeCommand(cmd *Command, args []string) error {
	if len(args) == 0 {
		if cmd.Run != nil {
			return cmd.Run([]string{})
		}
		return c.showCommandHelp(cmd)
	}

	// Check for --help or -h flag
	if args[0] == "--help" || args[0] == "-h" {
		return c.showCommandHelp(cmd)
	}

	// Check for subcommands
	if subcmd, exists := cmd.Subcommands[args[0]]; exists {
		return c.executeCommand(subcmd, args[1:])
	}

	// No subcommand found, run this command with args
	if cmd.Run != nil {
		return cmd.Run(args)
	}

	return errors.UnknownCommand(args[0])
}

// showHelp shows the main help message
func (c *CLI) showHelp() error {
	fmt.Println("multiclaude - repo-centric orchestrator for Claude Code")
	fmt.Println()
	fmt.Println("Usage: multiclaude <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")

	for name, cmd := range c.rootCmd.Subcommands {
		fmt.Printf("  %-15s %s\n", name, cmd.Description)
	}

	fmt.Println()
	fmt.Println("Use 'multiclaude <command> --help' for more information about a command.")
	return nil
}

// showCommandHelp shows help for a specific command
func (c *CLI) showCommandHelp(cmd *Command) error {
	fmt.Printf("%s - %s\n", cmd.Name, cmd.Description)
	fmt.Println()
	if cmd.Usage != "" {
		fmt.Printf("Usage: %s\n", cmd.Usage)
		fmt.Println()
	}

	if len(cmd.Subcommands) > 0 {
		fmt.Println("Subcommands:")
		for name, subcmd := range cmd.Subcommands {
			fmt.Printf("  %-15s %s\n", name, subcmd.Description)
		}
		fmt.Println()
	}

	return nil
}

// registerCommands registers all CLI commands
func (c *CLI) registerCommands() {
	// Daemon commands
	c.rootCmd.Subcommands["start"] = &Command{
		Name:        "start",
		Description: "Start the multiclaude daemon",
		Usage:       "multiclaude start",
		Run:         c.startDaemon,
	}

	daemonCmd := &Command{
		Name:        "daemon",
		Description: "Manage the multiclaude daemon",
		Subcommands: make(map[string]*Command),
	}

	daemonCmd.Subcommands["start"] = &Command{
		Name:        "start",
		Description: "Start the daemon",
		Run:         c.startDaemon,
	}

	daemonCmd.Subcommands["stop"] = &Command{
		Name:        "stop",
		Description: "Stop the daemon",
		Run:         c.stopDaemon,
	}

	daemonCmd.Subcommands["status"] = &Command{
		Name:        "status",
		Description: "Check daemon status",
		Run:         c.daemonStatus,
	}

	daemonCmd.Subcommands["logs"] = &Command{
		Name:        "logs",
		Description: "View daemon logs",
		Run:         c.daemonLogs,
	}

	daemonCmd.Subcommands["_run"] = &Command{
		Name:        "_run",
		Description: "Internal: run daemon in foreground (used by daemon start)",
		Run:         c.runDaemon,
	}

	c.rootCmd.Subcommands["daemon"] = daemonCmd

	// Stop-all command (convenience for stopping everything)
	c.rootCmd.Subcommands["stop-all"] = &Command{
		Name:        "stop-all",
		Description: "Stop daemon and kill all multiclaude tmux sessions",
		Usage:       "multiclaude stop-all [--clean]",
		Run:         c.stopAll,
	}

	// Repository commands
	c.rootCmd.Subcommands["init"] = &Command{
		Name:        "init",
		Description: "Initialize a repository",
		Usage:       "multiclaude init <github-url> [path] [name]",
		Run:         c.initRepo,
	}

	c.rootCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List tracked repositories",
		Run:         c.listRepos,
	}

	// Worker commands
	workCmd := &Command{
		Name:        "work",
		Description: "Manage worker agents",
		Subcommands: make(map[string]*Command),
	}

	workCmd.Run = c.createWorker // Default action for 'work' command

	workCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List workers",
		Run:         c.listWorkers,
	}

	workCmd.Subcommands["rm"] = &Command{
		Name:        "rm",
		Description: "Remove a worker",
		Usage:       "multiclaude work rm <worker-name>",
		Run:         c.removeWorker,
	}

	c.rootCmd.Subcommands["work"] = workCmd

	// Agent commands (run from within Claude)
	agentCmd := &Command{
		Name:        "agent",
		Description: "Agent communication commands",
		Subcommands: make(map[string]*Command),
	}

	agentCmd.Subcommands["send-message"] = &Command{
		Name:        "send-message",
		Description: "Send a message to another agent",
		Run:         c.sendMessage,
	}

	agentCmd.Subcommands["list-messages"] = &Command{
		Name:        "list-messages",
		Description: "List messages",
		Run:         c.listMessages,
	}

	agentCmd.Subcommands["read-message"] = &Command{
		Name:        "read-message",
		Description: "Read a specific message",
		Run:         c.readMessage,
	}

	agentCmd.Subcommands["ack-message"] = &Command{
		Name:        "ack-message",
		Description: "Acknowledge a message",
		Run:         c.ackMessage,
	}

	agentCmd.Subcommands["complete"] = &Command{
		Name:        "complete",
		Description: "Signal worker completion",
		Run:         c.completeWorker,
	}

	c.rootCmd.Subcommands["agent"] = agentCmd

	// Attach command
	c.rootCmd.Subcommands["attach"] = &Command{
		Name:        "attach",
		Description: "Attach to an agent",
		Usage:       "multiclaude attach <agent-name> [--read-only]",
		Run:         c.attachAgent,
	}

	// Maintenance commands
	c.rootCmd.Subcommands["cleanup"] = &Command{
		Name:        "cleanup",
		Description: "Clean up orphaned resources",
		Usage:       "multiclaude cleanup [--dry-run] [--verbose]",
		Run:         c.cleanup,
	}

	c.rootCmd.Subcommands["repair"] = &Command{
		Name:        "repair",
		Description: "Repair state after crash",
		Usage:       "multiclaude repair [--verbose]",
		Run:         c.repair,
	}

	// Debug command
	c.rootCmd.Subcommands["docs"] = &Command{
		Name:        "docs",
		Description: "Show generated CLI documentation",
		Run:         c.showDocs,
	}

	// Review command
	c.rootCmd.Subcommands["review"] = &Command{
		Name:        "review",
		Description: "Spawn a review agent for a PR",
		Usage:       "multiclaude review <pr-url>",
		Run:         c.reviewPR,
	}
}

// Daemon command implementations

func (c *CLI) startDaemon(args []string) error {
	return daemon.RunDetached()
}

func (c *CLI) runDaemon(args []string) error {
	return daemon.Run()
}

func (c *CLI) stopDaemon(args []string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "stop",
	})
	if err != nil {
		return fmt.Errorf("failed to send stop command: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("daemon stop failed: %s", resp.Error)
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}

func (c *CLI) daemonStatus(args []string) error {
	// Check PID file first
	pidFile := daemon.NewPIDFile(c.paths.DaemonPID)
	running, pid, err := pidFile.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		fmt.Println("Daemon is not running")
		return nil
	}

	// Try to connect to daemon
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "status",
	})
	if err != nil {
		fmt.Printf("Daemon PID file exists (PID: %d) but daemon is not responding\n", pid)
		return nil
	}

	if !resp.Success {
		return fmt.Errorf("status check failed: %s", resp.Error)
	}

	// Pretty print status
	fmt.Println("Daemon Status:")
	if statusMap, ok := resp.Data.(map[string]interface{}); ok {
		fmt.Printf("  Running: %v\n", statusMap["running"])
		fmt.Printf("  PID: %v\n", statusMap["pid"])
		fmt.Printf("  Repos: %v\n", statusMap["repos"])
		fmt.Printf("  Agents: %v\n", statusMap["agents"])
		fmt.Printf("  Socket: %v\n", statusMap["socket_path"])
	} else {
		// Fallback: print as JSON
		jsonData, _ := json.MarshalIndent(resp.Data, "  ", "  ")
		fmt.Println(string(jsonData))
	}

	return nil
}

func (c *CLI) daemonLogs(args []string) error {
	flags, _ := ParseFlags(args)

	// Check if we should follow logs
	follow := flags["follow"] == "true" || flags["f"] == "true"

	if follow {
		// Use tail -f to follow logs
		cmd := exec.Command("tail", "-f", c.paths.DaemonLog)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Show last 50 lines
	lines := "50"
	if n, ok := flags["n"]; ok {
		lines = n
	}

	cmd := exec.Command("tail", "-n", lines, c.paths.DaemonLog)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLI) stopAll(args []string) error {
	flags, _ := ParseFlags(args)
	clean := flags["clean"] == "true"

	fmt.Println("Stopping all multiclaude sessions...")

	// Get list of repos (try daemon first, then state file)
	var repos []string
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "list_repos"})
	if err == nil && resp.Success {
		// Daemon is running, get repos from it
		if repoList, ok := resp.Data.([]interface{}); ok {
			for _, repo := range repoList {
				if repoStr, ok := repo.(string); ok {
					repos = append(repos, repoStr)
				}
			}
		}
	} else {
		// Daemon not running, try to load from state file
		st, err := state.Load(c.paths.StateFile)
		if err == nil {
			repos = st.ListRepos()
		}
	}

	// Kill all multiclaude tmux sessions
	tmuxClient := tmux.NewClient()
	if tmuxClient.IsTmuxAvailable() {
		for _, repo := range repos {
			sessionName := fmt.Sprintf("mc-%s", repo)
			exists, err := tmuxClient.HasSession(sessionName)
			if err == nil && exists {
				fmt.Printf("Killing tmux session: %s\n", sessionName)
				if err := tmuxClient.KillSession(sessionName); err != nil {
					fmt.Printf("Warning: failed to kill session %s: %v\n", sessionName, err)
				}
			}
		}

		// Also check for any mc-* sessions we might have missed
		sessions, err := tmuxClient.ListSessions()
		if err == nil {
			for _, session := range sessions {
				if strings.HasPrefix(session, "mc-") {
					exists := false
					for _, repo := range repos {
						if fmt.Sprintf("mc-%s", repo) == session {
							exists = true
							break
						}
					}
					if !exists {
						fmt.Printf("Killing orphaned tmux session: %s\n", session)
						if err := tmuxClient.KillSession(session); err != nil {
							fmt.Printf("Warning: failed to kill session %s: %v\n", session, err)
						}
					}
				}
			}
		}
	}

	// Stop the daemon
	fmt.Println("Stopping daemon...")
	resp, err = client.Send(socket.Request{Command: "stop"})
	if err != nil {
		fmt.Printf("Daemon already stopped or not responding\n")
	} else if resp.Success {
		fmt.Println("Daemon stopped")
	}

	// Clean up state if requested
	if clean {
		fmt.Println("\nCleaning up state and data...")

		// Remove state file
		if err := os.Remove(c.paths.StateFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove state file: %v\n", err)
		} else {
			fmt.Println("Removed state file")
		}

		// Remove PID file
		if err := os.Remove(c.paths.DaemonPID); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove PID file: %v\n", err)
		}

		// Remove socket file
		if err := os.Remove(c.paths.DaemonSock); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove socket file: %v\n", err)
		}

		fmt.Println("\nNote: Repositories, worktrees, and messages are preserved.")
		fmt.Println("To remove everything, manually delete: ~/.multiclaude/")
	}

	fmt.Println("\n✓ All multiclaude sessions stopped")
	return nil
}

func (c *CLI) initRepo(args []string) error {
	if len(args) < 1 {
		return errors.InvalidUsage("usage: multiclaude init <github-url> [name]")
	}

	githubURL := args[0]

	// Parse repository name from URL if not provided
	var repoName string
	if len(args) >= 2 {
		repoName = args[1]
	} else {
		// Extract repo name from URL (e.g., github.com/user/repo -> repo)
		parts := strings.Split(githubURL, "/")
		repoName = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}

	fmt.Printf("Initializing repository: %s\n", repoName)
	fmt.Printf("GitHub URL: %s\n", githubURL)

	// Check if daemon is running
	client := socket.NewClient(c.paths.DaemonSock)
	_, err := client.Send(socket.Request{Command: "ping"})
	if err != nil {
		return errors.DaemonNotRunning()
	}

	// Clone repository
	repoPath := c.paths.RepoDir(repoName)
	fmt.Printf("Cloning to: %s\n", repoPath)

	cmd := exec.Command("git", "clone", githubURL, repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.GitOperationFailed("clone", err)
	}

	// Create tmux session
	tmuxSession := fmt.Sprintf("mc-%s", repoName)

	fmt.Printf("Creating tmux session: %s\n", tmuxSession)

	// Create session with supervisor window
	cmd = exec.Command("tmux", "new-session", "-d", "-s", tmuxSession, "-n", "supervisor", "-c", repoPath)
	if err := cmd.Run(); err != nil {
		return errors.TmuxOperationFailed("create session", err)
	}

	// Create merge-queue window (detached so it doesn't switch focus)
	cmd = exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", "merge-queue", "-c", repoPath)
	if err := cmd.Run(); err != nil {
		return errors.TmuxOperationFailed("create window", err)
	}

	// Generate session IDs for agents
	supervisorSessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate supervisor session ID: %w", err)
	}

	mergeQueueSessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate merge-queue session ID: %w", err)
	}

	// Write prompt files
	supervisorPromptFile, err := c.writePromptFile(repoPath, prompts.TypeSupervisor, "supervisor")
	if err != nil {
		return fmt.Errorf("failed to write supervisor prompt: %w", err)
	}

	mergeQueuePromptFile, err := c.writePromptFile(repoPath, prompts.TypeMergeQueue, "merge-queue")
	if err != nil {
		return fmt.Errorf("failed to write merge-queue prompt: %w", err)
	}

	// Copy hooks configuration if it exists (for supervisor and merge-queue)
	if err := c.copyHooksConfig(repoPath, repoPath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config: %v\n", err)
	}

	// Start Claude in supervisor window (skip in test mode)
	var supervisorPID, mergeQueuePID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		fmt.Println("Starting Claude Code in supervisor window...")
		pid, err := c.startClaudeInTmux(tmuxSession, "supervisor", repoPath, supervisorSessionID, supervisorPromptFile, "")
		if err != nil {
			return fmt.Errorf("failed to start supervisor Claude: %w", err)
		}
		supervisorPID = pid

		// Start Claude in merge-queue window
		fmt.Println("Starting Claude Code in merge-queue window...")
		pid, err = c.startClaudeInTmux(tmuxSession, "merge-queue", repoPath, mergeQueueSessionID, mergeQueuePromptFile, "")
		if err != nil {
			return fmt.Errorf("failed to start merge-queue Claude: %w", err)
		}
		mergeQueuePID = pid
	}

	// Add repository to daemon state
	resp, err := client.Send(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":         repoName,
			"github_url":   githubURL,
			"tmux_session": tmuxSession,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register repository with daemon: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register repository: %s", resp.Error)
	}

	// Add supervisor agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         "supervisor",
			"type":          "supervisor",
			"worktree_path": repoPath,
			"tmux_window":   "supervisor",
			"session_id":    supervisorSessionID,
			"pid":           supervisorPID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register supervisor: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register supervisor: %s", resp.Error)
	}

	// Add merge-queue agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         "merge-queue",
			"type":          "merge-queue",
			"worktree_path": repoPath,
			"tmux_window":   "merge-queue",
			"session_id":    mergeQueueSessionID,
			"pid":           mergeQueuePID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register merge-queue: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register merge-queue: %s", resp.Error)
	}

	// Create workspace worktree
	wt := worktree.NewManager(repoPath)
	workspacePath := c.paths.AgentWorktree(repoName, "workspace")
	workspaceBranch := "workspace"

	fmt.Printf("Creating workspace worktree at: %s\n", workspacePath)
	if err := wt.CreateNewBranch(workspacePath, workspaceBranch, "HEAD"); err != nil {
		return fmt.Errorf("failed to create workspace worktree: %w", err)
	}

	// Create workspace tmux window (detached so it doesn't switch focus)
	cmd = exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", "workspace", "-c", workspacePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create workspace window: %w", err)
	}

	// Generate session ID for workspace
	workspaceSessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate workspace session ID: %w", err)
	}

	// Write prompt file for workspace
	workspacePromptFile, err := c.writePromptFile(repoPath, prompts.TypeWorkspace, "workspace")
	if err != nil {
		return fmt.Errorf("failed to write workspace prompt: %w", err)
	}

	// Copy hooks configuration if it exists
	if err := c.copyHooksConfig(repoPath, workspacePath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config to workspace: %v\n", err)
	}

	// Start Claude in workspace window (skip in test mode)
	var workspacePID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		fmt.Println("Starting Claude Code in workspace window...")
		pid, err := c.startClaudeInTmux(tmuxSession, "workspace", workspacePath, workspaceSessionID, workspacePromptFile, "")
		if err != nil {
			return fmt.Errorf("failed to start workspace Claude: %w", err)
		}
		workspacePID = pid
	}

	// Add workspace agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         "workspace",
			"type":          "workspace",
			"worktree_path": workspacePath,
			"tmux_window":   "workspace",
			"session_id":    workspaceSessionID,
			"pid":           workspacePID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register workspace: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register workspace: %s", resp.Error)
	}

	fmt.Println()
	fmt.Println("✓ Repository initialized successfully!")
	fmt.Printf("  Tmux session: %s\n", tmuxSession)
	fmt.Printf("  Agents: supervisor, merge-queue, workspace\n")
	fmt.Printf("\nAttach to session: tmux attach -t %s\n", tmuxSession)
	fmt.Printf("Or attach to your workspace: multiclaude attach workspace\n")

	return nil
}

func (c *CLI) listRepos(args []string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_repos",
		Args: map[string]interface{}{
			"rich": true,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("listing repositories", err)
	}

	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to list repos", fmt.Errorf("%s", resp.Error))
	}

	repos, ok := resp.Data.([]interface{})
	if !ok {
		return errors.New(errors.CategoryRuntime, "unexpected response format from daemon")
	}

	if len(repos) == 0 {
		fmt.Println("No repositories tracked")
		format.Dimmed("\nInitialize a repository with: multiclaude init <github-url>")
		return nil
	}

	format.Header("Tracked repositories (%d):", len(repos))
	fmt.Println()

	table := format.NewColoredTable("REPO", "AGENTS", "STATUS", "SESSION")
	for _, repo := range repos {
		if repoMap, ok := repo.(map[string]interface{}); ok {
			name, _ := repoMap["name"].(string)
			totalAgents := 0
			if v, ok := repoMap["total_agents"].(float64); ok {
				totalAgents = int(v)
			}
			workerCount := 0
			if v, ok := repoMap["worker_count"].(float64); ok {
				workerCount = int(v)
			}
			sessionHealthy, _ := repoMap["session_healthy"].(bool)
			tmuxSession, _ := repoMap["tmux_session"].(string)

			// Format agent count
			agentStr := fmt.Sprintf("%d total", totalAgents)
			if workerCount > 0 {
				agentStr = fmt.Sprintf("%d (%d workers)", totalAgents, workerCount)
			}

			// Format status
			var statusCell format.ColoredCell
			if sessionHealthy {
				statusCell = format.ColorCell(format.ColoredStatus(format.StatusHealthy), nil)
			} else {
				statusCell = format.ColorCell(format.ColoredStatus(format.StatusError), nil)
			}

			table.AddRow(
				format.Cell(name),
				format.Cell(agentStr),
				statusCell,
				format.ColorCell(tmuxSession, format.Dim),
			)
		}
	}
	table.Print()

	return nil
}

func (c *CLI) createWorker(args []string) error {
	flags, posArgs := ParseFlags(args)

	// Get task description
	task := strings.Join(posArgs, " ")
	if task == "" {
		return errors.InvalidUsage("usage: multiclaude work <task description>")
	}

	// Determine repository (from flag or current directory)
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		// Check if we're in a tracked repo
		repos := c.getReposList()
		for _, repo := range repos {
			repoPath := c.paths.RepoDir(repo)
			if strings.HasPrefix(cwd, repoPath) {
				repoName = repo
				break
			}
		}

		if repoName == "" {
			return errors.NotInRepo()
		}
	}

	// Generate worker name (Docker-style)
	workerName := names.Generate()
	if name, ok := flags["name"]; ok {
		workerName = name
	}

	// Determine branch to start from
	startBranch := "HEAD" // Default to current branch/HEAD
	if branch, ok := flags["branch"]; ok {
		startBranch = branch
		fmt.Printf("Creating worker '%s' in repo '%s' from branch '%s'\n", workerName, repoName, branch)
	} else {
		fmt.Printf("Creating worker '%s' in repo '%s'\n", workerName, repoName)
	}
	fmt.Printf("Task: %s\n", task)

	// Get repository path
	repoPath := c.paths.RepoDir(repoName)

	// Create worktree
	wt := worktree.NewManager(repoPath)
	wtPath := c.paths.AgentWorktree(repoName, workerName)
	branchName := fmt.Sprintf("work/%s", workerName)

	fmt.Printf("Creating worktree at: %s\n", wtPath)
	if err := wt.CreateNewBranch(wtPath, branchName, startBranch); err != nil {
		return errors.WorktreeCreationFailed(err)
	}

	// Get repository info to determine tmux session
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("getting repo info", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to get repo info", fmt.Errorf("%s", resp.Error))
	}

	// Get tmux session name (it's mc-<reponame>)
	tmuxSession := fmt.Sprintf("mc-%s", repoName)

	// Create tmux window for worker (detached so it doesn't switch focus)
	fmt.Printf("Creating tmux window: %s\n", workerName)
	cmd := exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", workerName, "-c", wtPath)
	if err := cmd.Run(); err != nil {
		return errors.TmuxOperationFailed("create window", err)
	}

	// Generate session ID for worker
	workerSessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate worker session ID: %w", err)
	}

	// Write prompt file for worker
	workerPromptFile, err := c.writePromptFile(repoPath, prompts.TypeWorker, workerName)
	if err != nil {
		return fmt.Errorf("failed to write worker prompt: %w", err)
	}

	// Copy hooks configuration if it exists
	if err := c.copyHooksConfig(repoPath, wtPath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config: %v\n", err)
	}

	// Start Claude in worker window with initial task (skip in test mode)
	var workerPID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		fmt.Println("Starting Claude Code in worker window...")
		initialMessage := fmt.Sprintf("Task: %s", task)
		pid, err := c.startClaudeInTmux(tmuxSession, workerName, wtPath, workerSessionID, workerPromptFile, initialMessage)
		if err != nil {
			return fmt.Errorf("failed to start worker Claude: %w", err)
		}
		workerPID = pid
	}

	// Register worker with daemon
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         workerName,
			"type":          "worker",
			"worktree_path": wtPath,
			"tmux_window":   workerName,
			"task":          task,
			"session_id":    workerSessionID,
			"pid":           workerPID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register worker: %s", resp.Error)
	}

	fmt.Println()
	fmt.Println("✓ Worker created successfully!")
	fmt.Printf("  Name: %s\n", workerName)
	fmt.Printf("  Branch: %s\n", branchName)
	fmt.Printf("  Worktree: %s\n", wtPath)
	fmt.Printf("\nAttach to worker: tmux select-window -t %s:%s\n", tmuxSession, workerName)
	fmt.Printf("Or use: multiclaude attach %s\n", workerName)

	return nil
}

func (c *CLI) listWorkers(args []string) error {
	flags, _ := ParseFlags(args)

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// List all repos
		repos := c.getReposList()
		if len(repos) == 0 {
			fmt.Println("No repositories tracked")
			return nil
		}

		// If only one repo, use it
		if len(repos) == 1 {
			repoName = repos[0]
		} else {
			return errors.MultipleRepos()
		}
	}

	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
			"rich": true,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("listing workers", err)
	}

	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to list workers", fmt.Errorf("%s", resp.Error))
	}

	agents, ok := resp.Data.([]interface{})
	if !ok {
		return errors.New(errors.CategoryRuntime, "unexpected response format from daemon")
	}

	// Filter for workers and workspace
	workers := []map[string]interface{}{}
	var workspace map[string]interface{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			if agentType == "worker" {
				workers = append(workers, agentMap)
			} else if agentType == "workspace" {
				workspace = agentMap
			}
		}
	}

	// Show workspace first if it exists
	if workspace != nil {
		format.Header("Workspace in '%s':", repoName)
		status, _ := workspace["status"].(string)
		var statusCell format.ColoredCell
		switch status {
		case "running":
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusRunning), nil)
		case "completed":
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusCompleted), nil)
		default:
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusIdle), nil)
		}
		fmt.Printf("  workspace ")
		fmt.Print(statusCell.Text)
		fmt.Println()
		fmt.Println()
	}

	if len(workers) == 0 {
		fmt.Printf("No workers in repository '%s'\n", repoName)
		format.Dimmed("\nCreate a worker with: multiclaude work <task>")
		return nil
	}

	format.Header("Workers in '%s' (%d):", repoName, len(workers))
	fmt.Println()

	table := format.NewColoredTable("NAME", "STATUS", "BRANCH", "MSGS", "TASK")
	for _, worker := range workers {
		name, _ := worker["name"].(string)
		task, _ := worker["task"].(string)
		status, _ := worker["status"].(string)
		branch, _ := worker["branch"].(string)
		msgsTotal := 0
		if v, ok := worker["messages_total"].(float64); ok {
			msgsTotal = int(v)
		}
		msgsPending := 0
		if v, ok := worker["messages_pending"].(float64); ok {
			msgsPending = int(v)
		}

		// Format status with color
		var statusCell format.ColoredCell
		switch status {
		case "running":
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusRunning), nil)
		case "completed":
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusCompleted), nil)
		case "stopped":
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusError), nil)
		default:
			statusCell = format.ColorCell(format.ColoredStatus(format.StatusIdle), nil)
		}

		// Format branch
		branchCell := format.ColorCell(branch, format.Cyan)
		if branch == "" {
			branchCell = format.ColorCell("-", format.Dim)
		}

		// Format message count
		msgStr := format.MessageBadge(msgsPending, msgsTotal)

		// Truncate task
		truncTask := format.Truncate(task, 40)

		table.AddRow(
			format.Cell(name),
			statusCell,
			branchCell,
			format.Cell(msgStr),
			format.Cell(truncTask),
		)
	}
	table.Print()

	return nil
}

func (c *CLI) removeWorker(args []string) error {
	if len(args) < 1 {
		return errors.InvalidUsage("usage: multiclaude work rm <worker-name>")
	}

	workerName := args[0]

	// Determine repository
	flags, _ := ParseFlags(args[1:])
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from tracked repos
		repos := c.getReposList()
		if len(repos) == 1 {
			repoName = repos[0]
		} else {
			return errors.MultipleRepos()
		}
	}

	fmt.Printf("Removing worker '%s' from repo '%s'\n", workerName, repoName)

	// Get worker info
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("getting worker info", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to get worker info", fmt.Errorf("%s", resp.Error))
	}

	// Find worker
	agents, _ := resp.Data.([]interface{})
	var workerInfo map[string]interface{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			if name, _ := agentMap["name"].(string); name == workerName {
				workerInfo = agentMap
				break
			}
		}
	}

	if workerInfo == nil {
		return errors.AgentNotFound("worker", workerName, repoName)
	}

	// Get worktree path
	wtPath := workerInfo["worktree_path"].(string)

	// Check for uncommitted changes
	hasUncommitted, err := worktree.HasUncommittedChanges(wtPath)
	if err != nil {
		fmt.Printf("Warning: failed to check for uncommitted changes: %v\n", err)
	} else if hasUncommitted {
		fmt.Println("\nWarning: Worker has uncommitted changes!")
		fmt.Println("Files may be lost if you continue with cleanup.")
		fmt.Print("Continue with cleanup? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Check for unpushed commits
	hasUnpushed, err := worktree.HasUnpushedCommits(wtPath)
	if err != nil {
		// This is ok - might not have a tracking branch
		fmt.Printf("Note: Could not check for unpushed commits (no tracking branch?)\n")
	} else if hasUnpushed {
		fmt.Println("\nWarning: Worker has unpushed commits!")
		branch, err := worktree.GetCurrentBranch(wtPath)
		if err == nil {
			fmt.Printf("Branch '%s' has commits not pushed to remote.\n", branch)
		}
		fmt.Println("These commits may be lost if you continue with cleanup.")
		fmt.Print("Continue with cleanup? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Kill tmux window
	tmuxSession := fmt.Sprintf("mc-%s", repoName)
	tmuxWindow := workerInfo["tmux_window"].(string)
	fmt.Printf("Killing tmux window: %s\n", tmuxWindow)
	cmd := exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to kill tmux window: %v\n", err)
	}

	// Remove worktree
	repoPath := c.paths.RepoDir(repoName)
	wt := worktree.NewManager(repoPath)

	fmt.Printf("Removing worktree: %s\n", wtPath)
	if err := wt.Remove(wtPath, false); err != nil {
		fmt.Printf("Warning: failed to remove worktree: %v\n", err)
	}

	// Unregister from daemon
	resp, err = client.Send(socket.Request{
		Command: "remove_agent",
		Args: map[string]interface{}{
			"repo":  repoName,
			"agent": workerName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to unregister worker: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to unregister worker: %s", resp.Error)
	}

	fmt.Println("✓ Worker removed successfully")
	return nil
}

// getReposList is a helper to get the list of repos
func (c *CLI) getReposList() []string {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "list_repos"})
	if err != nil {
		return []string{}
	}

	if !resp.Success {
		return []string{}
	}

	repos, ok := resp.Data.([]interface{})
	if !ok {
		return []string{}
	}

	result := make([]string, 0, len(repos))
	for _, repo := range repos {
		if repoStr, ok := repo.(string); ok {
			result = append(result, repoStr)
		}
	}

	return result
}

func (c *CLI) sendMessage(args []string) error {
	if len(args) < 2 {
		return errors.InvalidUsage("usage: multiclaude agent send-message <to> <message>")
	}

	to := args[0]
	body := strings.Join(args[1:], " ")

	// Determine current agent and repo
	repoName, agentName, err := c.inferAgentContext()
	if err != nil {
		return err
	}

	// Create message manager
	msgMgr := messages.NewManager(c.paths.MessagesDir)

	// Send message
	msg, err := msgMgr.Send(repoName, agentName, to, body)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	fmt.Printf("Message sent to %s (ID: %s)\n", to, msg.ID)
	return nil
}

func (c *CLI) listMessages(args []string) error {
	// Determine current agent and repo
	repoName, agentName, err := c.inferAgentContext()
	if err != nil {
		return err
	}

	msgMgr := messages.NewManager(c.paths.MessagesDir)

	// List messages
	msgs, err := msgMgr.List(repoName, agentName)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	if len(msgs) == 0 {
		fmt.Println("No messages")
		return nil
	}

	fmt.Printf("Messages for %s (%d):\n", agentName, len(msgs))
	for _, msg := range msgs {
		status := msg.Status
		if msg.Status == messages.StatusAcked && msg.AckedAt != nil {
			status = messages.Status(fmt.Sprintf("acked (%s)", formatTime(*msg.AckedAt)))
		}
		fmt.Printf("  [%s] %s - From: %s - %s - %s\n",
			msg.ID,
			formatTime(msg.Timestamp),
			msg.From,
			status,
			truncateString(msg.Body, 60))
	}

	return nil
}

func (c *CLI) readMessage(args []string) error {
	if len(args) < 1 {
		return errors.InvalidUsage("usage: multiclaude agent read-message <message-id>")
	}

	messageID := args[0]

	// Determine current agent and repo
	repoName, agentName, err := c.inferAgentContext()
	if err != nil {
		return err
	}

	msgMgr := messages.NewManager(c.paths.MessagesDir)

	// Get message
	msg, err := msgMgr.Get(repoName, agentName, messageID)
	if err != nil {
		return fmt.Errorf("failed to read message: %w", err)
	}

	// Update status to read
	if msg.Status == messages.StatusPending || msg.Status == messages.StatusDelivered {
		if err := msgMgr.UpdateStatus(repoName, agentName, messageID, messages.StatusRead); err != nil {
			fmt.Printf("Warning: failed to update message status: %v\n", err)
		}
	}

	// Display message
	fmt.Printf("Message: %s\n", msg.ID)
	fmt.Printf("From: %s\n", msg.From)
	fmt.Printf("To: %s\n", msg.To)
	fmt.Printf("Time: %s\n", msg.Timestamp.Format(time.RFC3339))
	fmt.Printf("Status: %s\n", msg.Status)
	if msg.AckedAt != nil {
		fmt.Printf("Acked: %s\n", msg.AckedAt.Format(time.RFC3339))
	}
	fmt.Println()
	fmt.Println(msg.Body)

	return nil
}

func (c *CLI) ackMessage(args []string) error {
	if len(args) < 1 {
		return errors.InvalidUsage("usage: multiclaude agent ack-message <message-id>")
	}

	messageID := args[0]

	// Determine current agent and repo
	repoName, agentName, err := c.inferAgentContext()
	if err != nil {
		return err
	}

	msgMgr := messages.NewManager(c.paths.MessagesDir)

	// Ack message
	if err := msgMgr.Ack(repoName, agentName, messageID); err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}

	fmt.Printf("Message %s acknowledged\n", messageID)
	return nil
}

// inferAgentContext infers the current agent and repo from working directory
func (c *CLI) inferAgentContext() (repoName, agentName string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if we're in a worktree path
	// Path format: ~/.multiclaude/wts/<repo>/<agent>
	if strings.Contains(cwd, c.paths.WorktreesDir) {
		// Extract repo and agent from path
		rel, err := filepath.Rel(c.paths.WorktreesDir, cwd)
		if err == nil {
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) >= 2 {
				return parts[0], parts[1], nil
			}
			if len(parts) == 1 {
				// We're in the repo worktree dir itself
				return parts[0], "", fmt.Errorf("cannot determine agent - in repo worktree directory")
			}
		}
	}

	// Check if we're in a main repo path
	// Path format: ~/.multiclaude/repos/<repo>
	if strings.Contains(cwd, c.paths.ReposDir) {
		rel, err := filepath.Rel(c.paths.ReposDir, cwd)
		if err == nil {
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) >= 1 {
				// In main repo - could be supervisor or merge-queue
				// Try to get tmux window name
				tmuxWindow := os.Getenv("TMUX_PANE")
				if tmuxWindow != "" {
					// Get window name from tmux
					cmd := exec.Command("tmux", "display-message", "-p", "#{window_name}")
					output, err := cmd.Output()
					if err == nil {
						windowName := strings.TrimSpace(string(output))
						return parts[0], windowName, nil
					}
				}

				// Fallback: assume supervisor
				return parts[0], "supervisor", nil
			}
		}
	}

	return "", "", errors.NotInAgentContext()
}

// Helper functions

func formatTime(t time.Time) string {
	if time.Since(t) < 24*time.Hour {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (c *CLI) completeWorker(args []string) error {
	// Determine current agent and repo
	repoName, agentName, err := c.inferAgentContext()
	if err != nil {
		return fmt.Errorf("failed to determine agent context: %w", err)
	}

	fmt.Printf("Marking agent '%s' as complete...\n", agentName)

	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"repo":  repoName,
			"agent": agentName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("marking agent complete", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to mark agent complete", fmt.Errorf("%s", resp.Error))
	}

	fmt.Println("✓ Agent marked as complete")
	fmt.Println("The daemon will clean up this agent's resources shortly.")
	return nil
}

func (c *CLI) reviewPR(args []string) error {
	if len(args) < 1 {
		return errors.InvalidUsage("usage: multiclaude review <pr-url>")
	}

	prURL := args[0]

	// Parse PR URL to extract owner, repo, and PR number
	// Expected formats:
	// - https://github.com/owner/repo/pull/123
	// - github.com/owner/repo/pull/123
	prURL = strings.TrimPrefix(prURL, "https://")
	prURL = strings.TrimPrefix(prURL, "http://")
	parts := strings.Split(prURL, "/")

	if len(parts) < 5 || parts[3] != "pull" {
		return errors.InvalidPRURL()
	}

	prNumber := parts[4]
	fmt.Printf("Reviewing PR #%s\n", prNumber)

	// Determine repository from flag or current directory
	flags, _ := ParseFlags(args[1:])
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		// Check if we're in a tracked repo
		repos := c.getReposList()
		for _, repo := range repos {
			repoPath := c.paths.RepoDir(repo)
			if strings.HasPrefix(cwd, repoPath) {
				repoName = repo
				break
			}
		}

		if repoName == "" {
			return errors.NotInRepo()
		}
	}

	// Generate review agent name
	reviewerName := fmt.Sprintf("review-%s", prNumber)

	fmt.Printf("Creating review agent '%s' in repo '%s'\n", reviewerName, repoName)

	// Get repository path
	repoPath := c.paths.RepoDir(repoName)

	// Get the PR branch name using gh CLI
	fmt.Printf("Fetching PR branch information...\n")
	cmd := exec.Command("gh", "pr", "view", prNumber, "--repo", fmt.Sprintf("%s/%s", parts[1], parts[2]), "--json", "headRefName", "-q", ".headRefName")
	cmd.Dir = repoPath
	branchOutput, err := cmd.Output()
	if err != nil {
		return errors.Wrap(errors.CategoryRuntime, "failed to get PR branch info", err).WithSuggestion("ensure 'gh' CLI is installed and authenticated: gh auth login")
	}
	prBranch := strings.TrimSpace(string(branchOutput))
	if prBranch == "" {
		return errors.New(errors.CategoryRuntime, "could not determine PR branch name - the PR may not exist or be accessible")
	}

	fmt.Printf("PR branch: %s\n", prBranch)

	// Fetch the PR branch
	cmd = exec.Command("git", "fetch", "origin", prBranch)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return errors.GitOperationFailed("fetch", err)
	}

	// Create worktree for review
	wt := worktree.NewManager(repoPath)
	wtPath := c.paths.AgentWorktree(repoName, reviewerName)
	reviewBranch := fmt.Sprintf("review/%s", reviewerName)

	fmt.Printf("Creating worktree at: %s\n", wtPath)
	if err := wt.CreateNewBranch(wtPath, reviewBranch, fmt.Sprintf("origin/%s", prBranch)); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Get tmux session name
	tmuxSession := fmt.Sprintf("mc-%s", repoName)

	// Create tmux window for reviewer (detached so it doesn't switch focus)
	fmt.Printf("Creating tmux window: %s\n", reviewerName)
	cmd = exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", reviewerName, "-c", wtPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Generate session ID for reviewer
	reviewerSessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate reviewer session ID: %w", err)
	}

	// Write prompt file for reviewer
	reviewerPromptFile, err := c.writePromptFile(repoPath, prompts.TypeReview, reviewerName)
	if err != nil {
		return fmt.Errorf("failed to write reviewer prompt: %w", err)
	}

	// Copy hooks configuration if it exists
	if err := c.copyHooksConfig(repoPath, wtPath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config: %v\n", err)
	}

	// Start Claude in reviewer window with initial task (skip in test mode)
	var reviewerPID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		fmt.Println("Starting Claude Code in reviewer window...")
		initialMessage := fmt.Sprintf("Review PR #%s: https://github.com/%s/%s/pull/%s", prNumber, parts[1], parts[2], prNumber)
		pid, err := c.startClaudeInTmux(tmuxSession, reviewerName, wtPath, reviewerSessionID, reviewerPromptFile, initialMessage)
		if err != nil {
			return fmt.Errorf("failed to start reviewer Claude: %w", err)
		}
		reviewerPID = pid
	}

	// Register reviewer with daemon
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         reviewerName,
			"type":          "review",
			"worktree_path": wtPath,
			"tmux_window":   reviewerName,
			"task":          fmt.Sprintf("Review PR #%s", prNumber),
			"session_id":    reviewerSessionID,
			"pid":           reviewerPID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register reviewer: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register reviewer: %s", resp.Error)
	}

	fmt.Println()
	fmt.Println("✓ Review agent created successfully!")
	fmt.Printf("  Name: %s\n", reviewerName)
	fmt.Printf("  Branch: %s\n", reviewBranch)
	fmt.Printf("  Worktree: %s\n", wtPath)
	fmt.Printf("\nAttach to reviewer: tmux select-window -t %s:%s\n", tmuxSession, reviewerName)
	fmt.Printf("Or use: multiclaude attach %s\n", reviewerName)

	return nil
}

func (c *CLI) attachAgent(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: multiclaude attach <agent-name> [--read-only]")
	}

	agentName := args[0]
	flags, _ := ParseFlags(args[1:])
	readOnly := flags["read-only"] == "true" || flags["r"] == "true"

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from tracked repos
		repos := c.getReposList()
		if len(repos) == 0 {
			return fmt.Errorf("no repositories tracked")
		}
		if len(repos) == 1 {
			repoName = repos[0]
		} else {
			return errors.MultipleRepos()
		}
	}

	// Get agent info to find tmux session and window
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get agent info: %w (is daemon running?)", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to get agent info: %s", resp.Error)
	}

	// Find agent
	agents, _ := resp.Data.([]interface{})
	var agentInfo map[string]interface{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			if name, _ := agentMap["name"].(string); name == agentName {
				agentInfo = agentMap
				break
			}
		}
	}

	if agentInfo == nil {
		return fmt.Errorf("agent '%s' not found in repo '%s'", agentName, repoName)
	}

	// Get tmux session and window
	tmuxSession := fmt.Sprintf("mc-%s", repoName)
	tmuxWindow := agentInfo["tmux_window"].(string)

	// Attach to tmux
	target := fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)

	tmuxArgs := []string{"attach", "-t", target}
	if readOnly {
		tmuxArgs = append(tmuxArgs, "-r")
	}

	cmd := exec.Command("tmux", tmuxArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (c *CLI) cleanup(args []string) error {
	flags, _ := ParseFlags(args)
	dryRun := flags["dry-run"] == "true"
	verbose := flags["verbose"] == "true" || flags["v"] == "true"

	if dryRun {
		fmt.Println("Running cleanup in dry-run mode (no changes will be made)...")
	} else {
		fmt.Println("Running cleanup...")
	}

	client := socket.NewClient(c.paths.DaemonSock)

	// Check if daemon is running
	_, err := client.Send(socket.Request{Command: "ping"})
	if err != nil {
		fmt.Println("Daemon is not running. Running local cleanup...")
		return c.localCleanup(dryRun, verbose)
	}

	// Trigger daemon cleanup
	resp, err := client.Send(socket.Request{
		Command: "trigger_cleanup",
		Args: map[string]interface{}{
			"dry_run": dryRun,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to trigger cleanup: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("cleanup failed: %s", resp.Error)
	}

	fmt.Println("✓ Cleanup completed")
	return nil
}

func (c *CLI) localCleanup(dryRun bool, verbose bool) error {
	// Clean up orphaned worktrees, tmux sessions, and other resources
	fmt.Println("\nChecking for orphaned resources...")

	totalRemoved := 0
	totalIssues := 0

	// Load state for reference
	st, err := state.Load(c.paths.StateFile)
	if err != nil {
		fmt.Printf("Warning: could not load state file: %v\n", err)
		st = state.New(c.paths.StateFile)
	}

	// Check for orphaned tmux sessions (mc-* sessions not in state)
	tmuxClient := tmux.NewClient()
	if tmuxClient.IsTmuxAvailable() {
		sessions, err := tmuxClient.ListSessions()
		if err == nil {
			repos := st.ListRepos()
			validSessions := make(map[string]bool)
			for _, repo := range repos {
				validSessions[fmt.Sprintf("mc-%s", repo)] = true
			}

			orphanedSessions := []string{}
			for _, session := range sessions {
				if strings.HasPrefix(session, "mc-") && !validSessions[session] {
					orphanedSessions = append(orphanedSessions, session)
				}
			}

			if len(orphanedSessions) > 0 {
				fmt.Printf("\nOrphaned tmux sessions (%d):\n", len(orphanedSessions))
				for _, session := range orphanedSessions {
					if dryRun {
						fmt.Printf("  Would kill: %s\n", session)
					} else {
						if err := tmuxClient.KillSession(session); err != nil {
							fmt.Printf("  Failed to kill %s: %v\n", session, err)
						} else {
							fmt.Printf("  Killed: %s\n", session)
							totalRemoved++
						}
					}
				}
			} else if verbose {
				fmt.Println("\nNo orphaned tmux sessions found")
			}
		}
	}

	// Check for orphaned worktree directories (in wts/ but not in any repo's git worktrees)
	entries, err := os.ReadDir(c.paths.WorktreesDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to read worktrees directory: %v\n", err)
	} else if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			repoName := entry.Name()
			repoPath := c.paths.RepoDir(repoName)
			wtRootDir := c.paths.WorktreeDir(repoName)

			// Check if the repo still exists
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				fmt.Printf("\nOrphaned worktree directory (repo missing): %s\n", wtRootDir)
				if !dryRun {
					if err := os.RemoveAll(wtRootDir); err != nil {
						fmt.Printf("  Failed to remove: %v\n", err)
					} else {
						fmt.Printf("  Removed\n")
						totalRemoved++
					}
				}
				continue
			}

			if verbose {
				fmt.Printf("\nRepository: %s\n", repoName)
			}

			wt := worktree.NewManager(repoPath)

			// Cleanup orphaned worktree directories
			if !dryRun {
				removed, err := worktree.CleanupOrphaned(wtRootDir, wt)
				if err != nil {
					fmt.Printf("  Warning: failed to cleanup worktrees: %v\n", err)
				} else if len(removed) > 0 {
					for _, path := range removed {
						fmt.Printf("  Removed: %s\n", path)
					}
					totalRemoved += len(removed)
				} else if verbose {
					fmt.Println("  No orphaned worktrees")
				}
			} else {
				// Dry run: just check what would be removed
				gitWorktrees, _ := wt.List()
				gitPaths := make(map[string]bool)
				for _, gwt := range gitWorktrees {
					absPath, _ := filepath.Abs(gwt.Path)
					evalPath, err := filepath.EvalSymlinks(absPath)
					if err != nil {
						evalPath = absPath
					}
					gitPaths[evalPath] = true
				}

				dirEntries, _ := os.ReadDir(wtRootDir)
				for _, de := range dirEntries {
					if !de.IsDir() {
						continue
					}
					path := filepath.Join(wtRootDir, de.Name())
					absPath, _ := filepath.Abs(path)
					evalPath, err := filepath.EvalSymlinks(absPath)
					if err != nil {
						evalPath = absPath
					}
					if !gitPaths[evalPath] {
						fmt.Printf("  Would remove: %s\n", path)
						totalIssues++
					}
				}
			}

			// Prune git worktree references
			if !dryRun {
				if err := wt.Prune(); err != nil && verbose {
					fmt.Printf("  Warning: failed to prune worktrees: %v\n", err)
				}
			}
		}
	}

	// Check for orphaned message directories
	msgEntries, err := os.ReadDir(c.paths.MessagesDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to read messages directory: %v\n", err)
	} else if err == nil {
		for _, entry := range msgEntries {
			if !entry.IsDir() {
				continue
			}

			repoName := entry.Name()
			validAgents, _ := st.ListAgents(repoName)

			msgMgr := messages.NewManager(c.paths.MessagesDir)

			if !dryRun {
				count, err := msgMgr.CleanupOrphaned(repoName, validAgents)
				if err != nil && verbose {
					fmt.Printf("Warning: failed to cleanup messages for %s: %v\n", repoName, err)
				} else if count > 0 {
					fmt.Printf("Cleaned up %d orphaned message dir(s) for %s\n", count, repoName)
					totalRemoved += count
				}
			} else {
				// Dry run check
				repoDir := filepath.Join(c.paths.MessagesDir, repoName)
				agentEntries, _ := os.ReadDir(repoDir)
				validAgentMap := make(map[string]bool)
				for _, a := range validAgents {
					validAgentMap[a] = true
				}
				for _, ae := range agentEntries {
					if ae.IsDir() && !validAgentMap[ae.Name()] {
						fmt.Printf("Would remove orphaned message dir: %s/%s\n", repoName, ae.Name())
						totalIssues++
					}
				}
			}
		}
	}

	// Check for stale socket and PID files (when daemon not running)
	pidFile := daemon.NewPIDFile(c.paths.DaemonPID)
	if running, _, _ := pidFile.IsRunning(); !running {
		// Daemon not running, check for stale files
		if _, err := os.Stat(c.paths.DaemonPID); err == nil {
			if dryRun {
				fmt.Printf("\nWould remove stale PID file: %s\n", c.paths.DaemonPID)
				totalIssues++
			} else {
				if err := os.Remove(c.paths.DaemonPID); err == nil {
					fmt.Printf("Removed stale PID file: %s\n", c.paths.DaemonPID)
					totalRemoved++
				}
			}
		}
		if _, err := os.Stat(c.paths.DaemonSock); err == nil {
			if dryRun {
				fmt.Printf("Would remove stale socket file: %s\n", c.paths.DaemonSock)
				totalIssues++
			} else {
				if err := os.Remove(c.paths.DaemonSock); err == nil {
					fmt.Printf("Removed stale socket file: %s\n", c.paths.DaemonSock)
					totalRemoved++
				}
			}
		}
	}

	fmt.Println()
	if dryRun {
		if totalIssues > 0 {
			fmt.Printf("✓ Dry run completed: would fix %d issue(s)\n", totalIssues)
		} else {
			fmt.Println("✓ Dry run completed: no issues found")
		}
	} else {
		if totalRemoved > 0 {
			fmt.Printf("✓ Cleanup completed: removed %d item(s)\n", totalRemoved)
		} else {
			fmt.Println("✓ Cleanup completed: no orphaned resources found")
		}
	}

	return nil
}

func (c *CLI) repair(args []string) error {
	flags, _ := ParseFlags(args)
	verbose := flags["verbose"] == "true" || flags["v"] == "true"

	fmt.Println("Repairing state...")

	// Check if daemon is running
	client := socket.NewClient(c.paths.DaemonSock)
	_, err := client.Send(socket.Request{Command: "ping"})
	if err != nil {
		// Daemon not running - do local repair
		fmt.Println("Daemon is not running. Performing local repair...")
		return c.localRepair(verbose)
	}

	// Trigger state repair via daemon
	resp, err := client.Send(socket.Request{
		Command: "repair_state",
	})
	if err != nil {
		return fmt.Errorf("failed to trigger repair: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("repair failed: %s", resp.Error)
	}

	fmt.Println("✓ State repaired successfully")
	if data, ok := resp.Data.(map[string]interface{}); ok {
		if removed, ok := data["agents_removed"].(float64); ok && removed > 0 {
			fmt.Printf("  Removed %d dead agent(s)\n", int(removed))
		}
		if fixed, ok := data["issues_fixed"].(float64); ok && fixed > 0 {
			fmt.Printf("  Fixed %d issue(s)\n", int(fixed))
		}
	}

	return nil
}

// localRepair performs state repair without the daemon running
func (c *CLI) localRepair(verbose bool) error {
	// Load state from disk
	st, err := state.Load(c.paths.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	tmuxClient := tmux.NewClient()
	agentsRemoved := 0
	issuesFixed := 0

	// Track orphaned tmux sessions
	orphanedSessions := []string{}

	// Get all tmux sessions and find orphaned ones
	sessions, err := tmuxClient.ListSessions()
	if err == nil {
		repos := st.ListRepos()
		validSessions := make(map[string]bool)
		for _, repo := range repos {
			validSessions[fmt.Sprintf("mc-%s", repo)] = true
		}
		for _, session := range sessions {
			if strings.HasPrefix(session, "mc-") && !validSessions[session] {
				orphanedSessions = append(orphanedSessions, session)
			}
		}
	}

	// Check each repo and its agents
	repos := st.GetAllRepos()
	for repoName, repo := range repos {
		if verbose {
			fmt.Printf("\nChecking repository: %s\n", repoName)
		}

		// Check if tmux session exists
		hasSession, err := tmuxClient.HasSession(repo.TmuxSession)
		if err != nil && verbose {
			fmt.Printf("  Warning: failed to check session %s: %v\n", repo.TmuxSession, err)
			continue
		}

		if !hasSession {
			if verbose {
				fmt.Printf("  Tmux session %s not found\n", repo.TmuxSession)
			}
			// Remove all agents for this repo
			for agentName := range repo.Agents {
				if verbose {
					fmt.Printf("  Removing agent %s (session gone)\n", agentName)
				}
				if err := st.RemoveAgent(repoName, agentName); err == nil {
					agentsRemoved++
				}
			}
			issuesFixed++
			continue
		}

		// Check each agent
		for agentName, agent := range repo.Agents {
			// Check if window exists
			hasWindow, _ := tmuxClient.HasWindow(repo.TmuxSession, agent.TmuxWindow)
			if !hasWindow {
				if verbose {
					fmt.Printf("  Removing agent %s (window %s not found)\n", agentName, agent.TmuxWindow)
				}
				if err := st.RemoveAgent(repoName, agentName); err == nil {
					agentsRemoved++
					issuesFixed++
				}
				continue
			}

			// Check if worktree exists (for workers)
			if agent.Type == state.AgentTypeWorker && agent.WorktreePath != "" {
				if _, err := os.Stat(agent.WorktreePath); os.IsNotExist(err) {
					if verbose {
						fmt.Printf("  Warning: worktree missing for %s: %s\n", agentName, agent.WorktreePath)
					}
					// Don't remove - window exists, user may have manually deleted worktree
				}
			}

			if verbose {
				fmt.Printf("  Agent %s: OK\n", agentName)
			}
		}
	}

	// Clean up orphaned worktrees
	for _, repoName := range st.ListRepos() {
		repoPath := c.paths.RepoDir(repoName)
		wtRootDir := c.paths.WorktreeDir(repoName)

		if _, err := os.Stat(wtRootDir); os.IsNotExist(err) {
			continue
		}

		wt := worktree.NewManager(repoPath)
		removed, err := worktree.CleanupOrphaned(wtRootDir, wt)
		if err != nil {
			if verbose {
				fmt.Printf("  Warning: failed to cleanup worktrees for %s: %v\n", repoName, err)
			}
			continue
		}

		if len(removed) > 0 {
			if verbose {
				fmt.Printf("  Cleaned up %d orphaned worktree(s) for %s\n", len(removed), repoName)
			}
			issuesFixed += len(removed)
		}

		// Prune git worktree references
		if err := wt.Prune(); err != nil && verbose {
			fmt.Printf("  Warning: failed to prune worktrees for %s: %v\n", repoName, err)
		}
	}

	// Clean up orphaned message directories
	msgMgr := messages.NewManager(c.paths.MessagesDir)
	for _, repoName := range st.ListRepos() {
		validAgents, _ := st.ListAgents(repoName)
		if count, err := msgMgr.CleanupOrphaned(repoName, validAgents); err == nil && count > 0 {
			if verbose {
				fmt.Printf("  Cleaned up %d orphaned message dir(s) for %s\n", count, repoName)
			}
			issuesFixed += count
		}
	}

	// Report orphaned tmux sessions
	if len(orphanedSessions) > 0 {
		fmt.Printf("\nFound %d orphaned tmux session(s) not in state:\n", len(orphanedSessions))
		for _, session := range orphanedSessions {
			fmt.Printf("  - %s\n", session)
		}
		fmt.Println("To remove these, run: tmux kill-session -t <session>")
		fmt.Println("Or use: multiclaude stop-all")
	}

	// Save updated state
	if err := st.Save(); err != nil {
		return fmt.Errorf("failed to save repaired state: %w", err)
	}

	fmt.Println("\n✓ Local repair completed")
	if agentsRemoved > 0 {
		fmt.Printf("  Removed %d dead agent(s)\n", agentsRemoved)
	}
	if issuesFixed > 0 {
		fmt.Printf("  Fixed %d issue(s)\n", issuesFixed)
	}
	if agentsRemoved == 0 && issuesFixed == 0 {
		fmt.Println("  No issues found")
	}

	return nil
}

func (c *CLI) showDocs(args []string) error {
	fmt.Println(c.documentation)
	return nil
}

// GenerateDocumentation generates markdown documentation for all CLI commands
func (c *CLI) GenerateDocumentation() string {
	var sb strings.Builder

	sb.WriteString("# Multiclaude CLI Reference\n\n")
	sb.WriteString("This is an automatically generated reference for all multiclaude commands.\n\n")

	// Generate docs for each top-level command
	for name, cmd := range c.rootCmd.Subcommands {
		c.generateCommandDocs(&sb, name, cmd, 0)
	}

	return sb.String()
}

// generateCommandDocs recursively generates documentation for a command and its subcommands
func (c *CLI) generateCommandDocs(sb *strings.Builder, name string, cmd *Command, level int) {
	indent := strings.Repeat("#", level+2)

	// Command header
	sb.WriteString(fmt.Sprintf("%s %s\n\n", indent, name))

	// Description
	if cmd.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", cmd.Description))
	}

	// Usage
	if cmd.Usage != "" {
		sb.WriteString(fmt.Sprintf("**Usage:** `%s`\n\n", cmd.Usage))
	}

	// Subcommands
	if len(cmd.Subcommands) > 0 {
		sb.WriteString("**Subcommands:**\n\n")
		for subName, subCmd := range cmd.Subcommands {
			// Skip internal commands
			if strings.HasPrefix(subName, "_") {
				continue
			}
			sb.WriteString(fmt.Sprintf("- `%s` - %s\n", subName, subCmd.Description))
		}
		sb.WriteString("\n")

		// Recursively document subcommands
		for subName, subCmd := range cmd.Subcommands {
			if !strings.HasPrefix(subName, "_") {
				c.generateCommandDocs(sb, subName, subCmd, level+1)
			}
		}
	}
}

// ParseFlags is a simple flag parser
func ParseFlags(args []string) (map[string]string, []string) {
	flags := make(map[string]string)
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			// Long flag
			flag := strings.TrimPrefix(arg, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags[flag] = args[i+1]
				i++
			} else {
				flags[flag] = "true"
			}
		} else if strings.HasPrefix(arg, "-") {
			// Short flag
			flag := strings.TrimPrefix(arg, "-")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags[flag] = args[i+1]
				i++
			} else {
				flags[flag] = "true"
			}
		} else {
			positional = append(positional, arg)
		}
	}

	return flags, positional
}

// generateSessionID generates a unique UUID v4 session ID for an agent
func generateSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Set version (4) and variant bits for UUID v4
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // Version 4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // Variant 10

	// Format as UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}

// writePromptFile writes the agent prompt to a temporary file and returns the path
func (c *CLI) writePromptFile(repoPath string, agentType prompts.AgentType, agentName string) (string, error) {
	// Get the complete prompt (default + custom + CLI docs)
	promptText, err := prompts.GetPrompt(repoPath, agentType, c.documentation)
	if err != nil {
		return "", fmt.Errorf("failed to get prompt: %w", err)
	}

	// Create a prompt file in the prompts directory
	promptDir := filepath.Join(c.paths.Root, "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create prompt directory: %w", err)
	}

	promptPath := filepath.Join(promptDir, fmt.Sprintf("%s.md", agentName))
	if err := os.WriteFile(promptPath, []byte(promptText), 0644); err != nil {
		return "", fmt.Errorf("failed to write prompt file: %w", err)
	}

	return promptPath, nil
}

// copyHooksConfig copies hooks configuration from repo to worktree if it exists
func (c *CLI) copyHooksConfig(repoPath, worktreePath string) error {
	hooksPath := filepath.Join(repoPath, ".multiclaude", "hooks.json")

	// Check if hooks.json exists
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		// No hooks config, that's fine
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check hooks config: %w", err)
	}

	// Create .claude directory in worktree
	claudeDir := filepath.Join(worktreePath, ".claude")
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

// startClaudeInTmux starts Claude Code in a tmux window with the given configuration
// Returns the PID of the Claude process
func (c *CLI) startClaudeInTmux(tmuxSession, tmuxWindow, workDir, sessionID, promptFile string, initialMessage string) (int, error) {
	// Build Claude command using the full path to prevent version drift
	claudeCmd := fmt.Sprintf("%s --session-id %s --dangerously-skip-permissions", c.claudeBinaryPath, sessionID)

	// Add prompt file if provided
	if promptFile != "" {
		claudeCmd += fmt.Sprintf(" --append-system-prompt-file %s", promptFile)
	}

	// Send command to tmux window
	target := fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)
	cmd := exec.Command("tmux", "send-keys", "-t", target, claudeCmd, "C-m")
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("failed to start Claude in tmux: %w", err)
	}

	// Wait a moment for Claude to start
	time.Sleep(500 * time.Millisecond)

	// Get the PID of the Claude process
	tmuxClient := tmux.NewClient()
	pid, err := tmuxClient.GetPanePID(tmuxSession, tmuxWindow)
	if err != nil {
		// Non-fatal - we'll just not have the PID
		fmt.Printf("Warning: failed to get Claude PID: %v\n", err)
		pid = 0
	}

	// If there's an initial message, send it after Claude is ready
	if initialMessage != "" {
		// Wait a bit more for Claude to fully initialize
		time.Sleep(1 * time.Second)

		// Send message using literal mode to ensure it's processed correctly
		if err := tmuxClient.SendKeysLiteral(tmuxSession, tmuxWindow, initialMessage); err != nil {
			return pid, fmt.Errorf("failed to send initial message text to Claude: %w", err)
		}
		if err := tmuxClient.SendEnter(tmuxSession, tmuxWindow); err != nil {
			return pid, fmt.Errorf("failed to send Enter for initial message to Claude: %w", err)
		}
		// Send Enter twice - Claude Code may swallow the first Enter when processing large pasted text
		if err := tmuxClient.SendEnter(tmuxSession, tmuxWindow); err != nil {
			return pid, fmt.Errorf("failed to send second Enter for initial message to Claude: %w", err)
		}
	}

	return pid, nil
}
