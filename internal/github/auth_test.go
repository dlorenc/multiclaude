package github

import (
	"testing"
)

func TestParseRepoRef(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "simple owner/repo",
			input:     "owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "with https URL",
			input:     "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "with http URL",
			input:     "http://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "with .git suffix",
			input:     "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "with whitespace",
			input:     "  owner/repo  ",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:    "missing repo",
			input:   "owner/",
			wantErr: true,
		},
		{
			name:    "missing owner",
			input:   "/repo",
			wantErr: true,
		},
		{
			name:    "no slash",
			input:   "ownerrepo",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseRepoRef(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseRepoRef(%q) expected error, got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRepoRef(%q) unexpected error: %v", tt.input, err)
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("ParseRepoRef(%q) owner = %q, want %q", tt.input, owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("ParseRepoRef(%q) repo = %q, want %q", tt.input, repo, tt.wantRepo)
			}
		})
	}
}

func TestCheckRequiredScopes(t *testing.T) {
	tests := []struct {
		name        string
		scopes      []string
		wantHas     bool
		wantPresent []string
		wantMissing []string
	}{
		{
			name:        "has all required scopes",
			scopes:      []string{"repo", "read:org", "gist"},
			wantHas:     true,
			wantPresent: []string{"repo"},
			wantMissing: []string{},
		},
		{
			name:        "missing repo scope",
			scopes:      []string{"read:org", "gist"},
			wantHas:     false,
			wantPresent: []string{},
			wantMissing: []string{"repo"},
		},
		{
			name:        "empty scopes",
			scopes:      []string{},
			wantHas:     false,
			wantPresent: []string{},
			wantMissing: []string{"repo"},
		},
		{
			name:        "only repo scope",
			scopes:      []string{"repo"},
			wantHas:     true,
			wantPresent: []string{"repo"},
			wantMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckRequiredScopes(tt.scopes)

			if result.HasRequired != tt.wantHas {
				t.Errorf("HasRequired = %v, want %v", result.HasRequired, tt.wantHas)
			}

			if len(result.Present) != len(tt.wantPresent) {
				t.Errorf("Present = %v, want %v", result.Present, tt.wantPresent)
			}
			for i, want := range tt.wantPresent {
				if i >= len(result.Present) || result.Present[i] != want {
					t.Errorf("Present[%d] = %v, want %v", i, result.Present, tt.wantPresent)
					break
				}
			}

			if len(result.Missing) != len(tt.wantMissing) {
				t.Errorf("Missing = %v, want %v", result.Missing, tt.wantMissing)
			}
			for i, want := range tt.wantMissing {
				if i >= len(result.Missing) || result.Missing[i] != want {
					t.Errorf("Missing[%d] = %v, want %v", i, result.Missing, tt.wantMissing)
					break
				}
			}
		})
	}
}

func TestVerifyResult_Errors(t *testing.T) {
	// Test that VerifyResult properly collects errors
	result := &VerifyResult{
		Errors: []error{},
	}

	if len(result.Errors) != 0 {
		t.Error("expected empty errors slice")
	}

	// Test that auth status can be nil
	if result.AuthStatus != nil {
		t.Error("expected nil AuthStatus")
	}

	// Test that scopes can be nil
	if result.Scopes != nil {
		t.Error("expected nil Scopes")
	}

	// Test that repo permissions can be nil
	if result.RepoPermissions != nil {
		t.Error("expected nil RepoPermissions")
	}
}

func TestAuthStatus_Fields(t *testing.T) {
	status := &AuthStatus{
		Authenticated: true,
		Username:      "testuser",
		Scopes:        []string{"repo", "gist"},
	}

	if !status.Authenticated {
		t.Error("expected Authenticated to be true")
	}
	if status.Username != "testuser" {
		t.Errorf("Username = %q, want %q", status.Username, "testuser")
	}
	if len(status.Scopes) != 2 {
		t.Errorf("Scopes = %v, want 2 items", status.Scopes)
	}
}

func TestRepoPermissions_Fields(t *testing.T) {
	perms := &RepoPermissions{
		Owner:    "owner",
		Repo:     "repo",
		Pull:     true,
		Push:     true,
		Maintain: false,
		Admin:    false,
	}

	if perms.Owner != "owner" {
		t.Errorf("Owner = %q, want %q", perms.Owner, "owner")
	}
	if perms.Repo != "repo" {
		t.Errorf("Repo = %q, want %q", perms.Repo, "repo")
	}
	if !perms.Pull {
		t.Error("expected Pull to be true")
	}
	if !perms.Push {
		t.Error("expected Push to be true")
	}
	if perms.Maintain {
		t.Error("expected Maintain to be false")
	}
	if perms.Admin {
		t.Error("expected Admin to be false")
	}
}

func TestScopesResult_Fields(t *testing.T) {
	result := &ScopesResult{
		HasRequired: true,
		Present:     []string{"repo"},
		Missing:     []string{},
	}

	if !result.HasRequired {
		t.Error("expected HasRequired to be true")
	}
	if len(result.Present) != 1 || result.Present[0] != "repo" {
		t.Errorf("Present = %v, want [repo]", result.Present)
	}
	if len(result.Missing) != 0 {
		t.Errorf("Missing = %v, want empty", result.Missing)
	}
}
