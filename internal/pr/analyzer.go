package pr

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ClosedPR represents a closed pull request from GitHub
type ClosedPR struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	HeadRefName string    `json:"headRefName"`
	Author      string    `json:"author"`
	ClosedAt    time.Time `json:"closedAt"`
	URL         string    `json:"url"`
	Additions   int       `json:"additions"`
	Deletions   int       `json:"deletions"`
	// Analysis results
	HasCIFailures    bool
	HasReviewComments bool
	IsSuperseded     bool
	RecoveryReason   string
	CleanupReason    string
}

// RecoveryDecision represents whether a PR should be recovered or cleaned up
type RecoveryDecision struct {
	ShouldRecover bool
	Reason        string
	PRNumber      int
	Title         string
	Branch        string
}

// Analyzer handles closed PR analysis
type Analyzer struct {
	repoDir string
}

// NewAnalyzer creates a new PR analyzer
func NewAnalyzer(repoDir string) *Analyzer {
	return &Analyzer{repoDir: repoDir}
}

// ListClosedPRs returns all closed (not merged) PRs with the multiclaude label
func (a *Analyzer) ListClosedPRs() ([]ClosedPR, error) {
	// Use gh CLI to list closed PRs with multiclaude label
	cmd := exec.Command("gh", "pr", "list",
		"--state", "closed",
		"--label", "multiclaude",
		"--json", "number,title,headRefName,author,closedAt,url,additions,deletions,mergedAt",
		"--limit", "100",
	)
	cmd.Dir = a.repoDir

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run gh pr list: %w", err)
	}

	var rawPRs []struct {
		Number      int       `json:"number"`
		Title       string    `json:"title"`
		HeadRefName string    `json:"headRefName"`
		Author      struct {
			Login string `json:"login"`
		} `json:"author"`
		ClosedAt  time.Time  `json:"closedAt"`
		MergedAt  *time.Time `json:"mergedAt"`
		URL       string     `json:"url"`
		Additions int        `json:"additions"`
		Deletions int        `json:"deletions"`
	}

	if err := json.Unmarshal(output, &rawPRs); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	// Filter to only closed (not merged) PRs
	var closedPRs []ClosedPR
	for _, raw := range rawPRs {
		// Skip merged PRs - we only want closed without merge
		if raw.MergedAt != nil {
			continue
		}

		closedPRs = append(closedPRs, ClosedPR{
			Number:      raw.Number,
			Title:       raw.Title,
			HeadRefName: raw.HeadRefName,
			Author:      raw.Author.Login,
			ClosedAt:    raw.ClosedAt,
			URL:         raw.URL,
			Additions:   raw.Additions,
			Deletions:   raw.Deletions,
		})
	}

	return closedPRs, nil
}

// AnalyzePR analyzes a closed PR to determine if it has recoverable work
func (a *Analyzer) AnalyzePR(pr *ClosedPR) error {
	// Check for CI failures
	hasFailures, err := a.checkCIStatus(pr.Number)
	if err != nil {
		// If we can't check CI, assume there were failures
		pr.HasCIFailures = true
	} else {
		pr.HasCIFailures = hasFailures
	}

	// Check for review comments
	hasComments, err := a.checkReviewComments(pr.Number)
	if err != nil {
		pr.HasReviewComments = false
	} else {
		pr.HasReviewComments = hasComments
	}

	// Check if superseded by another PR
	isSuperseded, err := a.checkIfSuperseded(pr)
	if err != nil {
		pr.IsSuperseded = false
	} else {
		pr.IsSuperseded = isSuperseded
	}

	return nil
}

// checkCIStatus checks if the PR had CI failures
func (a *Analyzer) checkCIStatus(prNumber int) (bool, error) {
	cmd := exec.Command("gh", "pr", "checks", strconv.Itoa(prNumber), "--json", "state")
	cmd.Dir = a.repoDir

	output, err := cmd.Output()
	if err != nil {
		return true, err
	}

	var checks []struct {
		State string `json:"state"`
	}

	if err := json.Unmarshal(output, &checks); err != nil {
		return true, err
	}

	// Check if any checks failed
	for _, check := range checks {
		if check.State == "FAILURE" || check.State == "ERROR" {
			return true, nil
		}
	}

	return false, nil
}

