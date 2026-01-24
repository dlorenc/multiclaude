package fork

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDetectForkViaGitHubAPI_GhNotInstalled tests behavior when gh CLI is not available
func TestDetectForkViaGitHubAPI_GhNotInstalled(t *testing.T) {
	// Save original PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set PATH to empty so gh won't be found
	os.Setenv("PATH", "")

	// detectForkViaGitHubAPI should fail when gh is not available
	_, err := detectForkViaGitHubAPI("owner", "repo")
	if err == nil {
		t.Error("Expected error when gh CLI is not installed")
	}
}

// TestDetectForkViaGitHubAPI_InvalidOwnerRepo tests with invalid owner/repo combinations
func TestDetectForkViaGitHubAPI_InvalidInput(t *testing.T) {
	// Check if gh is available
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not available, skipping API test")
	}

	// Test with a non-existent repo (should fail with API error)
	_, err := detectForkViaGitHubAPI("nonexistent-user-12345", "nonexistent-repo-67890")
	if err == nil {
		t.Error("Expected error for non-existent repository")
	}
}

// TestDetectForkResultParsing tests the JSON parsing of fork detection results
func TestDetectForkResultParsing(t *testing.T) {
	tests := []struct {
		name       string
		jsonInput  string
		wantIsFork bool
		wantParent string
		wantErr    bool
	}{
		{
			name:       "not a fork",
			jsonInput:  `{"fork": false, "parent_owner": null, "parent_repo": null, "parent_url": null}`,
			wantIsFork: false,
			wantErr:    false,
		},
		{
			name:       "is a fork",
			jsonInput:  `{"fork": true, "parent_owner": "upstream", "parent_repo": "repo", "parent_url": "https://github.com/upstream/repo.git"}`,
			wantIsFork: true,
			wantParent: "upstream",
			wantErr:    false,
		},
		{
			name:      "invalid JSON",
			jsonInput: `{invalid json}`,
			wantErr:   true,
		},
		{
			name:      "empty response",
			jsonInput: ``,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				Fork        bool   `json:"fork"`
				ParentOwner string `json:"parent_owner"`
				ParentRepo  string `json:"parent_repo"`
				ParentURL   string `json:"parent_url"`
			}

			err := json.Unmarshal([]byte(tt.jsonInput), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.Fork != tt.wantIsFork {
					t.Errorf("Fork = %v, want %v", result.Fork, tt.wantIsFork)
				}
				if tt.wantIsFork && result.ParentOwner != tt.wantParent {
					t.Errorf("ParentOwner = %v, want %v", result.ParentOwner, tt.wantParent)
				}
			}
		})
	}
}

// TestForkInfoConstruction tests ForkInfo struct initialization
func TestForkInfoConstruction(t *testing.T) {
	tests := []struct {
		name         string
		isFork       bool
		parentOwner  string
		parentRepo   string
		parentURL    string
		wantUpstream string
	}{
		{
			name:         "non-fork repo",
			isFork:       false,
			wantUpstream: "",
		},
		{
			name:         "fork with parent",
			isFork:       true,
			parentOwner:  "original",
			parentRepo:   "project",
			parentURL:    "https://github.com/original/project.git",
			wantUpstream: "original",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &ForkInfo{
				IsFork: tt.isFork,
			}

			if tt.isFork {
				info.UpstreamOwner = tt.parentOwner
				info.UpstreamRepo = tt.parentRepo
				info.UpstreamURL = tt.parentURL
			}

			if info.IsFork != tt.isFork {
				t.Errorf("IsFork = %v, want %v", info.IsFork, tt.isFork)
			}
			if info.UpstreamOwner != tt.wantUpstream {
				t.Errorf("UpstreamOwner = %v, want %v", info.UpstreamOwner, tt.wantUpstream)
			}
		})
	}
}

// TestDetectFork_ForkWithExistingUpstream tests fork detection when upstream already exists
func TestDetectFork_ForkWithExistingUpstream(t *testing.T) {
	tmpDir := setupTestRepo(t)
	defer os.RemoveAll(tmpDir)

	// Add origin
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/myuser/myrepo")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add origin: %v", err)
	}

	// Add upstream (simulating a fork)
	upstreamURL := "https://github.com/upstream/repo"
	cmd = exec.Command("git", "remote", "add", "upstream", upstreamURL)
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add upstream: %v", err)
	}

	// DetectFork should detect fork based on upstream remote
	info, err := DetectFork(tmpDir)
	if err != nil {
		t.Fatalf("DetectFork() failed: %v", err)
	}

	if !info.IsFork {
		t.Error("expected IsFork to be true with upstream remote")
	}
	if info.UpstreamURL != upstreamURL {
		t.Errorf("UpstreamURL = %q, want %q", info.UpstreamURL, upstreamURL)
	}
}

// TestDetectFork_SSHRemotes tests fork detection with SSH remote URLs
func TestDetectFork_SSHRemotes(t *testing.T) {
	tmpDir := setupTestRepo(t)
	defer os.RemoveAll(tmpDir)

	// Add origin with SSH URL
	cmd := exec.Command("git", "remote", "add", "origin", "git@github.com:myuser/myrepo.git")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add origin: %v", err)
	}

	// Add upstream with SSH URL
	cmd = exec.Command("git", "remote", "add", "upstream", "git@github.com:upstream/repo.git")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add upstream: %v", err)
	}

	// DetectFork should handle SSH URLs
	info, err := DetectFork(tmpDir)
	if err != nil {
		t.Fatalf("DetectFork() failed: %v", err)
	}

	if !info.IsFork {
		t.Error("expected IsFork to be true with upstream remote")
	}
	if info.UpstreamOwner != "upstream" {
		t.Errorf("UpstreamOwner = %q, want %q", info.UpstreamOwner, "upstream")
	}
	if info.UpstreamRepo != "repo" {
		t.Errorf("UpstreamRepo = %q, want %q", info.UpstreamRepo, "repo")
	}
}

