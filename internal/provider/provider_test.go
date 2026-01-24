package provider

import (
	"testing"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected Type
		wantErr  bool
	}{
		// GitHub URLs
		{
			name:     "GitHub HTTPS",
			url:      "https://github.com/owner/repo",
			expected: TypeGitHub,
		},
		{
			name:     "GitHub HTTPS with .git",
			url:      "https://github.com/owner/repo.git",
			expected: TypeGitHub,
		},
		{
			name:     "GitHub SSH",
			url:      "git@github.com:owner/repo.git",
			expected: TypeGitHub,
		},
		{
			name:     "GitHub SSH without .git",
			url:      "git@github.com:owner/repo",
			expected: TypeGitHub,
		},

		// Azure DevOps URLs
		{
			name:     "ADO HTTPS",
			url:      "https://dev.azure.com/org/project/_git/repo",
			expected: TypeAzureDevOps,
		},
		{
			name:     "ADO HTTPS with .git",
			url:      "https://dev.azure.com/org/project/_git/repo.git",
			expected: TypeAzureDevOps,
		},
		{
			name:     "ADO SSH",
			url:      "git@ssh.dev.azure.com:v3/org/project/repo",
			expected: TypeAzureDevOps,
		},
		{
			name:     "ADO Legacy visualstudio.com",
			url:      "https://myorg.visualstudio.com/myproject/_git/myrepo",
			expected: TypeAzureDevOps,
		},
		{
			name:     "ADO Legacy default project",
			url:      "https://myorg.visualstudio.com/_git/myrepo",
			expected: TypeAzureDevOps,
		},

		// Invalid URLs
		{
			name:    "Unknown provider",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "Invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectProvider(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("DetectProvider() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGitHubParseURL(t *testing.T) {
	gh := NewGitHub()

	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS basic",
			url:       "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS with .git",
			url:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS with trailing slash",
			url:       "https://github.com/owner/repo/",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH basic",
			url:       "git@github.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH with .git",
			url:       "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "Complex repo name",
			url:       "https://github.com/my-org/my-repo-name",
			wantOwner: "my-org",
			wantRepo:  "my-repo-name",
		},
		{
			name:    "Invalid URL",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := gh.ParseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if info.Owner != tt.wantOwner {
				t.Errorf("ParseURL() owner = %v, want %v", info.Owner, tt.wantOwner)
			}
			if info.Repo != tt.wantRepo {
				t.Errorf("ParseURL() repo = %v, want %v", info.Repo, tt.wantRepo)
			}
			if info.Provider != TypeGitHub {
				t.Errorf("ParseURL() provider = %v, want %v", info.Provider, TypeGitHub)
			}
		})
	}
}

