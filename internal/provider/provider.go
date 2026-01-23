// Package provider defines interfaces and types for git hosting providers (GitHub, Azure DevOps).
package provider

import (
	"fmt"
	"regexp"
	"strings"
)

// Type represents the type of git hosting provider.
type Type string

const (
	// TypeGitHub represents GitHub as the git hosting provider.
	TypeGitHub Type = "github"
	// TypeAzureDevOps represents Azure DevOps as the git hosting provider.
	TypeAzureDevOps Type = "ado"
)

// RepoInfo contains parsed repository information from a URL.
type RepoInfo struct {
	// Provider is the type of git hosting provider.
	Provider Type `json:"provider"`

	// Owner is the repository owner (GitHub) or organization (ADO).
	Owner string `json:"owner"`

	// Project is the ADO project name (empty for GitHub).
	Project string `json:"project,omitempty"`

	// Repo is the repository name.
	Repo string `json:"repo"`

	// CloneURL is the normalized clone URL.
	CloneURL string `json:"clone_url"`
}

// ForkInfo contains information about a repository's fork status.
type ForkInfo struct {
	// IsFork is true if the repository is a fork.
	IsFork bool `json:"is_fork"`

	// UpstreamOwner is the owner of the upstream repository (if fork).
	UpstreamOwner string `json:"upstream_owner,omitempty"`

	// UpstreamRepo is the name of the upstream repository (if fork).
	UpstreamRepo string `json:"upstream_repo,omitempty"`

	// UpstreamURL is the clone URL of the upstream repository (if fork).
	UpstreamURL string `json:"upstream_url,omitempty"`
}

// PRStatus contains pull request status information.
type PRStatus struct {
	// Number is the PR number.
	Number int `json:"number"`

	// State is the PR state (open, closed, merged).
	State string `json:"state"`

	// Title is the PR title.
	Title string `json:"title"`

	// URL is the web URL to view the PR.
	URL string `json:"url"`
}

// Provider defines the interface for git hosting providers.
type Provider interface {
	// Name returns the provider type.
	Name() Type

	// ParseURL parses a repository URL and returns repo information.
	// Returns an error if the URL is not recognized by this provider.
	ParseURL(url string) (*RepoInfo, error)

	// DetectFork checks if a repository is a fork.
	// repoPath is the local path to the cloned repository.
	DetectFork(repoPath string) (*ForkInfo, error)

	// PRListCommand returns the command to list PRs.
	// label is the PR label to filter by (e.g., "multiclaude").
	// authorFilter can be "@me", a username, or empty for all.
	PRListCommand(label string, authorFilter string) string

	// PRCreateCommand returns the command to create a PR.
	// targetRepo is the target repository (for forks).
	// headBranch is the source branch.
	PRCreateCommand(targetRepo, headBranch string) string

	// PRViewCommand returns the command to view a PR.
	// prNumber is the PR number.
	// jsonFields is a comma-separated list of JSON fields to return.
	PRViewCommand(prNumber int, jsonFields string) string

	// PRChecksCommand returns the command to view PR checks.
	PRChecksCommand(prNumber int) string

	// PRCommentCommand returns the command to comment on a PR.
	PRCommentCommand(prNumber int, body string) string

	// PREditCommand returns the command to edit a PR (e.g., add labels).
	PREditCommand(prNumber int, addLabel string) string

	// PRMergeCommand returns the command to merge a PR.
	PRMergeCommand(prNumber int) string

	// RunListCommand returns the command to list CI runs for a branch.
	RunListCommand(branch string, limit int) string

	// APICommand returns the command to call the provider's API.
	// endpoint is the API endpoint path.
	// jqFilter is an optional jq filter for the response.
	APICommand(endpoint, jqFilter string) string

	// ReviewCommand returns the command to spawn a review for a PR URL.
	ReviewCommand(prURL string) string
}

// DetectProvider determines the provider type from a repository URL.
func DetectProvider(url string) (Type, error) {
	url = strings.TrimSpace(url)
	url = strings.TrimRight(url, "/")

	// GitHub patterns
	if strings.Contains(url, "github.com") {
		return TypeGitHub, nil
	}

	// Azure DevOps patterns
	if strings.Contains(url, "dev.azure.com") ||
		strings.Contains(url, "visualstudio.com") ||
		strings.Contains(url, "ssh.dev.azure.com") {
		return TypeAzureDevOps, nil
	}

	return "", fmt.Errorf("unable to detect provider from URL: %s", url)
}

// ParseURL parses any supported repository URL and returns repo information.
func ParseURL(url string) (*RepoInfo, error) {
	providerType, err := DetectProvider(url)
	if err != nil {
		return nil, err
	}

	switch providerType {
	case TypeGitHub:
		prov := NewGitHub()
		return prov.ParseURL(url)
	case TypeAzureDevOps:
		prov := NewAzureDevOps()
		return prov.ParseURL(url)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// GetProvider returns the appropriate provider for the given type.
func GetProvider(providerType Type) (Provider, error) {
	switch providerType {
	case TypeGitHub:
		return NewGitHub(), nil
	case TypeAzureDevOps:
		return NewAzureDevOps(), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// GetProviderForURL returns the appropriate provider for the given URL.
func GetProviderForURL(url string) (Provider, error) {
	providerType, err := DetectProvider(url)
	if err != nil {
		return nil, err
	}
	return GetProvider(providerType)
}

// normalizeGitURL normalizes a git URL to HTTPS format.
func normalizeGitURL(url string) string {
	url = strings.TrimSpace(url)
	url = strings.TrimRight(url, "/")
	url = strings.TrimSuffix(url, ".git")
	return url
}

// compileRegex compiles a regex pattern, panicking on error (for package-level patterns).
func compileRegex(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
