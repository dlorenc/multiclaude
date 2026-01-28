// Package fork provides fork detection utilities for Git repositories.
package fork

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ForkInfo contains information about whether a repository is a fork
// and its relationship to upstream.
type ForkInfo struct {
	// IsFork is true if the repository is a fork of another repository
	IsFork bool `json:"is_fork"`

	// OriginURL is the URL of the origin remote (the user's repo)
	OriginURL string `json:"origin_url"`

	// UpstreamURL is the URL of the upstream remote (the original repo, if fork)
	UpstreamURL string `json:"upstream_url,omitempty"`

	// OriginOwner is the owner (user/org) of the origin repository
	OriginOwner string `json:"origin_owner"`

	// OriginRepo is the name of the origin repository
	OriginRepo string `json:"origin_repo"`

	// UpstreamOwner is the owner of the upstream repository (if fork)
	UpstreamOwner string `json:"upstream_owner,omitempty"`

	// UpstreamRepo is the name of the upstream repository (if fork)
	UpstreamRepo string `json:"upstream_repo,omitempty"`
}

// DetectFork analyzes a git repository to determine if it's a fork.
// It uses multiple detection strategies:
// 1. Check for "upstream" git remote (common convention)
// 2. Query GitHub API for fork status (most reliable)
//
// The repoPath should be the path to the git repository root.
func DetectFork(repoPath string) (*ForkInfo, error) {
	// Get origin remote URL
	originURL, err := getRemoteURL(repoPath, "origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get origin remote: %w", err)
	}

	// Parse origin URL
	originOwner, originRepo, err := ParseGitHubURL(originURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse origin URL: %w", err)
	}

	info := &ForkInfo{
		IsFork:      false,
		OriginURL:   originURL,
		OriginOwner: originOwner,
		OriginRepo:  originRepo,
	}

	// Check for upstream remote (common fork convention)
	upstreamURL, err := getRemoteURL(repoPath, "upstream")
	if err == nil && upstreamURL != "" {
		// Upstream remote exists - this is a fork
		upstreamOwner, upstreamRepo, err := ParseGitHubURL(upstreamURL)
		if err == nil {
			info.IsFork = true
			info.UpstreamURL = upstreamURL
			info.UpstreamOwner = upstreamOwner
			info.UpstreamRepo = upstreamRepo
			return info, nil
		}
	}

	// Try to detect via GitHub API using gh CLI
	forkInfo, err := detectForkViaGitHubAPI(originOwner, originRepo)
	if err == nil && forkInfo.IsFork {
		info.IsFork = true
		info.UpstreamURL = forkInfo.UpstreamURL
		info.UpstreamOwner = forkInfo.UpstreamOwner
		info.UpstreamRepo = forkInfo.UpstreamRepo
	}

	return info, nil
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

// ParseGitHubURL extracts owner and repo from a GitHub URL.
// Supports both HTTPS and SSH formats:
// - https://github.com/owner/repo.git
// - https://github.com/owner/repo
// - git@github.com:owner/repo.git
// - git@github.com:owner/repo
func ParseGitHubURL(url string) (owner, repo string, err error) {
	// HTTPS format: https://github.com/owner/repo(.git)?
	// Note: repo name can contain dots (e.g., demos.expanso.io)
	httpsRegex := regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := httpsRegex.FindStringSubmatch(url); matches != nil {
		return matches[1], matches[2], nil
	}

	// SSH format: git@github.com:owner/repo(.git)?
	// Note: repo name can contain dots (e.g., demos.expanso.io)
	sshRegex := regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := sshRegex.FindStringSubmatch(url); matches != nil {
		return matches[1], matches[2], nil
	}

	return "", "", fmt.Errorf("unable to parse GitHub URL: %s", url)
}

// detectForkViaGitHubAPI uses the gh CLI to check if a repo is a fork.
func detectForkViaGitHubAPI(owner, repo string) (*ForkInfo, error) {
	// Use gh api to get repo info
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

// AddUpstreamRemote adds an upstream remote to a git repository.
func AddUpstreamRemote(repoPath, upstreamURL string) error {
	// Check if upstream already exists
	_, err := getRemoteURL(repoPath, "upstream")
	if err == nil {
		// Upstream already exists - update it
		cmd := exec.Command("git", "-C", repoPath, "remote", "set-url", "upstream", upstreamURL)
		return cmd.Run()
	}

	// Add new upstream remote
	cmd := exec.Command("git", "-C", repoPath, "remote", "add", "upstream", upstreamURL)
	return cmd.Run()
}

// HasUpstreamRemote checks if the upstream remote is configured.
func HasUpstreamRemote(repoPath string) bool {
	_, err := getRemoteURL(repoPath, "upstream")
	return err == nil
}