func TestAzureDevOpsParseURL(t *testing.T) {
	ado := NewAzureDevOps()

	tests := []struct {
		name        string
		url         string
		wantOrg     string
		wantProject string
		wantRepo    string
		wantErr     bool
	}{
		{
			name:        "HTTPS basic",
			url:         "https://dev.azure.com/myorg/myproject/_git/myrepo",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
		},
		{
			name:        "HTTPS with .git",
			url:         "https://dev.azure.com/myorg/myproject/_git/myrepo.git",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
		},
		{
			name:        "SSH format",
			url:         "git@ssh.dev.azure.com:v3/myorg/myproject/myrepo",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
		},
		{
			name:        "Legacy visualstudio.com",
			url:         "https://myorg.visualstudio.com/myproject/_git/myrepo",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
		},
		{
			name:        "Legacy default project",
			url:         "https://myorg.visualstudio.com/_git/myrepo",
			wantOrg:     "myorg",
			wantProject: "myorg", // Project defaults to org name
			wantRepo:    "myrepo",
		},
		{
			name:        "Complex names with hyphens",
			url:         "https://dev.azure.com/my-org/my-project/_git/my-repo-name",
			wantOrg:     "my-org",
			wantProject: "my-project",
			wantRepo:    "my-repo-name",
		},
		{
			name:        "SSH with URL-encoded space in project",
			url:         "git@ssh.dev.azure.com:v3/k2intel/K2%20Engineering/cms-backend",
			wantOrg:     "k2intel",
			wantProject: "K2 Engineering", // Should be decoded
			wantRepo:    "cms-backend",
		},
		{
			name:        "HTTPS with URL-encoded space in project",
			url:         "https://dev.azure.com/myorg/My%20Project/_git/myrepo",
			wantOrg:     "myorg",
			wantProject: "My Project", // Should be decoded
			wantRepo:    "myrepo",
		},
		{
			name:    "Invalid URL - GitHub",
			url:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "Invalid URL - missing _git",
			url:     "https://dev.azure.com/myorg/myproject/myrepo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ado.ParseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if info.Owner != tt.wantOrg {
				t.Errorf("ParseURL() org = %v, want %v", info.Owner, tt.wantOrg)
			}
			if info.Project != tt.wantProject {
				t.Errorf("ParseURL() project = %v, want %v", info.Project, tt.wantProject)
			}
			if info.Repo != tt.wantRepo {
				t.Errorf("ParseURL() repo = %v, want %v", info.Repo, tt.wantRepo)
			}
			if info.Provider != TypeAzureDevOps {
				t.Errorf("ParseURL() provider = %v, want %v", info.Provider, TypeAzureDevOps)
			}
		})
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantProvider Type
		wantOwner    string
		wantRepo     string
		wantProject  string
		wantErr      bool
	}{
		{
			name:         "GitHub URL",
			url:          "https://github.com/dlorenc/multiclaude",
			wantProvider: TypeGitHub,
			wantOwner:    "dlorenc",
			wantRepo:     "multiclaude",
		},
		{
			name:         "ADO URL",
			url:          "https://dev.azure.com/myorg/myproject/_git/myrepo",
			wantProvider: TypeAzureDevOps,
			wantOwner:    "myorg",
			wantProject:  "myproject",
			wantRepo:     "myrepo",
		},
		{
			name:    "Unknown provider",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if info.Provider != tt.wantProvider {
				t.Errorf("ParseURL() provider = %v, want %v", info.Provider, tt.wantProvider)
			}
			if info.Owner != tt.wantOwner {
				t.Errorf("ParseURL() owner = %v, want %v", info.Owner, tt.wantOwner)
			}
			if info.Repo != tt.wantRepo {
				t.Errorf("ParseURL() repo = %v, want %v", info.Repo, tt.wantRepo)
			}
			if tt.wantProject != "" && info.Project != tt.wantProject {
				t.Errorf("ParseURL() project = %v, want %v", info.Project, tt.wantProject)
			}
		})
	}
}

func TestGetProvider(t *testing.T) {
	tests := []struct {
		name     string
		provType Type
		wantType Type
		wantErr  bool
	}{
		{
			name:     "GitHub",
			provType: TypeGitHub,
			wantType: TypeGitHub,
		},
		{
			name:     "Azure DevOps",
			provType: TypeAzureDevOps,
			wantType: TypeAzureDevOps,
		},
		{
			name:     "Unknown",
			provType: Type("unknown"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, err := GetProvider(tt.provType)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if prov.Name() != tt.wantType {
				t.Errorf("GetProvider().Name() = %v, want %v", prov.Name(), tt.wantType)
			}
		})
	}
}

func TestGitHubCommands(t *testing.T) {
	gh := NewGitHub()

	t.Run("PRListCommand", func(t *testing.T) {
		cmd := gh.PRListCommand("multiclaude", "@me")
		expected := "gh pr list --author @me --label multiclaude"
		if cmd != expected {
			t.Errorf("PRListCommand() = %v, want %v", cmd, expected)
		}
	})

	t.Run("PRCreateCommand", func(t *testing.T) {
		cmd := gh.PRCreateCommand("upstream/repo", "owner:branch")
		expected := "gh pr create --repo upstream/repo --head owner:branch"
		if cmd != expected {
			t.Errorf("PRCreateCommand() = %v, want %v", cmd, expected)
		}
	})

	t.Run("PRViewCommand", func(t *testing.T) {
		cmd := gh.PRViewCommand(123, "title,state")
		expected := "gh pr view 123 --json title,state"
		if cmd != expected {
			t.Errorf("PRViewCommand() = %v, want %v", cmd, expected)
		}
	})

	t.Run("PRChecksCommand", func(t *testing.T) {
		cmd := gh.PRChecksCommand(123)
		expected := "gh pr checks 123"
		if cmd != expected {
			t.Errorf("PRChecksCommand() = %v, want %v", cmd, expected)
		}
	})

	t.Run("RunListCommand", func(t *testing.T) {
		cmd := gh.RunListCommand("main", 5)
		expected := "gh run list --branch main --limit 5"
		if cmd != expected {
			t.Errorf("RunListCommand() = %v, want %v", cmd, expected)
		}
	})
}

