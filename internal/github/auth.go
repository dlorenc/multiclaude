// Package github provides GitHub CLI authentication verification.
package github

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/dlorenc/multiclaude/internal/errors"
)

// AuthStatus represents the result of an authentication check.
type AuthStatus struct {
	// Authenticated indicates whether the user is logged in to GitHub
	Authenticated bool
	// Username is the GitHub username if authenticated
	Username string
	// Scopes contains the OAuth scopes granted to the token
	Scopes []string
}

// RepoPermissions represents the user's permissions on a repository.
type RepoPermissions struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Pull     bool `json:"pull"`
	Triage   bool `json:"triage"`
}

// requiredScopes lists the minimum OAuth scopes needed for multiclaude operations.
var requiredScopes = []string{"repo"}

// CheckAuth verifies that the GitHub CLI is installed and the user is authenticated.
// Returns an AuthStatus with details, or an error if verification fails.
func CheckAuth() (*AuthStatus, error) {
	// Check if gh CLI is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.GitHubCLINotFound()
	}

	// Run gh auth status to check authentication
	cmd := exec.Command("gh", "auth", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 1 means not authenticated
		return nil, errors.GitHubNotAuthenticated()
	}

	// Parse the output to extract details
	status := &AuthStatus{Authenticated: true}
	outputStr := string(output)

	// Extract username (looks for "Logged in to github.com account <username>")
	for _, line := range strings.Split(outputStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Logged in to") && strings.Contains(line, "account") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "account" && i+1 < len(parts) {
					// Remove any trailing characters like "(keyring)"
					username := strings.TrimSuffix(parts[i+1], "(keyring)")
					username = strings.TrimSuffix(username, "(environment)")
					status.Username = strings.TrimSpace(username)
					break
				}
			}
		}

		// Extract scopes (looks for "Token scopes: 'scope1', 'scope2'")
		if strings.Contains(line, "Token scopes:") {
			scopeStr := strings.TrimPrefix(line, "- Token scopes:")
			scopeStr = strings.TrimSpace(scopeStr)
			// Parse 'scope1', 'scope2' format
			for _, s := range strings.Split(scopeStr, ",") {
				s = strings.TrimSpace(s)
				s = strings.Trim(s, "'\"")
				if s != "" {
					status.Scopes = append(status.Scopes, s)
				}
			}
		}
	}

	return status, nil
}

// CheckRequiredScopes verifies that the current auth token has the required scopes.
// Returns an error listing missing scopes if any are not present.
func CheckRequiredScopes(status *AuthStatus) error {
	if status == nil {
		return errors.GitHubNotAuthenticated()
	}

	var missing []string
	for _, required := range requiredScopes {
		found := false
		for _, scope := range status.Scopes {
			if scope == required {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		return errors.GitHubAuthScopesMissing(missing)
	}

	return nil
}

// CheckRepoPermissions checks the user's permissions on a specific repository.
// Returns an error if the user lacks the required permission level.
func CheckRepoPermissions(owner, repo string, requirePush bool) (*RepoPermissions, error) {
	// Check if gh CLI is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.GitHubCLINotFound()
	}

	// Use gh api to get repository permissions
	cmd := exec.Command("gh", "api", "repos/"+owner+"/"+repo, "--jq", ".permissions")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.GitHubAPIFailed("get repo permissions", err)
	}

	var perms RepoPermissions
	if err := json.Unmarshal(output, &perms); err != nil {
		return nil, errors.GitHubAPIFailed("parse permissions response", err)
	}

	if requirePush && !perms.Push {
		return &perms, errors.GitHubRepoAccessDenied(owner, repo, "push")
	}

	return &perms, nil
}

// VerifyAuthForRepo performs a complete authentication and permissions check
// for operations on a specific repository. This is the main entry point for
// pre-operation verification.
func VerifyAuthForRepo(owner, repo string, requirePush bool) error {
	// Step 1: Check basic auth
	status, err := CheckAuth()
	if err != nil {
		return err
	}

	// Step 2: Check scopes
	if err := CheckRequiredScopes(status); err != nil {
		return err
	}

	// Step 3: Check repo permissions
	_, err = CheckRepoPermissions(owner, repo, requirePush)
	if err != nil {
		return err
	}

	return nil
}
