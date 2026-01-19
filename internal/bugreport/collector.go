package bugreport

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/dlorenc/multiclaude/internal/redact"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/pkg/config"
)

// Report contains all collected diagnostic information
type Report struct {
	Description string
	Verbose     bool

	// Environment
	Version   string
	GoVersion string
	OS        string
	Arch      string

	// Tool versions
	TmuxVersion  string
	GitVersion   string
	ClaudeExists bool

	// Daemon status
	DaemonRunning bool
	DaemonPID     int

	// Statistics
	RepoCount        int
	WorkerCount      int
	SupervisorCount  int
	MergeQueueCount  int
	WorkspaceCount   int
	ReviewAgentCount int

	// Verbose stats (per-repo breakdown)
	RepoStats []RepoStat

	// Logs
	DaemonLogTail string
}

// RepoStat contains per-repo statistics for verbose mode
type RepoStat struct {
	Name         string // redacted
	WorkerCount  int
	HasSupervisor bool
	HasMergeQueue bool
	WorkspaceCount int
}

// Collector gathers diagnostic information
type Collector struct {
	paths    *config.Paths
	redactor *redact.Redactor
	version  string
}

// NewCollector creates a new diagnostic collector
func NewCollector(paths *config.Paths, version string) *Collector {
	return &Collector{
		paths:    paths,
		redactor: redact.New(),
		version:  version,
	}
}

// Collect gathers all diagnostic information
func (c *Collector) Collect(description string, verbose bool) (*Report, error) {
	report := &Report{
		Description: description,
		Verbose:     verbose,
		Version:     c.version,
		GoVersion:   runtime.Version(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
	}

	// Collect tool versions
	report.TmuxVersion = c.getTmuxVersion()
	report.GitVersion = c.getGitVersion()
	report.ClaudeExists = c.checkClaudeExists()

	// Check daemon status
	report.DaemonRunning, report.DaemonPID = c.checkDaemonStatus()

	// Load state and count agents
	if err := c.collectAgentStats(report); err != nil {
		// Non-fatal: continue with zero counts
	}

	// Collect daemon log tail
	report.DaemonLogTail = c.collectDaemonLog()

	return report, nil
}

// getTmuxVersion returns the tmux version or an error message
func (c *Collector) getTmuxVersion() string {
	cmd := exec.Command("tmux", "-V")
	output, err := cmd.Output()
	if err != nil {
		return "not installed"
	}
	return strings.TrimSpace(string(output))
}

// getGitVersion returns the git version or an error message
func (c *Collector) getGitVersion() string {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "not installed"
	}
	return strings.TrimSpace(string(output))
}

// checkClaudeExists checks if the claude CLI is available
func (c *Collector) checkClaudeExists() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// checkDaemonStatus checks if the daemon is running
func (c *Collector) checkDaemonStatus() (bool, int) {
	pidData, err := os.ReadFile(c.paths.DaemonPID)
	if err != nil {
		return false, 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return false, 0
	}

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid
	}

	// On Unix, FindProcess always succeeds, so we send signal 0 to check
	err = process.Signal(os.Signal(nil))
	if err != nil {
		return false, pid
	}

	return true, pid
}

// collectAgentStats loads state and counts agents
func (c *Collector) collectAgentStats(report *Report) error {
	st, err := state.Load(c.paths.StateFile)
	if err != nil {
		return err
	}

	repos := st.GetAllRepos()
	report.RepoCount = len(repos)

	for repoName, repo := range repos {
		repoStat := RepoStat{
			Name: c.redactor.RepoName(repoName),
		}

		for _, agent := range repo.Agents {
			switch agent.Type {
			case state.AgentTypeWorker:
				report.WorkerCount++
				repoStat.WorkerCount++
			case state.AgentTypeSupervisor:
				report.SupervisorCount++
				repoStat.HasSupervisor = true
			case state.AgentTypeMergeQueue:
				report.MergeQueueCount++
				repoStat.HasMergeQueue = true
			case state.AgentTypeWorkspace:
				report.WorkspaceCount++
				repoStat.WorkspaceCount++
			case state.AgentTypeReview:
				report.ReviewAgentCount++
			}
		}

		if report.Verbose {
			report.RepoStats = append(report.RepoStats, repoStat)
		}
	}

	return nil
}

// collectDaemonLog reads the last 50 lines of daemon.log and redacts them
func (c *Collector) collectDaemonLog() string {
	data, err := os.ReadFile(c.paths.DaemonLog)
	if err != nil {
		return "(no log file found)"
	}

	lines := strings.Split(string(data), "\n")

	// Get last 50 lines
	start := 0
	if len(lines) > 50 {
		start = len(lines) - 50
	}
	tail := strings.Join(lines[start:], "\n")

	// Redact sensitive information
	return c.redactor.Text(tail)
}

