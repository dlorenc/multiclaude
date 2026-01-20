package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dlorenc/multiclaude/internal/bugreport"
	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/errors"
	"github.com/dlorenc/multiclaude/internal/format"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/names"
	"github.com/dlorenc/multiclaude/internal/prompts"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/claude"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

// Version is the current version of multiclaude (set at build time via ldflags)
var Version = "dev"

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
	rootCmd       *Command
	paths         *config.Paths
	documentation string // Auto-generated CLI documentation for prompts
}

// New creates a new CLI
func New() (*CLI, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}

	cli := &CLI{
		paths: paths,
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
func NewWithPaths(paths *config.Paths, _ string) *CLI {
	cli := &CLI{
		paths: paths,
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

// getClaudeBinary resolves the claude binary path
func (c *CLI) getClaudeBinary() (string, error) {
	binaryPath, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.ProviderNotFound("claude", err)
	}
	return binaryPath, nil
}

// tmuxSanitizer replaces problematic characters with hyphens for tmux session names.
// tmux has issues with dots, colons, spaces, and forward slashes in session names.
var tmuxSanitizer = strings.NewReplacer(
	".", "-",
	":", "-",
	" ", "-",
	"/", "-",
)

// sanitizeTmuxSessionName creates a tmux-safe session name from a repo name.
// tmux has issues with certain characters like dots, so we replace them.
func sanitizeTmuxSessionName(repoName string) string {
	// Strip control characters (ASCII 0-31) for safety
	sanitized := strings.Map(func(r rune) rune {
		if r < 32 {
			return -1 // drop the character
		}
		return r
	}, repoName)
	return fmt.Sprintf("mc-%s", tmuxSanitizer.Replace(sanitized))
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
			// Skip internal commands (prefixed with _)
			if strings.HasPrefix(name, "_") {
				continue
			}
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
		Usage:       "multiclaude daemon start",
		Run:         c.startDaemon,
	}

	daemonCmd.Subcommands["stop"] = &Command{
		Name:        "stop",
		Description: "Stop the daemon",
		Usage:       "multiclaude daemon stop",
		Run:         c.stopDaemon,
	}

	daemonCmd.Subcommands["status"] = &Command{
		Name:        "status",
		Description: "Show daemon status",
		Usage:       "multiclaude daemon status",
		Run:         c.daemonStatus,
	}

	daemonCmd.Subcommands["logs"] = &Command{
		Name:        "logs",
		Description: "View daemon logs",
		Usage:       "multiclaude daemon logs [-f|--follow] [-n <lines>]",
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
		Usage:       "multiclaude init <github-url> [name] [--no-merge-queue] [--mq-track=all|author|assigned]",
		Run:         c.initRepo,
	}

	c.rootCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List tracked repositories",
		Usage:       "multiclaude list",
		Run:         c.listRepos,
	}

	// Repository commands (repo subcommand)
	repoCmd := &Command{
		Name:        "repo",
		Description: "Manage repositories",
		Subcommands: make(map[string]*Command),
	}

	repoCmd.Subcommands["rm"] = &Command{
		Name:        "rm",
		Description: "Remove a tracked repository",
		Usage:       "multiclaude repo rm <name>",
		Run:         c.removeRepo,
	}

	repoCmd.Subcommands["use"] = &Command{
		Name:        "use",
		Description: "Set the default repository",
		Usage:       "multiclaude repo use <name>",
		Run:         c.setCurrentRepo,
	}

	repoCmd.Subcommands["current"] = &Command{
		Name:        "current",
		Description: "Show the default repository",
		Usage:       "multiclaude repo current",
		Run:         c.getCurrentRepo,
	}

	repoCmd.Subcommands["unset"] = &Command{
		Name:        "unset",
		Description: "Clear the default repository",
		Usage:       "multiclaude repo unset",
		Run:         c.clearCurrentRepo,
	}

	c.rootCmd.Subcommands["repo"] = repoCmd

	// Worker commands
	workCmd := &Command{
		Name:        "work",
		Description: "Manage worker agents",
		Usage:       "multiclaude work [<task>] [--repo <repo>]",
		Subcommands: make(map[string]*Command),
	}

	workCmd.Run = c.createWorker // Default action for 'work' command

	workCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List active workers",
		Usage:       "multiclaude work list [--repo <repo>]",
		Run:         c.listWorkers,
	}

	workCmd.Subcommands["rm"] = &Command{
		Name:        "rm",
		Description: "Remove a worker",
		Usage:       "multiclaude work rm <worker-name>",
		Run:         c.removeWorker,
	}

	c.rootCmd.Subcommands["work"] = workCmd

	// Workspace commands
	workspaceCmd := &Command{
		Name:        "workspace",
		Description: "Manage workspaces",
		Usage:       "multiclaude workspace [<name>]",
		Subcommands: make(map[string]*Command),
	}

	workspaceCmd.Run = c.workspaceDefault // Default action: list or connect

	workspaceCmd.Subcommands["add"] = &Command{
		Name:        "add",
		Description: "Add a new workspace",
		Usage:       "multiclaude workspace add <name> [--branch <branch>]",
		Run:         c.addWorkspace,
	}

	workspaceCmd.Subcommands["rm"] = &Command{
		Name:        "rm",
		Description: "Remove a workspace",
		Usage:       "multiclaude workspace rm <name>",
		Run:         c.removeWorkspace,
	}

	workspaceCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List workspaces",
		Usage:       "multiclaude workspace list",
		Run:         c.listWorkspaces,
	}

	workspaceCmd.Subcommands["connect"] = &Command{
		Name:        "connect",
		Description: "Connect to a workspace",
		Usage:       "multiclaude workspace connect <name>",
		Run:         c.connectWorkspace,
	}

	c.rootCmd.Subcommands["workspace"] = workspaceCmd

	// History command
	c.rootCmd.Subcommands["history"] = &Command{
		Name:        "history",
		Description: "Show task history for a repository",
		Usage:       "multiclaude history [--repo <repo>] [-n <count>]",
		Run:         c.showHistory,
	}

	// Agent commands (run from within Claude)
	agentCmd := &Command{
		Name:        "agent",
		Description: "Agent communication commands",
		Subcommands: make(map[string]*Command),
	}

	agentCmd.Subcommands["send-message"] = &Command{
		Name:        "send-message",
		Description: "Send a message to another agent",
		Usage:       "multiclaude agent send-message <recipient> <message>",
		Run:         c.sendMessage,
	}

	agentCmd.Subcommands["list-messages"] = &Command{
		Name:        "list-messages",
		Description: "List pending messages",
		Usage:       "multiclaude agent list-messages",
		Run:         c.listMessages,
	}

	agentCmd.Subcommands["read-message"] = &Command{
		Name:        "read-message",
		Description: "Read a specific message",
		Usage:       "multiclaude agent read-message <message-id>",
		Run:         c.readMessage,
	}

	agentCmd.Subcommands["ack-message"] = &Command{
		Name:        "ack-message",
		Description: "Acknowledge a message",
		Usage:       "multiclaude agent ack-message <message-id>",
		Run:         c.ackMessage,
	}

	agentCmd.Subcommands["complete"] = &Command{
		Name:        "complete",
		Description: "Signal worker completion",
		Usage:       "multiclaude agent complete",
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
		Usage:       "multiclaude cleanup [--dry-run] [--verbose] [--merged]",
		Run:         c.cleanup,
	}

	c.rootCmd.Subcommands["repair"] = &Command{
		Name:        "repair",
		Description: "Repair state after crash",
		Usage:       "multiclaude repair [--verbose]",
		Run:         c.repair,
	}

	// Claude restart command - for resuming Claude after exit
	c.rootCmd.Subcommands["claude"] = &Command{
		Name:        "claude",
		Description: "Restart Claude in current agent context",
		Usage:       "multiclaude claude",
		Run:         c.restartClaude,
	}

	// Debug command
	c.rootCmd.Subcommands["docs"] = &Command{
		Name:        "docs",
		Description: "Show generated CLI documentation",
		Usage:       "multiclaude docs",
		Run:         c.showDocs,
	}

	// Review command
	c.rootCmd.Subcommands["review"] = &Command{
		Name:        "review",
		Description: "Spawn a review agent for a PR",
		Usage:       "multiclaude review <pr-url>",
		Run:         c.reviewPR,
	}

	// Logs commands
	logsCmd := &Command{
		Name:        "logs",
		Description: "View and manage agent output logs",
		Usage:       "multiclaude logs [<agent-name>] [-f|--follow]",
		Subcommands: make(map[string]*Command),
	}

	logsCmd.Run = c.viewLogs // Default action: view logs for an agent

	logsCmd.Subcommands["list"] = &Command{
		Name:        "list",
		Description: "List log files",
		Usage:       "multiclaude logs list [--repo <repo>]",
		Run:         c.listLogs,
	}

	logsCmd.Subcommands["search"] = &Command{
		Name:        "search",
		Description: "Search across logs",
		Usage:       "multiclaude logs search <pattern> [--repo <repo>]",
		Run:         c.searchLogs,
	}

	logsCmd.Subcommands["clean"] = &Command{
		Name:        "clean",
		Description: "Remove old logs",
		Usage:       "multiclaude logs clean --older-than <duration>",
		Run:         c.cleanLogs,
	}

	c.rootCmd.Subcommands["logs"] = logsCmd

	// Config command
	c.rootCmd.Subcommands["config"] = &Command{
		Name:        "config",
		Description: "View or modify repository configuration",
		Usage:       "multiclaude config [repo] [--mq-enabled=true|false] [--mq-track=all|author|assigned]",
		Run:         c.configRepo,
	}

	// Bug report command
	c.rootCmd.Subcommands["bug"] = &Command{
		Name:        "bug",
		Description: "Generate a diagnostic bug report",
		Usage:       "multiclaude bug [--output <file>] [--verbose] [description]",
		Run:         c.bugReport,
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
	flags, posArgs := ParseFlags(args)

	if len(posArgs) < 1 {
		return errors.InvalidUsage("usage: multiclaude init <github-url> [name] [--no-merge-queue] [--mq-track=all|author|assigned]")
	}

	githubURL := strings.TrimRight(posArgs[0], "/")

	// Parse repository name from URL if not provided
	var repoName string
	if len(posArgs) >= 2 {
		repoName = posArgs[1]
	} else {
		// Extract repo name from URL (e.g., github.com/user/repo -> repo)
		// A valid GitHub URL has format: https://github.com/owner/repo
		// When split by "/": ["https:", "", "github.com", "owner", "repo"] - 5+ parts
		parts := strings.Split(githubURL, "/")
		if len(parts) < 5 {
			return errors.InvalidUsage("could not determine repository name from URL; please provide a name: multiclaude init <url> <name>")
		}
		repoName = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}

	// Validate repository name before any operations
	if repoName == "" {
		return errors.InvalidUsage("could not determine repository name from URL; please provide a name: multiclaude init <url> <name>")
	}

	// Parse merge queue configuration flags
	mqEnabled := flags["no-merge-queue"] != "true"
	mqTrackMode := state.TrackModeAll
	if trackMode, ok := flags["mq-track"]; ok {
		switch trackMode {
		case "all":
			mqTrackMode = state.TrackModeAll
		case "author":
			mqTrackMode = state.TrackModeAuthor
		case "assigned":
			mqTrackMode = state.TrackModeAssigned
		default:
			return fmt.Errorf("invalid --mq-track value: %s (must be 'all', 'author', or 'assigned')", trackMode)
		}
	}

	mqConfig := state.MergeQueueConfig{
		Enabled:   mqEnabled,
		TrackMode: mqTrackMode,
	}

	fmt.Printf("Initializing repository: %s\n", repoName)
	fmt.Printf("GitHub URL: %s\n", githubURL)
	if mqEnabled {
		fmt.Printf("Merge queue: enabled (tracking: %s)\n", mqTrackMode)
	} else {
		fmt.Printf("Merge queue: disabled\n")
	}

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
	tmuxSession := sanitizeTmuxSessionName(repoName)
	if tmuxSession == "mc-" {
		return fmt.Errorf("invalid tmux session name: repository name cannot be empty")
	}

	fmt.Printf("Creating tmux session: %s\n", tmuxSession)

	// Create session with supervisor window
	cmd = exec.Command("tmux", "new-session", "-d", "-s", tmuxSession, "-n", "supervisor", "-c", repoPath)
	if err := cmd.Run(); err != nil {
		return errors.TmuxOperationFailed("create session", err)
	}

	// Create merge-queue window only if enabled
	if mqEnabled {
		cmd = exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", "merge-queue", "-c", repoPath)
		if err := cmd.Run(); err != nil {
			return errors.TmuxOperationFailed("create merge-queue window", err)
		}
	}

	// Generate session IDs for agents
	supervisorSessionID, err := claude.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate supervisor session ID: %w", err)
	}

	var mergeQueueSessionID string
	if mqEnabled {
		mergeQueueSessionID, err = claude.GenerateSessionID()
		if err != nil {
			return fmt.Errorf("failed to generate merge-queue session ID: %w", err)
		}
	}

	// Write prompt files
	supervisorPromptFile, err := c.writePromptFile(repoPath, prompts.TypeSupervisor, "supervisor")
	if err != nil {
		return fmt.Errorf("failed to write supervisor prompt: %w", err)
	}

	var mergeQueuePromptFile string
	if mqEnabled {
		mergeQueuePromptFile, err = c.writeMergeQueuePromptFile(repoPath, "merge-queue", mqConfig)
		if err != nil {
			return fmt.Errorf("failed to write merge-queue prompt: %w", err)
		}
	}

	// Copy hooks configuration if it exists (for supervisor and merge-queue)
	if err := c.copyHooksConfig(repoPath, repoPath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config: %v\n", err)
	}

	// Start Claude in supervisor window (skip in test mode)
	var supervisorPID, mergeQueuePID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		// Resolve claude binary
		claudeBinary, err := c.getClaudeBinary()
		if err != nil {
			return fmt.Errorf("failed to resolve claude binary: %w", err)
		}

		fmt.Println("Starting Claude Code in supervisor window...")
		pid, err := c.startClaudeInTmux(claudeBinary, tmuxSession, "supervisor", repoPath, supervisorSessionID, supervisorPromptFile, "")
		if err != nil {
			return fmt.Errorf("failed to start supervisor Claude: %w", err)
		}
		supervisorPID = pid

		// Set up output capture for supervisor
		if err := c.setupOutputCapture(tmuxSession, "supervisor", repoName, "supervisor", "supervisor"); err != nil {
			fmt.Printf("Warning: failed to setup output capture for supervisor: %v\n", err)
		}

		// Start Claude in merge-queue window only if enabled
		if mqEnabled {
			fmt.Println("Starting Claude Code in merge-queue window...")
			pid, err = c.startClaudeInTmux(claudeBinary, tmuxSession, "merge-queue", repoPath, mergeQueueSessionID, mergeQueuePromptFile, "")
			if err != nil {
				return fmt.Errorf("failed to start merge-queue Claude: %w", err)
			}
			mergeQueuePID = pid

			// Set up output capture for merge-queue
			if err := c.setupOutputCapture(tmuxSession, "merge-queue", repoName, "merge-queue", "merge-queue"); err != nil {
				fmt.Printf("Warning: failed to setup output capture for merge-queue: %v\n", err)
			}
		}
	}

	// Add repository to daemon state (with merge queue config)
	resp, err := client.Send(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":          repoName,
			"github_url":    githubURL,
			"tmux_session":  tmuxSession,
			"mq_enabled":    mqConfig.Enabled,
			"mq_track_mode": string(mqConfig.TrackMode),
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

	// Add merge-queue agent only if enabled
	if mqEnabled {
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
	}

	// Create default workspace worktree
	wt := worktree.NewManager(repoPath)
	workspacePath := c.paths.AgentWorktree(repoName, "default")

	// Check for and migrate legacy "workspace" branch to "workspace/default"
	// This allows the new workspace/<name> naming convention to work
	migrated, err := wt.MigrateLegacyWorkspaceBranch()
	if err != nil {
		// Check if it's a conflict state that requires manual resolution
		hasConflict, suggestion, checkErr := wt.CheckWorkspaceBranchConflict()
		if checkErr == nil && hasConflict {
			return fmt.Errorf("workspace branch conflict detected:\n%s", suggestion)
		}
		return fmt.Errorf("failed to check workspace branch state: %w", err)
	}
	if migrated {
		fmt.Println("Migrated legacy 'workspace' branch to 'workspace/default'")
	}
	workspaceBranch := "workspace/default"

	fmt.Printf("Creating default workspace worktree at: %s\n", workspacePath)
	if err := wt.CreateNewBranch(workspacePath, workspaceBranch, "HEAD"); err != nil {
		return fmt.Errorf("failed to create default workspace worktree: %w", err)
	}

	// Create default workspace tmux window (detached so it doesn't switch focus)
	cmd = exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", "default", "-c", workspacePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create workspace window: %w", err)
	}

	// Generate session ID for workspace
	workspaceSessionID, err := claude.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate workspace session ID: %w", err)
	}

	// Write prompt file for default workspace
	workspacePromptFile, err := c.writePromptFile(repoPath, prompts.TypeWorkspace, "default")
	if err != nil {
		return fmt.Errorf("failed to write default workspace prompt: %w", err)
	}

	// Copy hooks configuration if it exists
	if err := c.copyHooksConfig(repoPath, workspacePath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config to default workspace: %v\n", err)
	}

	// Start Claude in default workspace window (skip in test mode)
	var workspacePID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		// Resolve claude binary
		claudeBinary, err := c.getClaudeBinary()
		if err != nil {
			return fmt.Errorf("failed to resolve claude binary: %w", err)
		}

		fmt.Println("Starting Claude Code in default workspace window...")
		pid, err := c.startClaudeInTmux(claudeBinary, tmuxSession, "default", workspacePath, workspaceSessionID, workspacePromptFile, "")
		if err != nil {
			return fmt.Errorf("failed to start default workspace Claude: %w", err)
		}
		workspacePID = pid

		// Set up output capture for default workspace
		if err := c.setupOutputCapture(tmuxSession, "default", repoName, "default", "workspace"); err != nil {
			fmt.Printf("Warning: failed to setup output capture for default workspace: %v\n", err)
		}
	}

	// Add default workspace agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         "default",
			"type":          "workspace",
			"worktree_path": workspacePath,
			"tmux_window":   "default",
			"session_id":    workspaceSessionID,
			"pid":           workspacePID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register default workspace: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to register default workspace: %s", resp.Error)
	}

	fmt.Println()
	fmt.Println("✓ Repository initialized successfully!")
	fmt.Printf("  Tmux session: %s\n", tmuxSession)
	if mqEnabled {
		fmt.Printf("  Agents: supervisor, merge-queue, default (workspace)\n")
	} else {
		fmt.Printf("  Agents: supervisor, default (workspace)\n")
	}
	fmt.Printf("\nAttach to session: tmux attach -t %s\n", tmuxSession)
	fmt.Printf("Or connect to your workspace: multiclaude workspace connect default\n")

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

