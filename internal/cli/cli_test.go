package cli

import (
	"strings"
	"testing"
	"time"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantFlags    map[string]string
		wantPositional []string
	}{
		{
			name:         "empty args",
			args:         []string{},
			wantFlags:    map[string]string{},
			wantPositional: nil,
		},
		{
			name:         "positional only",
			args:         []string{"arg1", "arg2", "arg3"},
			wantFlags:    map[string]string{},
			wantPositional: []string{"arg1", "arg2", "arg3"},
		},
		{
			name:         "long flag with value",
			args:         []string{"--repo", "myrepo"},
			wantFlags:    map[string]string{"repo": "myrepo"},
			wantPositional: nil,
		},
		{
			name:         "long flag boolean",
			args:         []string{"--verbose"},
			wantFlags:    map[string]string{"verbose": "true"},
			wantPositional: nil,
		},
		{
			name:         "short flag with value",
			args:         []string{"-r", "myrepo"},
			wantFlags:    map[string]string{"r": "myrepo"},
			wantPositional: nil,
		},
		{
			name:         "short flag boolean",
			args:         []string{"-v"},
			wantFlags:    map[string]string{"v": "true"},
			wantPositional: nil,
		},
		{
			name:         "mixed flags and positional",
			args:         []string{"--repo", "myrepo", "task", "description", "-v"},
			wantFlags:    map[string]string{"repo": "myrepo", "v": "true"},
			wantPositional: []string{"task", "description"},
		},
		{
			name:         "multiple long flags",
			args:         []string{"--name", "worker1", "--branch", "main", "--dry-run"},
			wantFlags:    map[string]string{"name": "worker1", "branch": "main", "dry-run": "true"},
			wantPositional: nil,
		},
		{
			name:         "flag followed by flag (boolean)",
			args:         []string{"--verbose", "--debug"},
			wantFlags:    map[string]string{"verbose": "true", "debug": "true"},
			wantPositional: nil,
		},
		{
			name:         "positional before flags",
			args:         []string{"command", "--flag", "value"},
			wantFlags:    map[string]string{"flag": "value"},
			wantPositional: []string{"command"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlags, gotPositional := ParseFlags(tt.args)

			// Check flags
			if len(gotFlags) != len(tt.wantFlags) {
				t.Errorf("ParseFlags() flags len = %d, want %d", len(gotFlags), len(tt.wantFlags))
			}
			for k, v := range tt.wantFlags {
				if gotFlags[k] != v {
					t.Errorf("ParseFlags() flags[%q] = %q, want %q", k, gotFlags[k], v)
				}
			}

			// Check positional
			if len(gotPositional) != len(tt.wantPositional) {
				t.Errorf("ParseFlags() positional len = %d, want %d", len(gotPositional), len(tt.wantPositional))
			}
			for i, v := range tt.wantPositional {
				if i < len(gotPositional) && gotPositional[i] != v {
					t.Errorf("ParseFlags() positional[%d] = %q, want %q", i, gotPositional[i], v)
				}
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		wantType string // "time" for HH:MM:SS format, "date" for date format
	}{
		{
			name:     "recent time (today)",
			time:     time.Now().Add(-1 * time.Hour),
			wantType: "time",
		},
		{
			name:     "old time (yesterday)",
			time:     time.Now().Add(-25 * time.Hour),
			wantType: "date",
		},
		{
			name:     "old time (last week)",
			time:     time.Now().Add(-7 * 24 * time.Hour),
			wantType: "date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTime(tt.time)

			if tt.wantType == "time" {
				// Should contain colons (HH:MM:SS format)
				if !strings.Contains(got, ":") {
					t.Errorf("formatTime() = %q, expected time format with colons", got)
				}
				// Should not contain month abbreviation
				months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
				for _, m := range months {
					if strings.Contains(got, m) {
						t.Errorf("formatTime() = %q, expected time-only format without month", got)
					}
				}
			} else {
				// Should contain month abbreviation
				hasMonth := false
				months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
				for _, m := range months {
					if strings.Contains(got, m) {
						hasMonth = true
						break
					}
				}
				if !hasMonth {
					t.Errorf("formatTime() = %q, expected date format with month", got)
				}
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			s:      "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			s:      "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string",
			s:      "hello world this is a long string",
			maxLen: 15,
			want:   "hello world ...",
		},
		{
			name:   "empty string",
			s:      "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "truncate to minimum",
			s:      "abcdefgh",
			maxLen: 4,
			want:   "a...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString() = %q, want %q", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("truncateString() len = %d, exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestGenerateSessionID(t *testing.T) {
	// Generate multiple session IDs and verify uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := generateSessionID()
		if err != nil {
			t.Fatalf("generateSessionID() error = %v", err)
		}

		// Check format: UUID v4 format xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("generateSessionID() = %q, expected 5 parts separated by dashes", id)
		}

		// Check part lengths: 8-4-4-4-12
		expectedLens := []int{8, 4, 4, 4, 12}
		for j, part := range parts {
			if len(part) != expectedLens[j] {
				t.Errorf("generateSessionID() part %d len = %d, want %d", j, len(part), expectedLens[j])
			}
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("generateSessionID() generated duplicate ID: %q", id)
		}
		ids[id] = true
	}
}

func TestGenerateDocumentation(t *testing.T) {
	// Create a minimal CLI with commands registered
	cli := &CLI{
		paths: nil, // Not needed for doc generation
		rootCmd: &Command{
			Name:        "test",
			Description: "test cli",
			Subcommands: make(map[string]*Command),
		},
	}

	// Add some test commands
	cli.rootCmd.Subcommands["start"] = &Command{
		Name:        "start",
		Description: "Start the daemon",
		Usage:       "test start",
	}
	cli.rootCmd.Subcommands["stop"] = &Command{
		Name:        "stop",
		Description: "Stop the daemon",
	}
	cli.rootCmd.Subcommands["work"] = &Command{
		Name:        "work",
		Description: "Worker commands",
		Subcommands: map[string]*Command{
			"list": {
				Name:        "list",
				Description: "List workers",
			},
			"rm": {
				Name:        "rm",
				Description: "Remove a worker",
				Usage:       "test work rm <name>",
			},
		},
	}

	docs := cli.GenerateDocumentation()

	// Verify documentation contains expected content
	if !strings.Contains(docs, "# Multiclaude CLI Reference") {
		t.Error("GenerateDocumentation() missing header")
	}
	if !strings.Contains(docs, "## start") {
		t.Error("GenerateDocumentation() missing start command")
	}
	if !strings.Contains(docs, "Start the daemon") {
		t.Error("GenerateDocumentation() missing start description")
	}
	if !strings.Contains(docs, "## work") {
		t.Error("GenerateDocumentation() missing work command")
	}
	if !strings.Contains(docs, "**Subcommands:**") {
		t.Error("GenerateDocumentation() missing subcommands section")
	}
	if !strings.Contains(docs, "**Usage:**") {
		t.Error("GenerateDocumentation() missing usage section")
	}
}
