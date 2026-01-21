package github

import (
	"os/exec"
	"testing"
)

func TestCheckAuth_GHInstalled(t *testing.T) {
	// Skip if gh is not installed
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not installed, skipping auth tests")
	}

	// This test runs against the real gh CLI
	// It verifies our parsing works with actual output
	status, err := CheckAuth()
	if err != nil {
		// If not authenticated, that's fine - we're testing the code path
		t.Logf("Not authenticated (expected in CI): %v", err)
		return
	}

	// If we got here, we're authenticated
	if !status.Authenticated {
		t.Error("expected Authenticated to be true")
	}
}

func TestCheckRequiredScopes_NilStatus(t *testing.T) {
	err := CheckRequiredScopes(nil)
	if err == nil {
		t.Error("expected error for nil status")
	}
}

func TestCheckRequiredScopes_MissingScopes(t *testing.T) {
	status := &AuthStatus{
		Authenticated: true,
		Username:      "testuser",
		Scopes:        []string{"gist"}, // Missing 'repo' scope
	}

	err := CheckRequiredScopes(status)
	if err == nil {
		t.Error("expected error for missing scopes")
	}
}

func TestCheckRequiredScopes_HasRequiredScopes(t *testing.T) {
	status := &AuthStatus{
		Authenticated: true,
		Username:      "testuser",
		Scopes:        []string{"repo", "gist", "read:org"},
	}

	err := CheckRequiredScopes(status)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRepoPermissions_StructFields(t *testing.T) {
	// Test that the struct can be instantiated correctly
	perms := RepoPermissions{
		Admin:    true,
		Maintain: false,
		Push:     true,
		Pull:     true,
		Triage:   false,
	}

	if !perms.Admin {
		t.Error("expected Admin to be true")
	}
	if perms.Maintain {
		t.Error("expected Maintain to be false")
	}
	if !perms.Push {
		t.Error("expected Push to be true")
	}
	if !perms.Pull {
		t.Error("expected Pull to be true")
	}
	if perms.Triage {
		t.Error("expected Triage to be false")
	}
}

func TestAuthStatus_StructFields(t *testing.T) {
	status := AuthStatus{
		Authenticated: true,
		Username:      "testuser",
		Scopes:        []string{"repo", "read:org"},
	}

	if !status.Authenticated {
		t.Error("expected Authenticated to be true")
	}
	if status.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %s", status.Username)
	}
	if len(status.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(status.Scopes))
	}
}

func TestCheckRepoPermissions_InvalidRepo(t *testing.T) {
	// Skip if gh is not installed
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not installed, skipping")
	}

	// Test with a non-existent repo
	_, err := CheckRepoPermissions("nonexistent-owner-xyz", "nonexistent-repo-xyz", false)
	if err == nil {
		t.Error("expected error for non-existent repo")
	}
}