func (c *CLI) removeRepo(args []string) error {
	var repoName string
	if len(args) > 0 {
		repoName = args[0]
	} else {
		// Interactive selection - list repos
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

		repos, _ := resp.Data.([]interface{})
		items := reposToSelectableItems(repos)
		if len(items) == 0 {
			return fmt.Errorf("no repositories found")
		}
		selected, err := SelectFromList("Select repository to remove:", items)
		if err != nil {
			return err
		}
		if selected == "" {
			fmt.Println("Cancelled")
			return nil
		}
		repoName = selected
	}

	fmt.Printf("Removing repository '%s'...\n", repoName)

	// Get repo info from daemon
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

	// Get list of agents
	agents, _ := resp.Data.([]interface{})

	// Check for any workers with uncommitted changes
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			if agentType == "worker" || agentType == "review" {
				wtPath, _ := agentMap["worktree_path"].(string)
				if wtPath != "" {
					hasUncommitted, err := worktree.HasUncommittedChanges(wtPath)
					if err == nil && hasUncommitted {
						agentName, _ := agentMap["name"].(string)
						fmt.Printf("\nWarning: Agent '%s' has uncommitted changes!\n", agentName)
						fmt.Println("Files may be lost if you continue.")
						fmt.Print("Continue with removal? [y/N]: ")

						var response string
						fmt.Scanln(&response)
						if response != "y" && response != "Y" {
							fmt.Println("Removal cancelled")
							return nil
						}
						break // Only ask once
					}
				}
			}
		}
	}

	// Kill tmux session
	tmuxSession := sanitizeTmuxSessionName(repoName)
	tmuxClient := tmux.NewClient()
	if exists, err := tmuxClient.HasSession(tmuxSession); err == nil && exists {
		fmt.Printf("Killing tmux session: %s\n", tmuxSession)
		if err := tmuxClient.KillSession(tmuxSession); err != nil {
			fmt.Printf("Warning: failed to kill tmux session: %v\n", err)
		}
	}

	// Remove worktrees for all agents
	repoPath := c.paths.RepoDir(repoName)
	wt := worktree.NewManager(repoPath)
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			wtPath, _ := agentMap["worktree_path"].(string)
			agentName, _ := agentMap["name"].(string)
			if wtPath != "" && wtPath != repoPath {
				fmt.Printf("Removing worktree for '%s': %s\n", agentName, wtPath)
				if err := wt.Remove(wtPath, true); err != nil {
					fmt.Printf("Warning: failed to remove worktree: %v\n", err)
				}
			}
		}
	}

	// Remove the worktrees directory for this repo
	wtDir := c.paths.WorktreeDir(repoName)
	if _, err := os.Stat(wtDir); err == nil {
		fmt.Printf("Removing worktrees directory: %s\n", wtDir)
		if err := os.RemoveAll(wtDir); err != nil {
			fmt.Printf("Warning: failed to remove worktrees directory: %v\n", err)
		}
	}

	// Clean up messages directory for this repo
	msgDir := filepath.Join(c.paths.MessagesDir, repoName)
	if _, err := os.Stat(msgDir); err == nil {
		fmt.Printf("Removing messages directory: %s\n", msgDir)
		if err := os.RemoveAll(msgDir); err != nil {
			fmt.Printf("Warning: failed to remove messages directory: %v\n", err)
		}
	}

	// Unregister from daemon
	resp, err = client.Send(socket.Request{
		Command: "remove_repo",
		Args: map[string]interface{}{
			"name": repoName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("removing repo", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to remove repo from state", fmt.Errorf("%s", resp.Error))
	}

	fmt.Println("✓ Repository removed successfully")
	fmt.Printf("\nNote: The cloned repository at '%s' was NOT deleted.\n", repoPath)
	fmt.Println("Delete it manually if you no longer need it.")
	return nil
}

func (c *CLI) setCurrentRepo(args []string) error {
	if len(args) < 1 {
		return errors.InvalidUsage("usage: multiclaude repo use <name>")
	}

	repoName := args[0]

	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "set_current_repo",
		Args: map[string]interface{}{
			"name": repoName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("setting current repo", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to set current repo", fmt.Errorf("%s", resp.Error))
	}

	fmt.Printf("Current repository set to: %s\n", repoName)
	return nil
}

func (c *CLI) getCurrentRepo(args []string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "get_current_repo",
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("getting current repo", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to get current repo", fmt.Errorf("%s", resp.Error))
	}

	currentRepo, _ := resp.Data.(string)
	if currentRepo == "" {
		fmt.Println("No current repository set")
		fmt.Println("\nUse 'multiclaude repo use <name>' to set one")
	} else {
		fmt.Printf("Current repository: %s\n", currentRepo)
	}
	return nil
}

func (c *CLI) clearCurrentRepo(args []string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "clear_current_repo",
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("clearing current repo", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to clear current repo", fmt.Errorf("%s", resp.Error))
	}

	fmt.Println("Current repository cleared")
	return nil
}

func (c *CLI) configRepo(args []string) error {
	flags, posArgs := ParseFlags(args)

	// Determine repository
	var repoName string
	if len(posArgs) >= 1 {
		repoName = posArgs[0]
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
			// If only one repo exists, use it
			if len(repos) == 1 {
				repoName = repos[0]
			} else {
				return fmt.Errorf("please specify a repository name or run from within a tracked repository")
			}
		}
	}

	// Check if any config flags are provided
	hasMqEnabled := flags["mq-enabled"] != ""
	hasMqTrack := flags["mq-track"] != ""
	hasProvider := flags["provider"] != ""

	if !hasMqEnabled && !hasMqTrack && !hasProvider {
		// No flags - just show current config
		return c.showRepoConfig(repoName)
	}

	// Apply config changes
	return c.updateRepoConfig(repoName, flags)
}

func (c *CLI) showRepoConfig(repoName string) error {
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "get_repo_config",
		Args: map[string]interface{}{
			"name": repoName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get repo config: %w (is daemon running?)", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to get repo config: %s", resp.Error)
	}

	// Parse response
	configMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	fmt.Printf("Configuration for repository: %s\n\n", repoName)
	fmt.Println("Merge Queue:")

	mqEnabled := true
	if enabled, ok := configMap["mq_enabled"].(bool); ok {
		mqEnabled = enabled
	}

	mqTrackMode := "all"
	if trackMode, ok := configMap["mq_track_mode"].(string); ok {
		mqTrackMode = trackMode
	}

	if mqEnabled {
		fmt.Printf("  Enabled: true\n")
		fmt.Printf("  Track mode: %s\n", mqTrackMode)
	} else {
		fmt.Printf("  Enabled: false\n")
	}

	// Show provider config
	providerStr := "claude"
	if p, ok := configMap["provider"].(string); ok && p != "" {
		providerStr = p
	}
	fmt.Printf("\nProvider: %s\n", providerStr)

	fmt.Println("\nTo modify:")
	fmt.Printf("  multiclaude config %s --mq-enabled=true|false\n", repoName)
	fmt.Printf("  multiclaude config %s --mq-track=all|author|assigned\n", repoName)
	fmt.Printf("  multiclaude config %s --provider=claude|happy\n", repoName)

	return nil
}

func (c *CLI) updateRepoConfig(repoName string, flags map[string]string) error {
	// Build update args
	updateArgs := map[string]interface{}{
		"name": repoName,
	}

	// Parse and validate flags
	if mqEnabled, ok := flags["mq-enabled"]; ok {
		switch mqEnabled {
		case "true":
			updateArgs["mq_enabled"] = true
		case "false":
			updateArgs["mq_enabled"] = false
		default:
			return fmt.Errorf("invalid --mq-enabled value: %s (must be 'true' or 'false')", mqEnabled)
		}
	}

	if mqTrack, ok := flags["mq-track"]; ok {
		switch mqTrack {
		case "all", "author", "assigned":
			updateArgs["mq_track_mode"] = mqTrack
		default:
			return fmt.Errorf("invalid --mq-track value: %s (must be 'all', 'author', or 'assigned')", mqTrack)
		}
	}

	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "update_repo_config",
		Args:    updateArgs,
	})
	if err != nil {
		return fmt.Errorf("failed to update repo config: %w (is daemon running?)", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to update repo config: %s", resp.Error)
	}

	fmt.Printf("Configuration updated for repository: %s\n", repoName)

	// Show the updated config
	return c.showRepoConfig(repoName)
}