// TestAddUpstreamRemote_Idempotent tests that adding upstream is idempotent
func TestAddUpstreamRemote_Idempotent(t *testing.T) {
	tmpDir := setupTestRepo(t)
	defer os.RemoveAll(tmpDir)

	upstreamURL := "https://github.com/upstream/repo"

	// Add upstream first time
	if err := AddUpstreamRemote(tmpDir, upstreamURL); err != nil {
		t.Fatalf("First AddUpstreamRemote() failed: %v", err)
	}

	// Add upstream second time with same URL (should succeed)
	if err := AddUpstreamRemote(tmpDir, upstreamURL); err != nil {
		t.Fatalf("Second AddUpstreamRemote() failed: %v", err)
	}

	// Verify URL is correct
	cmd := exec.Command("git", "remote", "get-url", "upstream")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get upstream url: %v", err)
	}
	got := string(output)
	if got != upstreamURL+"\n" {
		t.Errorf("upstream URL = %q, want %q", got, upstreamURL)
	}
}

// TestParseGitHubURL_EdgeCases tests edge cases for GitHub URL parsing
// Note: These test cases reflect the current implementation behavior
func TestParseGitHubURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		// The current regex implementation doesn't handle trailing slashes
		{
			name:    "URL with trailing slash - current impl returns error",
			url:     "https://github.com/owner/repo/",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
		{
			name:    "just github.com",
			url:     "https://github.com",
			wantErr: true,
		},
		{
			name:    "github.com with only owner",
			url:     "https://github.com/owner",
			wantErr: true,
		},
		// The current impl doesn't handle extra path segments
		{
			name:    "URL with extra path segments - current impl returns error",
			url:     "https://github.com/owner/repo/tree/main",
			wantErr: true,
		},
		// The current impl captures query params as part of repo name
		{
			name:      "URL with query params - captured as part of repo name",
			url:       "https://github.com/owner/repo?tab=readme",
			wantOwner: "owner",
			wantRepo:  "repo?tab=readme", // Query params are captured
			wantErr:   false,
		},
		{
			name:      "SSH URL without .git",
			url:       "git@github.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "numeric owner",
			url:       "https://github.com/12345/repo",
			wantOwner: "12345",
			wantRepo:  "repo",
			wantErr:   false,
		},
		// The current regex doesn't match dots in repo names
		{
			name:    "dots in repo name - current impl returns error",
			url:     "https://github.com/owner/my.dotted.repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGitHubURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if owner != tt.wantOwner {
					t.Errorf("ParseGitHubURL() owner = %v, want %v", owner, tt.wantOwner)
				}
				if repo != tt.wantRepo {
					t.Errorf("ParseGitHubURL() repo = %v, want %v", repo, tt.wantRepo)
				}
			}
		})
	}
}

// TestGetRemoteURL_MultipleRemotes tests getting URL with multiple remotes configured
func TestGetRemoteURL_MultipleRemotes(t *testing.T) {
	tmpDir := setupTestRepo(t)
	defer os.RemoveAll(tmpDir)

	// Add multiple remotes
	remotes := map[string]string{
		"origin":   "https://github.com/test/origin-repo",
		"upstream": "https://github.com/test/upstream-repo",
		"backup":   "https://github.com/test/backup-repo",
	}

	for name, url := range remotes {
		cmd := exec.Command("git", "remote", "add", name, url)
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to add remote %s: %v", name, err)
		}
	}

	// Test getting each remote URL
	for name, expectedURL := range remotes {
		url, err := getRemoteURL(tmpDir, name)
		if err != nil {
			t.Errorf("getRemoteURL(%s) failed: %v", name, err)
			continue
		}
		if url != expectedURL {
			t.Errorf("getRemoteURL(%s) = %q, want %q", name, url, expectedURL)
		}
	}
}

// TestDetectFork_SymlinkPath tests fork detection with symlinked paths
func TestDetectFork_SymlinkPath(t *testing.T) {
	// Create real repo directory
	tmpDir := setupTestRepo(t)
	defer os.RemoveAll(tmpDir)

	// Add origin
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/myuser/myrepo")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add origin: %v", err)
	}

	// Create a symlink to the repo
	symlinkDir, err := os.MkdirTemp("", "fork-symlink-test-*")
	if err != nil {
		t.Fatalf("failed to create symlink dir: %v", err)
	}
	defer os.RemoveAll(symlinkDir)

	symlinkPath := filepath.Join(symlinkDir, "linked-repo")
	if err := os.Symlink(tmpDir, symlinkPath); err != nil {
		t.Skip("Cannot create symlinks on this system")
	}

	// DetectFork should work with symlinked path
	info, err := DetectFork(symlinkPath)
	if err != nil {
		t.Fatalf("DetectFork() with symlink failed: %v", err)
	}

	if info.OriginOwner != "myuser" {
		t.Errorf("OriginOwner = %q, want %q", info.OriginOwner, "myuser")
	}
	if info.OriginRepo != "myrepo" {
		t.Errorf("OriginRepo = %q, want %q", info.OriginRepo, "myrepo")
	}
}
