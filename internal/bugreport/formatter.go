package bugreport

import (
	"fmt"
	"strings"
)

// FormatMarkdown formats the report as a Markdown document
func FormatMarkdown(report *Report) string {
	var sb strings.Builder

	// Title
	sb.WriteString("# Multiclaude Bug Report\n\n")

	// Description (if provided)
	if report.Description != "" {
		sb.WriteString("## Description\n\n")
		sb.WriteString(report.Description)
		sb.WriteString("\n\n")
	}

	// Environment section
	sb.WriteString("## Environment\n\n")
	sb.WriteString("| Property | Value |\n")
	sb.WriteString("|----------|-------|\n")
	sb.WriteString(fmt.Sprintf("| multiclaude version | %s |\n", report.Version))
	sb.WriteString(fmt.Sprintf("| Go version | %s |\n", report.GoVersion))
	sb.WriteString(fmt.Sprintf("| OS | %s |\n", report.OS))
	sb.WriteString(fmt.Sprintf("| Architecture | %s |\n", report.Arch))
	sb.WriteString("\n")

	// Tool versions section
	sb.WriteString("## Tool Versions\n\n")
	sb.WriteString("| Tool | Status |\n")
	sb.WriteString("|------|--------|\n")
	sb.WriteString(fmt.Sprintf("| tmux | %s |\n", report.TmuxVersion))
	sb.WriteString(fmt.Sprintf("| git | %s |\n", report.GitVersion))
	claudeStatus := "not found"
	if report.ClaudeExists {
		claudeStatus = "installed"
	}
	sb.WriteString(fmt.Sprintf("| claude CLI | %s |\n", claudeStatus))
	sb.WriteString("\n")

	// Daemon status section
	sb.WriteString("## Daemon Status\n\n")
	if report.DaemonRunning {
		sb.WriteString(fmt.Sprintf("- **Status**: Running (PID: %d)\n", report.DaemonPID))
	} else if report.DaemonPID > 0 {
		sb.WriteString(fmt.Sprintf("- **Status**: Not running (stale PID: %d)\n", report.DaemonPID))
	} else {
		sb.WriteString("- **Status**: Not running\n")
	}
	sb.WriteString("\n")

	// Statistics section
	sb.WriteString("## Statistics\n\n")
	sb.WriteString("| Metric | Count |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Repositories | %d |\n", report.RepoCount))
	sb.WriteString(fmt.Sprintf("| Workers | %d |\n", report.WorkerCount))
	sb.WriteString(fmt.Sprintf("| Supervisors | %d |\n", report.SupervisorCount))
	sb.WriteString(fmt.Sprintf("| Merge Queues | %d |\n", report.MergeQueueCount))
	sb.WriteString(fmt.Sprintf("| Workspaces | %d |\n", report.WorkspaceCount))
	sb.WriteString(fmt.Sprintf("| Review Agents | %d |\n", report.ReviewAgentCount))
	sb.WriteString("\n")

	// Verbose per-repo breakdown
	if report.Verbose && len(report.RepoStats) > 0 {
		sb.WriteString("### Per-Repository Breakdown\n\n")
		sb.WriteString("| Repository | Workers | Supervisor | Merge Queue | Workspaces |\n")
		sb.WriteString("|------------|---------|------------|-------------|------------|\n")
		for _, repo := range report.RepoStats {
			supervisor := "no"
			if repo.HasSupervisor {
				supervisor = "yes"
			}
			mergeQueue := "no"
			if repo.HasMergeQueue {
				mergeQueue = "yes"
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s | %s | %d |\n",
				repo.Name, repo.WorkerCount, supervisor, mergeQueue, repo.WorkspaceCount))
		}
		sb.WriteString("\n")
	}

	// Daemon log section
	sb.WriteString("## Daemon Log (last 50 lines, redacted)\n\n")
	sb.WriteString("```\n")
	sb.WriteString(report.DaemonLogTail)
	if !strings.HasSuffix(report.DaemonLogTail, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")

	return sb.String()
}