// checkReviewComments checks if the PR has substantive review comments
func (a *Analyzer) checkReviewComments(prNumber int) (bool, error) {
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", prNumber),
		"--jq", "length",
	)
	cmd.Dir = a.repoDir

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// checkIfSuperseded checks if another PR has been merged that addresses the same work
func (a *Analyzer) checkIfSuperseded(pr *ClosedPR) (bool, error) {
	// Check if there's a merged PR with similar title or from the same branch pattern
	// This is a heuristic - look for PRs with similar keywords in title

	// Get keywords from the PR title
	titleWords := strings.Fields(strings.ToLower(pr.Title))
	if len(titleWords) < 2 {
		return false, nil
	}

	// Search for merged PRs with similar titles
	cmd := exec.Command("gh", "pr", "list",
		"--state", "merged",
		"--json", "title,mergedAt",
		"--limit", "20",
	)
	cmd.Dir = a.repoDir

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	var mergedPRs []struct {
		Title    string    `json:"title"`
		MergedAt time.Time `json:"mergedAt"`
	}

	if err := json.Unmarshal(output, &mergedPRs); err != nil {
		return false, err
	}

	// Check if any merged PR after this one was closed shares significant keywords
	for _, merged := range mergedPRs {
		if merged.MergedAt.Before(pr.ClosedAt) {
			continue
		}

		mergedWords := strings.Fields(strings.ToLower(merged.Title))
		matchCount := 0
		for _, word := range titleWords {
			if len(word) < 4 { // Skip short words
				continue
			}
			for _, mWord := range mergedWords {
				if word == mWord {
					matchCount++
					break
				}
			}
		}

		// If more than 40% of significant words match, consider it superseded
		if len(titleWords) > 0 && float64(matchCount)/float64(len(titleWords)) > 0.4 {
			return true, nil
		}
	}

	return false, nil
}

// DecideRecovery determines whether a closed PR should be recovered or cleaned up
func (a *Analyzer) DecideRecovery(pr *ClosedPR) RecoveryDecision {
	totalChanges := pr.Additions + pr.Deletions

	decision := RecoveryDecision{
		PRNumber: pr.Number,
		Title:    pr.Title,
		Branch:   pr.HeadRefName,
	}

	// Rule 1: If superseded, clean up
	if pr.IsSuperseded {
		decision.ShouldRecover = false
		decision.Reason = "Superseded by a merged PR with similar work"
		return decision
	}

	// Rule 2: Trivial changes (< 20 lines) - clean up unless has review comments
	if totalChanges < 20 && !pr.HasReviewComments {
		decision.ShouldRecover = false
		decision.Reason = fmt.Sprintf("Trivial changes (%d lines) with no review feedback", totalChanges)
		return decision
	}

	// Rule 3: Substantial changes (> 50 lines) - recover
	if totalChanges > 50 {
		decision.ShouldRecover = true
		if pr.HasCIFailures {
			decision.Reason = fmt.Sprintf("Substantial work (%d lines) - CI failed but work may be salvageable", totalChanges)
		} else {
			decision.Reason = fmt.Sprintf("Substantial work (%d lines) - investigate why PR was closed", totalChanges)
		}
		return decision
	}

	// Rule 4: Medium changes with review comments - recover
	if pr.HasReviewComments {
		decision.ShouldRecover = true
		decision.Reason = fmt.Sprintf("Has review feedback worth preserving (%d lines changed)", totalChanges)
		return decision
	}

	// Rule 5: Medium changes, CI failing consistently - clean up
	if pr.HasCIFailures && totalChanges <= 50 {
		decision.ShouldRecover = false
		decision.Reason = fmt.Sprintf("CI failures with moderate changes (%d lines)", totalChanges)
		return decision
	}

	// Default: Recover medium-sized work
	decision.ShouldRecover = true
	decision.Reason = fmt.Sprintf("Moderate work (%d lines) may be worth investigating", totalChanges)
	return decision
}

// DeleteBranch deletes a remote branch
func (a *Analyzer) DeleteBranch(branchName string) error {
	// Check if branch exists on remote
	cmd := exec.Command("git", "ls-remote", "--heads", "origin", branchName)
	cmd.Dir = a.repoDir

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check remote branch: %w", err)
	}

	if len(strings.TrimSpace(string(output))) == 0 {
		// Branch doesn't exist on remote, nothing to do
		return nil
	}

	// Delete the remote branch
	cmd = exec.Command("git", "push", "origin", "--delete", branchName)
	cmd.Dir = a.repoDir

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete branch: %s", string(output))
	}

	return nil
}

// CreateRecoveryIssue creates a GitHub issue to track recovery of closed PR work
func (a *Analyzer) CreateRecoveryIssue(pr *ClosedPR, reason string) (string, error) {
	title := fmt.Sprintf("Recover work from closed PR #%d: %s", pr.Number, pr.Title)

	body := fmt.Sprintf(`## Closed PR Recovery

This issue was automatically created to track potential recovery of work from a closed PR.

### Original PR
- **PR**: #%d
- **Title**: %s
- **URL**: %s
- **Branch**: %s
- **Changes**: +%d / -%d lines
- **Closed**: %s

### Recovery Reason
%s

### Recommended Actions
1. Review the original PR to understand the work that was done
2. Determine if the work is still relevant
3. If relevant, create a new worker to complete or adapt the work
4. Close this issue when resolved

---
*This issue was automatically created by multiclaude's closed PR cleanup system.*
`,
		pr.Number,
		pr.Title,
		pr.URL,
		pr.HeadRefName,
		pr.Additions,
		pr.Deletions,
		pr.ClosedAt.Format(time.RFC3339),
		reason,
	)

	cmd := exec.Command("gh", "issue", "create",
		"--title", title,
		"--body", body,
		"--label", "multiclaude,recovery",
	)
	cmd.Dir = a.repoDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create issue: %s", string(output))
	}

	// Extract issue URL from output
	issueURL := strings.TrimSpace(string(output))
	return issueURL, nil
}
