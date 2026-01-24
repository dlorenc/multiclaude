// Package github provides GitHub CLI authentication verification utilities.
package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// AuthStatus represents the current GitHub CLI authentication status.
type AuthStatus struct {
	Authenticated bool
	Username      string
	Scopes        []string
}

// RepoPermissions represents the user's permissions on a specific repository.
type RepoPermissions struct {
	Owner    string
	Repo     string
	Pull     bool
	Push     bool
	Maintain bool
	Admin    bool
}

// VerifyResult contains the combined result of all verification checks.
type VerifyResult struct {
	AuthStatus      *AuthStatus
	Scopes          *ScopesResult
	RepoPermissions *RepoPermissions
	Errors          []error
}

// ScopesResult contains the result of scope verification.
type ScopesResult struct {
	HasRequired bool
	Present     []string
	Missing     []string
}

// RequiredScopes is the list of OAuth scopes required for full functionality.
var RequiredScopes = []string{"repo"}

// CheckAuth verifies that the gh CLI is installed and the user is authenticated.
func CheckAuth() (*AuthStatus, error) {
	// Check if gh CLI is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found: %w", err)
	}

	// Check authentication status
	cmd := exec.Command("gh", "auth", "status", "--show-token")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Not authenticated or error
		if strings.Contains(string(output), "not logged in") ||
			strings.Contains(string(output), "authentication required") {
			return &AuthStatus{Authenticated: false}, nil
		}
		return nil, fmt.Errorf("failed to check auth status: %w", err)
	}

	// Parse auth status output for username and scopes
	status := &AuthStatus{Authenticated: true}
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for "Logged in to github.com account username (keyring)"
		if strings.Contains(line, "Logged in to") && strings.Contains(line, "account") {
			parts := strings.Split(line, "account")
			if len(parts) > 1 {
				// Extract username - format is "account username (" or similar
				rest := strings.TrimSpace(parts[1])
				if idx := strings.Index(rest, " "); idx > 0 {
					status.Username = rest[:idx]
				} else if idx := strings.Index(rest, "("); idx > 0 {
					status.Username = strings.TrimSpace(rest[:idx])
				} else {
					status.Username = rest
				}
			}
		}

		// Look for "Token scopes:" line
		if strings.HasPrefix(line, "- Token scopes:") || strings.HasPrefix(line, "Token scopes:") {
			scopesPart := strings.TrimPrefix(line, "- Token scopes:")
			scopesPart = strings.TrimPrefix(scopesPart, "Token scopes:")
			scopesPart = strings.TrimSpace(scopesPart)
			if scopesPart != "" && scopesPart != "''" && scopesPart != "(none)" {
				scopes := strings.Split(scopesPart, ",")
				for _, s := range scopes {
					s = strings.TrimSpace(s)
					s = strings.Trim(s, "'")
					if s != "" {
						status.Scopes = append(status.Scopes, s)
					}
				}
			}
		}
	}

	return status, nil
}

// CheckRequiredScopes verifies that the token has the required OAuth scopes.
func CheckRequiredScopes(scopes []string) *ScopesResult {
	result := &ScopesResult{
		HasRequired: true,
		Present:     []string{},
		Missing:     []string{},
	}

	scopeSet := make(map[string]bool)
	for _, s := range scopes {
		scopeSet[s] = true
	}

	for _, required := range RequiredScopes {
		if scopeSet[required] {
			result.Present = append(result.Present, required)
		} else {
			result.Missing = append(result.Missing, required)
			result.HasRequired = false
		}
	}

	return result
}

// CheckRepoPermissions checks the user's permissions on a specific repository.
func CheckRepoPermissions(owner, repo string) (*RepoPermissions, error) {
	// Check if gh CLI is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found: %w", err)
	}

	// Use gh api to get repository permissions
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", owner, repo),
		"--jq", ".permissions")
	output, err := cmd.Output()
	if err != nil {
		// Check if it's a not found error
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "Not Found") ||
				strings.Contains(stderr, "404") {
				return nil, fmt.Errorf("repository %s/%s not found or not accessible", owner, repo)
			}
		}
		return nil, fmt.Errorf("failed to check repository permissions: %w", err)
	}

	// Parse permissions JSON
	var perms struct {
		Pull     bool `json:"pull"`
		Push     bool `json:"push"`
		Maintain bool `json:"maintain"`
		Admin    bool `json:"admin"`
	}

	if err := json.Unmarshal(output, &perms); err != nil {
		return nil, fmt.Errorf("failed to parse permissions: %w", err)
	}

	return &RepoPermissions{
		Owner:    owner,
		Repo:     repo,
		Pull:     perms.Pull,
		Push:     perms.Push,
		Maintain: perms.Maintain,
		Admin:    perms.Admin,
	}, nil
}

// VerifyAuthForRepo performs all verification checks for a repository.
// If repoRef is empty, only basic auth and scope checks are performed.
func VerifyAuthForRepo(repoRef string) *VerifyResult {
	result := &VerifyResult{
		Errors: []error{},
	}

	// Check basic authentication
	auth, err := CheckAuth()
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}
	result.AuthStatus = auth

	if !auth.Authenticated {
		result.Errors = append(result.Errors, fmt.Errorf("not authenticated with GitHub"))
		return result
	}

	// Check required scopes
	result.Scopes = CheckRequiredScopes(auth.Scopes)
	if !result.Scopes.HasRequired {
		result.Errors = append(result.Errors,
			fmt.Errorf("missing required OAuth scopes: %v", result.Scopes.Missing))
	}

	// Check repository permissions if repo specified
	if repoRef != "" {
		owner, repo, err := ParseRepoRef(repoRef)
		if err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}

		perms, err := CheckRepoPermissions(owner, repo)
		if err != nil {
			result.Errors = append(result.Errors, err)
		} else {
			result.RepoPermissions = perms
			if !perms.Push {
				result.Errors = append(result.Errors,
					fmt.Errorf("no push access to %s/%s", owner, repo))
			}
		}
	}

	return result
}

// ParseRepoRef parses a repository reference in the form "owner/repo".
func ParseRepoRef(ref string) (owner, repo string, err error) {
	ref = strings.TrimSpace(ref)

	// Handle URL format (e.g., https://github.com/owner/repo)
	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		ref = strings.TrimPrefix(ref, "https://")
		ref = strings.TrimPrefix(ref, "http://")
		ref = strings.TrimPrefix(ref, "github.com/")
		ref = strings.TrimSuffix(ref, ".git")
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository reference: %s (expected owner/repo)", ref)
	}

	return parts[0], parts[1], nil
}