func TestAzureDevOpsCommands(t *testing.T) {
	ado := NewAzureDevOpsWithConfig("myorg", "myproject", "myrepo")

	t.Run("PRListCommand", func(t *testing.T) {
		cmd := ado.PRListCommand("", "")
		if cmd == "" {
			t.Error("PRListCommand() should not return empty string")
		}
		if !contains(cmd, "curl") {
			t.Error("PRListCommand() should use curl")
		}
		if !contains(cmd, "AZURE_DEVOPS_PAT") {
			t.Error("PRListCommand() should reference AZURE_DEVOPS_PAT")
		}
	})

	t.Run("PRViewCommand", func(t *testing.T) {
		cmd := ado.PRViewCommand(123, "")
		if !contains(cmd, "pullrequests/123") {
			t.Errorf("PRViewCommand() should contain PR number, got: %s", cmd)
		}
	})

	t.Run("PRCommentCommand", func(t *testing.T) {
		cmd := ado.PRCommentCommand(123, "Test comment")
		if !contains(cmd, "threads") {
			t.Errorf("PRCommentCommand() should use threads API, got: %s", cmd)
		}
	})

	t.Run("PRMergeCommand", func(t *testing.T) {
		cmd := ado.PRMergeCommand(123)
		if !contains(cmd, "completed") {
			t.Errorf("PRMergeCommand() should set status to completed, got: %s", cmd)
		}
	})
}

func TestGetProviderForURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantType Type
		wantErr  bool
	}{
		{
			name:     "GitHub URL",
			url:      "https://github.com/owner/repo",
			wantType: TypeGitHub,
		},
		{
			name:     "ADO URL",
			url:      "https://dev.azure.com/org/project/_git/repo",
			wantType: TypeAzureDevOps,
		},
		{
			name:    "Unknown URL",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, err := GetProviderForURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProviderForURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if prov.Name() != tt.wantType {
				t.Errorf("GetProviderForURL().Name() = %v, want %v", prov.Name(), tt.wantType)
			}
		})
	}
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAzureDevOpsURLEncodingInCloneURL(t *testing.T) {
	ado := NewAzureDevOps()

	tests := []struct {
		name         string
		url          string
		wantProject  string
		wantCloneURL string
	}{
		{
			name:         "SSH with URL-encoded space",
			url:          "git@ssh.dev.azure.com:v3/k2intel/K2%20Engineering/cms-backend",
			wantProject:  "K2 Engineering",
			wantCloneURL: "https://dev.azure.com/k2intel/K2%20Engineering/_git/cms-backend",
		},
		{
			name:         "HTTPS with URL-encoded space",
			url:          "https://dev.azure.com/myorg/My%20Project/_git/myrepo",
			wantProject:  "My Project",
			wantCloneURL: "https://dev.azure.com/myorg/My%20Project/_git/myrepo",
		},
		{
			name:         "No encoding needed",
			url:          "https://dev.azure.com/myorg/MyProject/_git/myrepo",
			wantProject:  "MyProject",
			wantCloneURL: "https://dev.azure.com/myorg/MyProject/_git/myrepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ado.ParseURL(tt.url)
			if err != nil {
				t.Fatalf("ParseURL() error = %v", err)
			}
			if info.Project != tt.wantProject {
				t.Errorf("ParseURL().Project = %q, want %q", info.Project, tt.wantProject)
			}
			if info.CloneURL != tt.wantCloneURL {
				t.Errorf("ParseURL().CloneURL = %q, want %q", info.CloneURL, tt.wantCloneURL)
			}
		})
	}
}

func TestAzureDevOpsCommandsWithSpacesInProject(t *testing.T) {
	// Test that commands with spaces in project name have properly encoded URLs
	ado := NewAzureDevOpsWithConfig("k2intel", "K2 Engineering", "cms-backend")

	t.Run("PRListCommand URL encoding", func(t *testing.T) {
		cmd := ado.PRListCommand("", "")
		if !contains(cmd, "K2%20Engineering") {
			t.Errorf("PRListCommand() should URL-encode project name with space, got: %s", cmd)
		}
	})

	t.Run("PRViewCommand URL encoding", func(t *testing.T) {
		cmd := ado.PRViewCommand(123, "")
		if !contains(cmd, "K2%20Engineering") {
			t.Errorf("PRViewCommand() should URL-encode project name with space, got: %s", cmd)
		}
	})

	t.Run("getAPIBaseURL URL encoding", func(t *testing.T) {
		baseURL := ado.getAPIBaseURL()
		if !contains(baseURL, "K2%20Engineering") {
			t.Errorf("getAPIBaseURL() should URL-encode project name with space, got: %s", baseURL)
		}
	})
}
