package provider

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// GitHub URL patterns
var (
	githubHTTPSRegex = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/.]+)(?:\.git)?$`)
	githubSSHRegex   = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/.]+)(?:\.git)?$`)
)

// GitHub implements the Provider interface for GitHub.
type GitHub struct{}

// NewGitHub creates a new GitHub provider.
func NewGitHub() *GitHub {
	return &GitHub{}
}

// Name returns the provider type.
func (g *GitHub) Name() Type {
	return TypeGitHub
}

// ParseURL parses a GitHub repository URL.
func (g *GitHub) ParseURL(url string) (*RepoInfo, error) {
	url = normalizeGitURL(url)

	// Try HTTPS format: https://github.com/owner/repo
	if matches := githubHTTPSRegex.FindStringSubmatch(url + ".git"); matches != nil {
		return &RepoInfo{
			Provider: TypeGitHub,
			Owner:    matches[1],
			Repo:     matches[2],
			CloneURL: fmt.Sprintf("https://github.com/%s/%s", matches[1], matches[2]),
		}, nil
	}

	// Try SSH format: git@github.com:owner/repo
	if matches := githubSSHRegex.FindStringSubmatch(url + ".git"); matches != nil {
		return &RepoInfo{
			Provider: TypeGitHub,
			Owner:    matches[1],
			Repo:     matches[2],
			CloneURL: fmt.Sprintf("https://github.com/%s/%s", matches[1], matches[2]),
		}, nil
	}

	return nil, fmt.Errorf("unable to parse GitHub URL: %s", url)
}

// DetectFork checks if a GitHub repository is a fork.
func (g *GitHub) DetectFork(repoPath string) (*ForkInfo, error) {
	// First check for upstream remote (common convention)
	upstreamURL, err := getRemoteURL(repoPath, "upstream")
	if err == nil && upstreamURL != "" {
		info, err := g.ParseURL(upstreamURL)
		if err == nil {
			return &ForkInfo{
				IsFork:        true,
				UpstreamOwner: info.Owner,
				UpstreamRepo:  info.Repo,
				UpstreamURL:   info.CloneURL,
			}, nil
		}
	}

	// Get origin remote URL
	originURL, err := getRemoteURL(repoPath, "origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get origin remote: %w", err)
	}

	// Parse origin URL
	originInfo, err := g.ParseURL(originURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse origin URL: %w", err)
	}

	// Try to detect via GitHub API using gh CLI
	forkInfo, err := g.detectForkViaAPI(originInfo.Owner, originInfo.Repo)
	if err == nil && forkInfo.IsFork {
		return forkInfo, nil
	}

	return &ForkInfo{IsFork: false}, nil
}

// detectForkViaAPI uses the gh CLI to check if a repo is a fork.
func (g *GitHub) detectForkViaAPI(owner, repo string) (*ForkInfo, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", owner, repo),
		"--jq", "{fork: .fork, parent_owner: .parent.owner.login, parent_repo: .parent.name, parent_url: .parent.clone_url}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api failed: %w", err)
	}

	var result struct {
		Fork        bool   `json:"fork"`
		ParentOwner string `json:"parent_owner"`
		ParentRepo  string `json:"parent_repo"`
		ParentURL   string `json:"parent_url"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse gh api output: %w", err)
	}

	info := &ForkInfo{
		IsFork: result.Fork,
	}

	if result.Fork {
		info.UpstreamOwner = result.ParentOwner
		info.UpstreamRepo = result.ParentRepo
		info.UpstreamURL = result.ParentURL
	}

	return info, nil
}

// PRListCommand returns the gh pr list command.
func (g *GitHub) PRListCommand(label string, authorFilter string) string {
	cmd := "gh pr list"
	if authorFilter != "" {
		cmd += fmt.Sprintf(" --author %s", authorFilter)
	}
	if label != "" {
		cmd += fmt.Sprintf(" --label %s", label)
	}
	return cmd
}

// PRCreateCommand returns the gh pr create command.
func (g *GitHub) PRCreateCommand(targetRepo, headBranch string) string {
	cmd := "gh pr create"
	if targetRepo != "" {
		cmd += fmt.Sprintf(" --repo %s", targetRepo)
		if headBranch != "" {
			cmd += fmt.Sprintf(" --head %s", headBranch)
		}
	}
	return cmd
}

// PRViewCommand returns the gh pr view command.
func (g *GitHub) PRViewCommand(prNumber int, jsonFields string) string {
	cmd := fmt.Sprintf("gh pr view %d", prNumber)
	if jsonFields != "" {
		cmd += fmt.Sprintf(" --json %s", jsonFields)
	}
	return cmd
}

// PRChecksCommand returns the gh pr checks command.
func (g *GitHub) PRChecksCommand(prNumber int) string {
	return fmt.Sprintf("gh pr checks %d", prNumber)
}

// PRCommentCommand returns the gh pr comment command.
func (g *GitHub) PRCommentCommand(prNumber int, body string) string {
	return fmt.Sprintf("gh pr comment %d --body %q", prNumber, body)
}

// PREditCommand returns the gh pr edit command.
func (g *GitHub) PREditCommand(prNumber int, addLabel string) string {
	cmd := fmt.Sprintf("gh pr edit %d", prNumber)
	if addLabel != "" {
		cmd += fmt.Sprintf(" --add-label %q", addLabel)
	}
	return cmd
}

// PRMergeCommand returns the gh pr merge command.
func (g *GitHub) PRMergeCommand(prNumber int) string {
	return fmt.Sprintf("gh pr merge %d --merge --delete-branch", prNumber)
}

// RunListCommand returns the gh run list command.
func (g *GitHub) RunListCommand(branch string, limit int) string {
	cmd := "gh run list"
	if branch != "" {
		cmd += fmt.Sprintf(" --branch %s", branch)
	}
	if limit > 0 {
		cmd += fmt.Sprintf(" --limit %d", limit)
	}
	return cmd
}

// APICommand returns the gh api command.
func (g *GitHub) APICommand(endpoint, jqFilter string) string {
	cmd := fmt.Sprintf("gh api %s", endpoint)
	if jqFilter != "" {
		cmd += fmt.Sprintf(" --jq %q", jqFilter)
	}
	return cmd
}

// ReviewCommand returns the multiclaude review command.
func (g *GitHub) ReviewCommand(prURL string) string {
	return fmt.Sprintf("multiclaude review %s", prURL)
}

// getRemoteURL returns the URL of a git remote.
func getRemoteURL(repoPath, remoteName string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", remoteName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
