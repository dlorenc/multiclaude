package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/socket"
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
	rootCmd *Command
	paths   *config.Paths
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
	return cli, nil
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

	// Check for subcommands
	if subcmd, exists := cmd.Subcommands[args[0]]; exists {
		return c.executeCommand(subcmd, args[1:])
	}

	// No subcommand found, run this command with args
	if cmd.Run != nil {
		return cmd.Run(args)
	}

	return fmt.Errorf("unknown command: %s", args[0])
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
		Run:         c.cleanup,
	}

	c.rootCmd.Subcommands["repair"] = &Command{
		Name:        "repair",
		Description: "Repair state after crash",
		Run:         c.repair,
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

func (c *CLI) initRepo(args []string) error {
	fmt.Println("Initializing repository... (not yet implemented)")
	return nil
}

func (c *CLI) listRepos(args []string) error {
	fmt.Println("Listing repositories... (not yet implemented)")
	return nil
}

func (c *CLI) createWorker(args []string) error {
	fmt.Println("Creating worker... (not yet implemented)")
	return nil
}

func (c *CLI) listWorkers(args []string) error {
	fmt.Println("Listing workers... (not yet implemented)")
	return nil
}

func (c *CLI) removeWorker(args []string) error {
	fmt.Println("Removing worker... (not yet implemented)")
	return nil
}

func (c *CLI) sendMessage(args []string) error {
	fmt.Println("Sending message... (not yet implemented)")
	return nil
}

func (c *CLI) listMessages(args []string) error {
	fmt.Println("Listing messages... (not yet implemented)")
	return nil
}

func (c *CLI) readMessage(args []string) error {
	fmt.Println("Reading message... (not yet implemented)")
	return nil
}

func (c *CLI) ackMessage(args []string) error {
	fmt.Println("Acknowledging message... (not yet implemented)")
	return nil
}

func (c *CLI) completeWorker(args []string) error {
	fmt.Println("Completing worker... (not yet implemented)")
	return nil
}

func (c *CLI) attachAgent(args []string) error {
	fmt.Println("Attaching to agent... (not yet implemented)")
	return nil
}

func (c *CLI) cleanup(args []string) error {
	fmt.Println("Cleaning up... (not yet implemented)")
	return nil
}

func (c *CLI) repair(args []string) error {
	fmt.Println("Repairing state... (not yet implemented)")
	return nil
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