func (c *CLI) createWorker(args []string) error {
	flags, posArgs := ParseFlags(args)

	// Get task description
	task := strings.Join(posArgs, " ")
	if task == "" {
		return errors.InvalidUsage("usage: multiclaude work <task description>")
	}

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
	}

	// Generate worker name (Docker-style)
	workerName := names.Generate()
	if name, ok := flags["name"]; ok {
		workerName = name
	}

	// Get repository path
	repoPath := c.paths.RepoDir(repoName)

	// Fetch latest main from origin before creating worktree
	// This ensures workers start from the latest code, not stale local refs
	// Note: We use "git fetch origin main" (not "main:main") because the latter
	// fails when main is checked out in the bare repo with:
	// "fatal: refusing to fetch into branch 'refs/heads/main' checked out at ..."
	fmt.Println("Fetching latest from origin...")
	fetchCmd := exec.Command("git", "fetch", "origin", "main")
	fetchCmd.Dir = repoPath
	if err := fetchCmd.Run(); err != nil {
		// Best effort - don't fail if offline or fetch fails
		fmt.Printf("Warning: failed to fetch origin/main: %v (continuing with local refs)\n", err)
	}

	// Determine branch to start from
	// Prefer origin/main if it exists (updated by fetch), otherwise fall back to HEAD
	// This handles both normal repos and test repos without remotes
	startBranch := "HEAD"
	checkOriginCmd := exec.Command("git", "rev-parse", "--verify", "origin/main")
	checkOriginCmd.Dir = repoPath
	if err := checkOriginCmd.Run(); err == nil {
		startBranch = "origin/main"
	}
	if branch, ok := flags["branch"]; ok {
		startBranch = branch
		fmt.Printf("Creating worker '%s' in repo '%s' from branch '%s'\n", workerName, repoName, branch)
	} else {
		fmt.Printf("Creating worker '%s' in repo '%s'\n", workerName, repoName)
	}
	fmt.Printf("Task: %s\n", task)

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
	tmuxSession := sanitizeTmuxSessionName(repoName)

	// Ensure tmux session exists before creating window
	// This handles cases where the session was killed or daemon didn't restore it
	tmuxClient := tmux.NewClient()
	hasSession, err := tmuxClient.HasSession(tmuxSession)
	if err != nil {
		return errors.TmuxOperationFailed("check session", err)
	}
	if !hasSession {
		fmt.Printf("Tmux session '%s' not found, creating it...\n", tmuxSession)
		if err := tmuxClient.CreateSession(tmuxSession, true); err != nil {
			return errors.TmuxOperationFailed("create session", err)
		}
	}

	// Create tmux window for worker (detached so it doesn't switch focus)
	fmt.Printf("Creating tmux window: %s\n", workerName)
	cmd := exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", workerName, "-c", wtPath)
	if err := cmd.Run(); err != nil {
		return errors.TmuxOperationFailed("create window", err)
	}

	// Generate session ID for worker
	workerSessionID, err := claude.GenerateSessionID()
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
		// Resolve claude binary
		claudeBinary, err := c.getClaudeBinary()
		if err != nil {
			return fmt.Errorf("failed to resolve claude binary: %w", err)
		}

		fmt.Println("Starting Claude Code in worker window...")
		initialMessage := fmt.Sprintf("Task: %s", task)
		pid, err := c.startClaudeInTmux(claudeBinary, tmuxSession, workerName, wtPath, workerSessionID, workerPromptFile, initialMessage)
		if err != nil {
			return fmt.Errorf("failed to start worker Claude: %w", err)
		}
		workerPID = pid

		// Set up output capture for worker
		if err := c.setupOutputCapture(tmuxSession, workerName, repoName, workerName, "worker"); err != nil {
			fmt.Printf("Warning: failed to setup output capture for worker: %v\n", err)
		}
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
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
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

func (c *CLI) showHistory(args []string) error {
	flags, _ := ParseFlags(args)

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
	}

	// Get limit from flags (default 10)
	limit := 10
	if n, ok := flags["n"]; ok {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			limit = v
		}
	}

	// Get task history from daemon
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "task_history",
		Args: map[string]interface{}{
			"repo":  repoName,
			"limit": limit,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("getting task history", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to get task history", fmt.Errorf("%s", resp.Error))
	}

	history, ok := resp.Data.([]interface{})
	if !ok || len(history) == 0 {
		fmt.Printf("No task history for repository '%s'\n", repoName)
		format.Dimmed("\nCreate workers with: multiclaude work <task>")
		return nil
	}

	// Query GitHub for PR status for each task with a branch
	repoPath := c.paths.RepoDir(repoName)

	format.Header("Task History for '%s' (last %d):", repoName, len(history))
	fmt.Println()

	table := format.NewColoredTable("NAME", "STATUS", "PR", "COMPLETED", "TASK")
	for _, item := range history {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := entry["name"].(string)
		task, _ := entry["task"].(string)
		branch, _ := entry["branch"].(string)
		prURL, _ := entry["pr_url"].(string)
		completedAt, _ := entry["completed_at"].(string)

		// Try to get PR status from GitHub if we have a branch
		prStatus, prLink := c.getPRStatusForBranch(repoPath, branch, prURL)

		// Format status with color
		var statusCell format.ColoredCell
		switch prStatus {
		case "merged":
			statusCell = format.ColorCell("merged", format.Green)
		case "open":
			statusCell = format.ColorCell("open", format.Yellow)
		case "closed":
			statusCell = format.ColorCell("closed", format.Red)
		default:
			statusCell = format.ColorCell("no-pr", format.Dim)
		}

		// Format PR link
		prCell := format.ColorCell("-", format.Dim)
		if prLink != "" {
			// Extract just the PR number for display
			prCell = format.ColorCell(prLink, format.Cyan)
		}

		// Format completed time
		completedCell := format.ColorCell("-", format.Dim)
		if completedAt != "" {
			if t, err := time.Parse(time.RFC3339, completedAt); err == nil {
				completedCell = format.Cell(format.TimeAgo(t))
			}
		}

		// Truncate task
		truncTask := format.Truncate(task, 35)

		table.AddRow(
			format.Cell(name),
			statusCell,
			prCell,
			completedCell,
			format.Cell(truncTask),
		)
	}
	table.Print()

	return nil
}

// getPRStatusForBranch queries GitHub for the PR status of a branch
func (c *CLI) getPRStatusForBranch(repoPath, branch, existingPRURL string) (status, prLink string) {
	// If we already have a PR URL, just return it formatted
	if existingPRURL != "" {
		// Extract PR number from URL for shorter display
		parts := strings.Split(existingPRURL, "/")
		if len(parts) > 0 {
			prNum := parts[len(parts)-1]
			return "unknown", "#" + prNum
		}
		return "unknown", existingPRURL
	}

	// If no branch, nothing to query
	if branch == "" {
		return "no-pr", ""
	}

	// Query GitHub for PR associated with this branch using gh CLI
	cmd := exec.Command("gh", "pr", "list", "--head", branch, "--state", "all", "--json", "number,state,url", "--limit", "1")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "no-pr", ""
	}

	// Parse JSON output
	var prs []struct {
		Number int    `json:"number"`
		State  string `json:"state"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(output, &prs); err != nil || len(prs) == 0 {
		return "no-pr", ""
	}

	pr := prs[0]
	prLink = fmt.Sprintf("#%d", pr.Number)

	switch strings.ToLower(pr.State) {
	case "merged":
		return "merged", prLink
	case "open":
		return "open", prLink
	case "closed":
		return "closed", prLink
	default:
		return "unknown", prLink
	}
}

func (c *CLI) removeWorker(args []string) error {
	flags, remainingArgs := ParseFlags(args)

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
	}

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

	agents, _ := resp.Data.([]interface{})

	// Determine worker name - from args or interactive selection
	var workerName string
	if len(remainingArgs) > 0 {
		workerName = remainingArgs[0]
	} else {
		// Interactive selection
		items := agentsToSelectableItems(agents, []string{"worker"})
		if len(items) == 0 {
			return fmt.Errorf("no workers found in repo '%s'", repoName)
		}
		selected, err := SelectFromList("Select worker to remove:", items)
		if err != nil {
			return err
		}
		if selected == "" {
			fmt.Println("Cancelled")
			return nil
		}
		workerName = selected
	}

	fmt.Printf("Removing worker '%s' from repo '%s'\n", workerName, repoName)

	// Find worker
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
	tmuxSession := sanitizeTmuxSessionName(repoName)
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

// Workspace command implementations

// workspaceDefault handles `multiclaude workspace` with no subcommand or `multiclaude workspace <name>`
func (c *CLI) workspaceDefault(args []string) error {
	// If no args, list workspaces
	if len(args) == 0 {
		return c.listWorkspaces(args)
	}

	// If first arg looks like a workspace name (not a flag), treat as connect
	if !strings.HasPrefix(args[0], "-") {
		return c.connectWorkspace(args)
	}

	// Otherwise list with flags
	return c.listWorkspaces(args)
}

// addWorkspace creates a new workspace
func (c *CLI) addWorkspace(args []string) error {
	flags, posArgs := ParseFlags(args)

	if len(posArgs) < 1 {
		return errors.InvalidUsage("usage: multiclaude workspace add <name> [--branch <branch>]")
	}

	workspaceName := posArgs[0]

	// Validate workspace name (same restrictions as branch names)
	if err := validateWorkspaceName(workspaceName); err != nil {
		return err
	}

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		// Try to infer from current directory
		if inferred, err := c.inferRepoFromCwd(); err == nil {
			repoName = inferred
		} else {
			return errors.MultipleRepos()
		}
	}

	// Determine branch to start from
	startBranch := "HEAD" // Default to current branch/HEAD
	if branch, ok := flags["branch"]; ok {
		startBranch = branch
		fmt.Printf("Creating workspace '%s' in repo '%s' from branch '%s'\n", workspaceName, repoName, branch)
	} else {
		fmt.Printf("Creating workspace '%s' in repo '%s'\n", workspaceName, repoName)
	}

	// Check if workspace already exists
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("checking existing workspaces", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to check existing workspaces", fmt.Errorf("%s", resp.Error))
	}

	agents, _ := resp.Data.([]interface{})
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			name, _ := agentMap["name"].(string)
			if agentType == "workspace" && name == workspaceName {
				return fmt.Errorf("workspace '%s' already exists in repo '%s'", workspaceName, repoName)
			}
		}
	}

	// Get repository path
	repoPath := c.paths.RepoDir(repoName)

	// Create worktree
	wt := worktree.NewManager(repoPath)
	wtPath := c.paths.AgentWorktree(repoName, workspaceName)
	branchName := fmt.Sprintf("workspace/%s", workspaceName)

	fmt.Printf("Creating worktree at: %s\n", wtPath)
	if err := wt.CreateNewBranch(wtPath, branchName, startBranch); err != nil {
		return errors.WorktreeCreationFailed(err)
	}

	// Get tmux session name
	tmuxSession := sanitizeTmuxSessionName(repoName)

	// Create tmux window for workspace (detached so it doesn't switch focus)
	fmt.Printf("Creating tmux window: %s\n", workspaceName)
	cmd := exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", workspaceName, "-c", wtPath)
	if err := cmd.Run(); err != nil {
		return errors.TmuxOperationFailed("create window", err)
	}

	// Generate session ID for workspace
	workspaceSessionID, err := claude.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate workspace session ID: %w", err)
	}

	// Write prompt file for workspace
	workspacePromptFile, err := c.writePromptFile(repoPath, prompts.TypeWorkspace, workspaceName)
	if err != nil {
		return fmt.Errorf("failed to write workspace prompt: %w", err)
	}

	// Copy hooks configuration if it exists
	if err := c.copyHooksConfig(repoPath, wtPath); err != nil {
		fmt.Printf("Warning: failed to copy hooks config: %v\n", err)
	}

	// Start Claude in workspace window (skip in test mode)
	var workspacePID int
	if os.Getenv("MULTICLAUDE_TEST_MODE") != "1" {
		// Resolve claude binary
		claudeBinary, err := c.getClaudeBinary()
		if err != nil {
			return fmt.Errorf("failed to resolve claude binary: %w", err)
		}

		fmt.Println("Starting Claude Code in workspace window...")
		pid, err := c.startClaudeInTmux(claudeBinary, tmuxSession, workspaceName, wtPath, workspaceSessionID, workspacePromptFile, "")
		if err != nil {
			return fmt.Errorf("failed to start workspace Claude: %w", err)
		}
		workspacePID = pid

		// Set up output capture for workspace
		if err := c.setupOutputCapture(tmuxSession, workspaceName, repoName, workspaceName, "workspace"); err != nil {
			fmt.Printf("Warning: failed to setup output capture for workspace: %v\n", err)
		}
	}

	// Register workspace with daemon
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          repoName,
			"agent":         workspaceName,
			"type":          "workspace",
			"worktree_path": wtPath,
			"tmux_window":   workspaceName,
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
	fmt.Println("✓ Workspace created successfully!")
	fmt.Printf("  Name: %s\n", workspaceName)
	fmt.Printf("  Branch: %s\n", branchName)
	fmt.Printf("  Worktree: %s\n", wtPath)
	fmt.Printf("\nConnect to workspace: multiclaude workspace connect %s\n", workspaceName)
	fmt.Printf("Or use: multiclaude attach %s\n", workspaceName)

	return nil
}

// removeWorkspace removes a workspace
func (c *CLI) removeWorkspace(args []string) error {
	flags, remainingArgs := ParseFlags(args)

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
	}

	// Get workspace info
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return errors.DaemonCommunicationFailed("getting workspace info", err)
	}
	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to get workspace info", fmt.Errorf("%s", resp.Error))
	}

	agents, _ := resp.Data.([]interface{})

	// Determine workspace name - from args or interactive selection
	var workspaceName string
	if len(remainingArgs) > 0 {
		workspaceName = remainingArgs[0]
	} else {
		// Interactive selection
		items := agentsToSelectableItems(agents, []string{"workspace"})
		if len(items) == 0 {
			return fmt.Errorf("no workspaces found in repo '%s'", repoName)
		}
		selected, err := SelectFromList("Select workspace to remove:", items)
		if err != nil {
			return err
		}
		if selected == "" {
			fmt.Println("Cancelled")
			return nil
		}
		workspaceName = selected
	}

	fmt.Printf("Removing workspace '%s' from repo '%s'\n", workspaceName, repoName)

	// Find workspace
	var workspaceInfo map[string]interface{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			name, _ := agentMap["name"].(string)
			if agentType == "workspace" && name == workspaceName {
				workspaceInfo = agentMap
				break
			}
		}
	}

	if workspaceInfo == nil {
		return errors.AgentNotFound("workspace", workspaceName, repoName)
	}

	// Get worktree path
	wtPath := workspaceInfo["worktree_path"].(string)

	// Check for uncommitted changes
	hasUncommitted, err := worktree.HasUncommittedChanges(wtPath)
	if err != nil {
		fmt.Printf("Warning: failed to check for uncommitted changes: %v\n", err)
	} else if hasUncommitted {
		fmt.Println("\nWarning: Workspace has uncommitted changes!")
		fmt.Println("Files may be lost if you continue with removal.")
		fmt.Print("Continue with removal? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Removal cancelled")
			return nil
		}
	}

	// Check for unpushed commits
	hasUnpushed, err := worktree.HasUnpushedCommits(wtPath)
	if err != nil {
		// This is ok - might not have a tracking branch
		fmt.Printf("Note: Could not check for unpushed commits (no tracking branch?)\n")
	} else if hasUnpushed {
		fmt.Println("\nWarning: Workspace has unpushed commits!")
		branch, err := worktree.GetCurrentBranch(wtPath)
		if err == nil {
			fmt.Printf("Branch '%s' has commits not pushed to remote.\n", branch)
		}
		fmt.Println("These commits may be lost if you continue with removal.")
		fmt.Print("Continue with removal? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Removal cancelled")
			return nil
		}
	}

	// Kill tmux window
	tmuxSession := sanitizeTmuxSessionName(repoName)
	tmuxWindow := workspaceInfo["tmux_window"].(string)
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
			"agent": workspaceName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to unregister workspace: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to unregister workspace: %s", resp.Error)
	}

	fmt.Println("✓ Workspace removed successfully")
	return nil
}

// listWorkspaces lists all workspaces in a repository
func (c *CLI) listWorkspaces(args []string) error {
	flags, _ := ParseFlags(args)

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
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
		return errors.DaemonCommunicationFailed("listing workspaces", err)
	}

	if !resp.Success {
		return errors.Wrap(errors.CategoryRuntime, "failed to list workspaces", fmt.Errorf("%s", resp.Error))
	}

	agents, ok := resp.Data.([]interface{})
	if !ok {
		return errors.New(errors.CategoryRuntime, "unexpected response format from daemon")
	}

	// Filter for workspaces
	workspaces := []map[string]interface{}{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			if agentType == "workspace" {
				workspaces = append(workspaces, agentMap)
			}
		}
	}

	if len(workspaces) == 0 {
		fmt.Printf("No workspaces in repository '%s'\n", repoName)
		format.Dimmed("\nCreate a workspace with: multiclaude workspace add <name>")
		return nil
	}

	format.Header("Workspaces in '%s' (%d):", repoName, len(workspaces))
	fmt.Println()

	table := format.NewColoredTable("NAME", "BRANCH", "STATUS")
	for _, ws := range workspaces {
		name, _ := ws["name"].(string)
		status, _ := ws["status"].(string)
		branch, _ := ws["branch"].(string)

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

		table.AddRow(
			format.Cell(name),
			branchCell,
			statusCell,
		)
	}
	table.Print()

	return nil
}

// connectWorkspace attaches to a workspace
func (c *CLI) connectWorkspace(args []string) error {
	flags, remainingArgs := ParseFlags(args)

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
	}

	// Get workspace info
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get workspace info: %w (is daemon running?)", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to get workspace info: %s", resp.Error)
	}

	agents, _ := resp.Data.([]interface{})

	// Determine workspace name - from args or interactive selection
	var workspaceName string
	if len(remainingArgs) > 0 {
		workspaceName = remainingArgs[0]
	} else {
		// Interactive selection
		items := agentsToSelectableItems(agents, []string{"workspace"})
		if len(items) == 0 {
			return fmt.Errorf("no workspaces found in repo '%s'", repoName)
		}
		selected, err := SelectFromList("Select workspace to connect:", items)
		if err != nil {
			return err
		}
		if selected == "" {
			fmt.Println("Cancelled")
			return nil
		}
		workspaceName = selected
	}

	// Find workspace
	var workspaceInfo map[string]interface{}
	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			name, _ := agentMap["name"].(string)
			if agentType == "workspace" && name == workspaceName {
				workspaceInfo = agentMap
				break
			}
		}
	}

	if workspaceInfo == nil {
		return fmt.Errorf("workspace '%s' not found in repo '%s'", workspaceName, repoName)
	}

	// Get tmux session and window
	tmuxSession := sanitizeTmuxSessionName(repoName)
	tmuxWindow := workspaceInfo["tmux_window"].(string)

	// Attach to tmux
	target := fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)

	readOnly := flags["read-only"] == "true" || flags["r"] == "true"
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

// validateWorkspaceName validates that a workspace name follows branch name restrictions
func validateWorkspaceName(name string) error {
	if name == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}

	// Git branch name restrictions
	// - Cannot start with . or -
	// - Cannot contain consecutive dots ..
	// - Cannot contain \ or any of these characters: ~ ^ : ? * [ @ { } space
	// - Cannot end with . or /
	// - Cannot be "." or ".."

	if name == "." || name == ".." {
		return fmt.Errorf("workspace name cannot be '.' or '..'")
	}

	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") {
		return fmt.Errorf("workspace name cannot start with '.' or '-'")
	}

	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, "/") {
		return fmt.Errorf("workspace name cannot end with '.' or '/'")
	}

	if strings.Contains(name, "..") {
		return fmt.Errorf("workspace name cannot contain '..'")
	}

	invalidChars := []string{"\\", "~", "^", ":", "?", "*", "[", "@", "{", "}", " ", "\t", "\n"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return fmt.Errorf("workspace name cannot contain '%s'", char)
		}
	}

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

	// Trigger immediate routing (best-effort, polling is fallback)
	client := socket.NewClient(c.paths.DaemonSock)
	_, _ = client.Send(socket.Request{Command: "route_messages"})
	// Ignore errors - 2-minute polling fallback will catch it

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

// inferRepoFromCwd infers just the repository name from the current working directory.
// Unlike inferAgentContext, it doesn't require determining the specific agent.
func (c *CLI) inferRepoFromCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Resolve symlinks in cwd for proper path comparison
	// This is especially important on macOS where /tmp -> /private/tmp
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	// Check if we're in a worktree path
	// Path format: ~/.multiclaude/wts/<repo>/<agent>
	if hasPathPrefix(cwd, c.paths.WorktreesDir) {
		rel, err := filepath.Rel(c.paths.WorktreesDir, cwd)
		if err == nil {
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) >= 1 && parts[0] != "" && parts[0] != "." {
				return parts[0], nil
			}
		}
	}

	// Check if we're in a main repo path
	// Path format: ~/.multiclaude/repos/<repo>
	if hasPathPrefix(cwd, c.paths.ReposDir) {
		rel, err := filepath.Rel(c.paths.ReposDir, cwd)
		if err == nil {
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) >= 1 && parts[0] != "" && parts[0] != "." {
				return parts[0], nil
			}
		}
	}

	return "", fmt.Errorf("not in a multiclaude directory")
}

// resolveRepo determines the repository to use based on:
// 1. Explicit --repo flag (highest priority)
// 2. Current working directory (if in a multiclaude directory)
// 3. Current repo set via 'multiclaude repo use' (lowest priority)
func (c *CLI) resolveRepo(flags map[string]string) (string, error) {
	// 1. Check explicit --repo flag
	if r, ok := flags["repo"]; ok {
		return r, nil
	}

	// 2. Try to infer from current working directory
	if inferred, err := c.inferRepoFromCwd(); err == nil {
		return inferred, nil
	}

	// 3. Check current repo from daemon
	client := socket.NewClient(c.paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "get_current_repo",
	})
	if err == nil && resp.Success {
		if currentRepo, ok := resp.Data.(string); ok && currentRepo != "" {
			return currentRepo, nil
		}
	}

	return "", fmt.Errorf("could not determine repository; use --repo flag or run 'multiclaude repo use <name>'")
}

// inferAgentContext infers the current agent and repo from working directory
func (c *CLI) inferAgentContext() (repoName, agentName string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Resolve symlinks in cwd for proper path comparison
	// This is especially important on macOS where /tmp -> /private/tmp
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	// Check if we're in a worktree path
	// Path format: ~/.multiclaude/wts/<repo>/<agent>
	if hasPathPrefix(cwd, c.paths.WorktreesDir) {
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
	if hasPathPrefix(cwd, c.paths.ReposDir) {
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

// hasPathPrefix checks if path starts with prefix using proper path semantics.
// Unlike strings.Contains or strings.HasPrefix, this ensures we're comparing
// complete path components (e.g., "/foo/bar" is under "/foo" but not under "/fo").
func hasPathPrefix(path, prefix string) bool {
	// Clean both paths to normalize them
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)

	// Check if path equals or starts with prefix followed by separator
	if path == prefix {
		return true
	}
	// Ensure prefix ends with separator for proper prefix matching
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix = prefix + string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}

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

	// Fetch the PR using GitHub's PR refs - this works for both same-repo and fork PRs
	// The refs/pull/<number>/head ref always exists and points to the PR's head commit
	fmt.Printf("Fetching PR #%s...\n", prNumber)
	prRef := fmt.Sprintf("refs/pull/%s/head", prNumber)
	localRef := fmt.Sprintf("refs/multiclaude/pr-%s", prNumber)
	cmd := exec.Command("git", "fetch", "origin", fmt.Sprintf("%s:%s", prRef, localRef))
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(errors.CategoryRuntime, fmt.Sprintf("failed to fetch PR #%s: %s", prNumber, strings.TrimSpace(string(output))), err).
			WithSuggestion("ensure the PR exists and you have access to the repository")
	}

	// Create worktree for review
	wt := worktree.NewManager(repoPath)
	wtPath := c.paths.AgentWorktree(repoName, reviewerName)
	reviewBranch := fmt.Sprintf("review/%s", reviewerName)

	fmt.Printf("Creating worktree at: %s\n", wtPath)
	if err := wt.CreateNewBranch(wtPath, reviewBranch, localRef); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Get tmux session name
	tmuxSession := sanitizeTmuxSessionName(repoName)

	// Create tmux window for reviewer (detached so it doesn't switch focus)
	fmt.Printf("Creating tmux window: %s\n", reviewerName)
	cmd = exec.Command("tmux", "new-window", "-d", "-t", tmuxSession, "-n", reviewerName, "-c", wtPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Generate session ID for reviewer
	reviewerSessionID, err := claude.GenerateSessionID()
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
		// Resolve claude binary
		claudeBinary, err := c.getClaudeBinary()
		if err != nil {
			return fmt.Errorf("failed to resolve claude binary: %w", err)
		}

		fmt.Println("Starting Claude Code in reviewer window...")
		initialMessage := fmt.Sprintf("Review PR #%s: https://github.com/%s/%s/pull/%s", prNumber, parts[1], parts[2], prNumber)
		pid, err := c.startClaudeInTmux(claudeBinary, tmuxSession, reviewerName, wtPath, reviewerSessionID, reviewerPromptFile, initialMessage)
		if err != nil {
			return fmt.Errorf("failed to start reviewer Claude: %w", err)
		}
		reviewerPID = pid

		// Set up output capture for reviewer
		if err := c.setupOutputCapture(tmuxSession, reviewerName, repoName, reviewerName, "review"); err != nil {
			fmt.Printf("Warning: failed to setup output capture for reviewer: %v\n", err)
		}
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

// Logs command implementations

func (c *CLI) viewLogs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: multiclaude logs <agent> [--lines N] [--follow]")
	}

	agentName := args[0]
	flags, _ := ParseFlags(args[1:])

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	} else {
		repos := c.getReposList()
		if len(repos) == 0 {
			return fmt.Errorf("no repositories tracked")
		}
		if len(repos) == 1 {
			repoName = repos[0]
		} else {
			return fmt.Errorf("multiple repos exist. Use --repo flag to specify which one")
		}
	}

	// Determine if it's a worker or system agent by checking if it exists in workers dir
	workerLogFile := c.paths.AgentLogFile(repoName, agentName, true)
	systemLogFile := c.paths.AgentLogFile(repoName, agentName, false)

	var logFile string
	if _, err := os.Stat(workerLogFile); err == nil {
		logFile = workerLogFile
	} else if _, err := os.Stat(systemLogFile); err == nil {
		logFile = systemLogFile
	} else {
		return fmt.Errorf("no log file found for agent %s in repo %s", agentName, repoName)
	}

	// Check for --follow flag
	if _, ok := flags["follow"]; ok {
		// Use tail -f
		cmd := exec.Command("tail", "-f", logFile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Determine number of lines
	lines := "100"
	if l, ok := flags["lines"]; ok {
		lines = l
	}

	// Use tail to get recent lines
	cmd := exec.Command("tail", "-n", lines, logFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLI) listLogs(args []string) error {
	flags, _ := ParseFlags(args)

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	}

	if repoName != "" {
		// List logs for specific repo
		return c.listLogsForRepo(repoName)
	}

	// List logs for all repos
	repos := c.getReposList()
	if len(repos) == 0 {
		fmt.Println("No repositories tracked")
		return nil
	}

	for _, repo := range repos {
		if err := c.listLogsForRepo(repo); err != nil {
			fmt.Printf("Warning: failed to list logs for %s: %v\n", repo, err)
		}
	}
	return nil
}

func (c *CLI) listLogsForRepo(repoName string) error {
	repoOutputDir := c.paths.RepoOutputDir(repoName)

	// Check if directory exists
	if _, err := os.Stat(repoOutputDir); os.IsNotExist(err) {
		fmt.Printf("No logs for %s\n", repoName)
		return nil
	}

	fmt.Printf("\n%s:\n", repoName)

	// List system agent logs
	entries, err := os.ReadDir(repoOutputDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == "workers" {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".log") {
			info, _ := entry.Info()
			agentName := strings.TrimSuffix(entry.Name(), ".log")
			if info != nil {
				fmt.Printf("  %s (%d bytes)\n", agentName, info.Size())
			} else {
				fmt.Printf("  %s\n", agentName)
			}
		}
	}

	// List worker logs
	workersDir := c.paths.WorkersOutputDir(repoName)
	if _, err := os.Stat(workersDir); err == nil {
		workerEntries, err := os.ReadDir(workersDir)
		if err == nil && len(workerEntries) > 0 {
			fmt.Println("  workers/")
			for _, entry := range workerEntries {
				if strings.HasSuffix(entry.Name(), ".log") {
					info, _ := entry.Info()
					workerName := strings.TrimSuffix(entry.Name(), ".log")
					if info != nil {
						fmt.Printf("    %s (%d bytes)\n", workerName, info.Size())
					} else {
						fmt.Printf("    %s\n", workerName)
					}
				}
			}
		}
	}

	return nil
}

func (c *CLI) searchLogs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: multiclaude logs search <pattern> [--repo <repo>]")
	}

	pattern := args[0]
	flags, _ := ParseFlags(args[1:])

	// Determine repository
	var repoName string
	if r, ok := flags["repo"]; ok {
		repoName = r
	}

	// Get search directories
	var searchPaths []string
	if repoName != "" {
		repoOutputDir := c.paths.RepoOutputDir(repoName)
		if _, err := os.Stat(repoOutputDir); err == nil {
			searchPaths = append(searchPaths, repoOutputDir)
		}
	} else {
		// Search all repos
		repos := c.getReposList()
		for _, repo := range repos {
			repoOutputDir := c.paths.RepoOutputDir(repo)
			if _, err := os.Stat(repoOutputDir); err == nil {
				searchPaths = append(searchPaths, repoOutputDir)
			}
		}
	}

	if len(searchPaths) == 0 {
		fmt.Println("No log directories found")
		return nil
	}

	// Use grep to search recursively
	grepArgs := []string{"-r", "-n", "--include=*.log", pattern}
	grepArgs = append(grepArgs, searchPaths...)

	cmd := exec.Command("grep", grepArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run grep (exit code 1 means no matches, which is fine)
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		fmt.Println("No matches found")
		return nil
	}
	return err
}

func (c *CLI) cleanLogs(args []string) error {
	flags, _ := ParseFlags(args)

	olderThan, ok := flags["older-than"]
	if !ok {
		return fmt.Errorf("usage: multiclaude logs clean --older-than <duration> (e.g., 7d, 24h)")
	}

	// Parse duration
	duration, err := parseDuration(olderThan)
	if err != nil {
		return fmt.Errorf("invalid duration: %v", err)
	}

	cutoff := time.Now().Add(-duration)
	fmt.Printf("Cleaning logs older than %s...\n", cutoff.Format(time.RFC3339))

	var deletedCount, deletedBytes int64

	// Walk output directory
	err = filepath.Walk(c.paths.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".log") {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			deletedBytes += info.Size()
			if err := os.Remove(path); err != nil {
				fmt.Printf("Warning: failed to remove %s: %v\n", path, err)
			} else {
				deletedCount++
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk output directory: %w", err)
	}

	fmt.Printf("Deleted %d files (%.2f MB)\n", deletedCount, float64(deletedBytes)/(1024*1024))
	return nil
}

// parseDuration parses a duration string like "7d", "24h", "30m"
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("duration too short")
	}

	unit := s[len(s)-1]
	valueStr := s[:len(s)-1]

	var value int
	if _, err := fmt.Sscanf(valueStr, "%d", &value); err != nil {
		return 0, err
	}

	switch unit {
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'm':
		return time.Duration(value) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unknown unit: %c (use d, h, or m)", unit)
	}
}

func (c *CLI) attachAgent(args []string) error {
	flags, remainingArgs := ParseFlags(args)
	readOnly := flags["read-only"] == "true" || flags["r"] == "true"

	// Determine repository
	repoName, err := c.resolveRepo(flags)
	if err != nil {
		return errors.NotInRepo()
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

	agents, _ := resp.Data.([]interface{})

	// Determine agent name - from args or interactive selection
	var agentName string
	if len(remainingArgs) > 0 {
		agentName = remainingArgs[0]
	} else {
		// Interactive selection - all agent types
		items := agentsToSelectableItems(agents, nil)
		if len(items) == 0 {
			return fmt.Errorf("no agents found in repo '%s'", repoName)
		}
		selected, err := SelectFromList("Select agent to attach:", items)
		if err != nil {
			return err
		}
		if selected == "" {
			fmt.Println("Cancelled")
			return nil
		}
		agentName = selected
	}

	// Find agent
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
	tmuxSession := sanitizeTmuxSessionName(repoName)
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
	cleanMerged := flags["merged"] == "true"

	if dryRun {
		fmt.Println("Running cleanup in dry-run mode (no changes will be made)...")
	} else {
		fmt.Println("Running cleanup...")
	}

	// If --merged flag is set, run merged branch cleanup
	if cleanMerged {
		return c.cleanupMergedBranches(dryRun, verbose)
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

	fmt.Println("Cleanup completed")
	return nil
}

// cleanupMergedBranches cleans up branches that have been merged upstream
func (c *CLI) cleanupMergedBranches(dryRun bool, verbose bool) error {
	fmt.Println("\nChecking for branches merged upstream...")

	// Load state to get repository list
	st, err := state.Load(c.paths.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	totalDeleted := 0
	totalFound := 0

	// Process each repository
	repos := st.ListRepos()
	if len(repos) == 0 {
		fmt.Println("No repositories tracked. Nothing to clean up.")
		return nil
	}

	for _, repoName := range repos {
		repoPath := c.paths.RepoDir(repoName)

		// Check if repo exists
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			if verbose {
				fmt.Printf("\nRepository %s: path does not exist, skipping\n", repoName)
			}
			continue
		}

		if verbose {
			fmt.Printf("\nRepository: %s\n", repoName)
		}

		wt := worktree.NewManager(repoPath)

		// Check for merged branches with common prefixes
		for _, prefix := range []string{"multiclaude/", "work/"} {
			mergedBranches, err := wt.FindMergedUpstreamBranches(prefix)
			if err != nil {
				if verbose {
					fmt.Printf("  Warning: failed to find merged branches with prefix %s: %v\n", prefix, err)
				}
				continue
			}

			if len(mergedBranches) == 0 {
				if verbose {
					fmt.Printf("  No merged branches with prefix %s\n", prefix)
				}
				continue
			}

			// Get worktrees to skip branches that are still checked out
			worktrees, err := wt.List()
			if err != nil {
				if verbose {
					fmt.Printf("  Warning: failed to list worktrees: %v\n", err)
				}
				continue
			}

			activeBranches := make(map[string]bool)
			for _, wtInfo := range worktrees {
				if wtInfo.Branch != "" {
					activeBranches[wtInfo.Branch] = true
				}
			}

			fmt.Printf("\nMerged branches with prefix %s for %s:\n", prefix, repoName)
			for _, branch := range mergedBranches {
				if activeBranches[branch] {
					if verbose {
						fmt.Printf("  Skipping %s (still checked out)\n", branch)
					}
					continue
				}

				totalFound++
				if dryRun {
					fmt.Printf("  Would delete: %s\n", branch)
				} else {
					// Delete local branch
					if err := wt.DeleteBranch(branch); err != nil {
						fmt.Printf("  Failed to delete %s: %v\n", branch, err)
						continue
					}
					fmt.Printf("  Deleted: %s\n", branch)
					totalDeleted++

					// Try to delete remote branch from origin (the fork)
					if err := wt.DeleteRemoteBranch("origin", branch); err != nil {
						if verbose {
							fmt.Printf("    (remote branch deletion failed: %v)\n", err)
						}
					} else if verbose {
						fmt.Printf("    (also deleted from origin)\n")
					}
				}
			}
		}
	}

	if dryRun {
		if totalFound > 0 {
			fmt.Printf("\nFound %d merged branch(es) that would be deleted\n", totalFound)
		} else {
			fmt.Println("\nNo merged branches found to clean up")
		}
	} else {
		if totalDeleted > 0 {
			fmt.Printf("\nDeleted %d merged branch(es)\n", totalDeleted)
		} else {
			fmt.Println("\nNo merged branches found to clean up")
		}
	}

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

			// Clean up orphaned work/* branches (branches without corresponding worktrees)
			orphanedBranches, err := wt.FindOrphanedBranches("work/")
			if err != nil && verbose {
				fmt.Printf("  Warning: failed to find orphaned branches: %v\n", err)
			} else if len(orphanedBranches) > 0 {
				fmt.Printf("\nOrphaned work branches (%d) for %s:\n", len(orphanedBranches), repoName)
				for _, branch := range orphanedBranches {
					if dryRun {
						fmt.Printf("  Would delete branch: %s\n", branch)
						totalIssues++
					} else {
						if err := wt.DeleteBranch(branch); err != nil {
							fmt.Printf("  Failed to delete %s: %v\n", branch, err)
						} else {
							fmt.Printf("  Deleted branch: %s\n", branch)
							totalRemoved++
						}
					}
				}
			} else if verbose {
				fmt.Println("  No orphaned work branches")
			}

			// Also clean up orphaned workspace/* branches
			orphanedWorkspaces, err := wt.FindOrphanedBranches("workspace/")
			if err != nil && verbose {
				fmt.Printf("  Warning: failed to find orphaned workspace branches: %v\n", err)
			} else if len(orphanedWorkspaces) > 0 {
				fmt.Printf("\nOrphaned workspace branches (%d) for %s:\n", len(orphanedWorkspaces), repoName)
				for _, branch := range orphanedWorkspaces {
					if dryRun {
						fmt.Printf("  Would delete branch: %s\n", branch)
						totalIssues++
					} else {
						if err := wt.DeleteBranch(branch); err != nil {
							fmt.Printf("  Failed to delete %s: %v\n", branch, err)
						} else {
							fmt.Printf("  Deleted branch: %s\n", branch)
							totalRemoved++
						}
					}
				}
			} else if verbose {
				fmt.Println("  No orphaned workspace branches")
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

// restartClaude restarts Claude in the current agent context.
// It auto-detects whether to use --resume or --session-id based on session history.
func (c *CLI) restartClaude(args []string) error {
	// Infer agent context from cwd
	repoName, agentName, err := c.inferAgentContext()
	if err != nil {
		return fmt.Errorf("cannot determine agent context: %w\n\nRun this command from within a multiclaude agent tmux window", err)
	}

	// Load state to get session ID
	st, err := state.Load(c.paths.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	agent, exists := st.GetAgent(repoName, agentName)
	if !exists {
		return fmt.Errorf("agent '%s' not found in state for repo '%s'", agentName, repoName)
	}

	if agent.SessionID == "" {
		return fmt.Errorf("agent has no session ID - try removing and recreating the agent")
	}

	// Get the prompt file path (stored as ~/.multiclaude/prompts/<agent-name>.md)
	promptFile := filepath.Join(c.paths.Root, "prompts", agentName+".md")

	// Check if the session has history by looking for the .jsonl file
	// Claude stores sessions in ~/.claude/projects/<encoded-path>/<session-id>.jsonl
	claudeProjectsDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	hasHistory := false

	// The path encoding replaces / with - and prefixes with -
	// e.g., /Users/foo/bar becomes -Users-foo-bar
	encodedPath := strings.ReplaceAll(agent.WorktreePath, "/", "-")
	sessionFile := filepath.Join(claudeProjectsDir, encodedPath, agent.SessionID+".jsonl")

	if info, err := os.Stat(sessionFile); err == nil {
		// Check if file has content (not just empty)
		if info.Size() > 0 {
			hasHistory = true
		}
	}

	// Build the command
	var cmdArgs []string
	if hasHistory {
		// Session has history - use --resume to continue
		cmdArgs = []string{"--resume", agent.SessionID}
		fmt.Printf("Resuming Claude session %s...\n", agent.SessionID)
	} else {
		// New session - use --session-id
		cmdArgs = []string{"--session-id", agent.SessionID}
		fmt.Printf("Starting new Claude session %s...\n", agent.SessionID)
	}

	// Add common flags
	cmdArgs = append(cmdArgs, "--dangerously-skip-permissions")
	if _, err := os.Stat(promptFile); err == nil {
		cmdArgs = append(cmdArgs, "--append-system-prompt-file", promptFile)
	}

	// Exec claude
	claudePath := "claude"

	fmt.Printf("Running: %s %s\n\n", claudePath, strings.Join(cmdArgs, " "))

	// Run claude interactively
	cmd := exec.Command(claudePath, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = agent.WorktreePath

	return cmd.Run()
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

// writeMergeQueuePromptFile writes a merge-queue prompt file with tracking mode configuration
func (c *CLI) writeMergeQueuePromptFile(repoPath string, agentName string, mqConfig state.MergeQueueConfig) (string, error) {
	// Get the complete prompt (default + custom + CLI docs)
	promptText, err := prompts.GetPrompt(repoPath, prompts.TypeMergeQueue, c.documentation)
	if err != nil {
		return "", fmt.Errorf("failed to get prompt: %w", err)
	}

	// Add tracking mode configuration to the prompt
	trackingConfig := c.generateTrackingModePrompt(mqConfig.TrackMode)
	promptText = trackingConfig + "\n\n" + promptText

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

// generateTrackingModePrompt generates prompt text explaining which PRs to track based on tracking mode
func (c *CLI) generateTrackingModePrompt(trackMode state.TrackMode) string {
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

// setupOutputCapture sets up tmux pipe-pane to capture agent output to a log file.
// It creates the necessary directories and starts the pipe-pane command.
// The agentType should be "worker" for worker agents, anything else for system agents.
func (c *CLI) setupOutputCapture(tmuxSession, tmuxWindow, repoName, agentName, agentType string) error {
	// Determine log file path based on agent type
	isWorker := agentType == "worker" || agentType == "review"
	logFile := c.paths.AgentLogFile(repoName, agentName, isWorker)

	// Ensure directory exists
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Set up pipe-pane
	tmuxClient := tmux.NewClient()
	if err := tmuxClient.StartPipePane(tmuxSession, tmuxWindow, logFile); err != nil {
		return fmt.Errorf("failed to start output capture: %w", err)
	}

	return nil
}

// startClaudeInTmux starts Claude Code in a tmux window with the given configuration
// Returns the PID of the Claude process
func (c *CLI) startClaudeInTmux(binaryPath, tmuxSession, tmuxWindow, workDir, sessionID, promptFile string, initialMessage string) (int, error) {
	// Build Claude command using the full path to prevent version drift
	claudeCmd := fmt.Sprintf("%s --session-id %s --dangerously-skip-permissions", binaryPath, sessionID)

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

		// Send message using atomic method to avoid race conditions (issue #63)
		// The atomic method sends text + Enter in a single exec call
		if err := tmuxClient.SendKeysLiteralWithEnter(tmuxSession, tmuxWindow, initialMessage); err != nil {
			return pid, fmt.Errorf("failed to send initial message to Claude: %w", err)
		}
	}

	return pid, nil
}


// bugReport generates a diagnostic bug report with redacted sensitive information
func (c *CLI) bugReport(args []string) error {
	flags, positionalArgs := ParseFlags(args)

	// Check for verbose flag
	verbose := flags["verbose"] == "true" || flags["v"] == "true"

	// Get optional description from positional args
	description := ""
	if len(positionalArgs) > 0 {
		description = strings.Join(positionalArgs, " ")
	}

	// Create collector and generate report
	collector := bugreport.NewCollector(c.paths, Version)
	report, err := collector.Collect(description, verbose)
	if err != nil {
		return fmt.Errorf("failed to collect diagnostic information: %w", err)
	}

	// Format as Markdown
	markdown := bugreport.FormatMarkdown(report)

	// Check if output file specified
	if outputFile, ok := flags["output"]; ok {
		if err := os.WriteFile(outputFile, []byte(markdown), 0644); err != nil {
			return fmt.Errorf("failed to write report to %s: %w", outputFile, err)
		}
		fmt.Printf("Bug report written to: %s\n", outputFile)
		return nil
	}

	// Print to stdout
	fmt.Print(markdown)
	return nil
}
