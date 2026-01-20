package update

import (
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest   string
		current  string
		expected bool
	}{
		{"v1.0.0", "v0.9.0", true},
		{"v1.1.0", "v1.0.0", true},
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.9.0", "v1.0.0", false},
		{"v2.0.0", "v1.9.9", true},
		{"1.0.0", "0.9.0", true},        // Without v prefix
		{"v1.0.0-beta", "v0.9.0", true}, // With pre-release suffix
		{"v1.0.0", "v1.0.0-beta", false}, // Pre-release vs release
	}

	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			result := isNewerVersion(tt.latest, tt.current)
			if result != tt.expected {
				t.Errorf("isNewerVersion(%s, %s) = %v, want %v", tt.latest, tt.current, result, tt.expected)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"1.0.0", [3]int{1, 0, 0}},
		{"0.9.1", [3]int{0, 9, 1}},
		{"1.2.3-beta", [3]int{1, 2, 3}},
		{"v1.2.3-rc1", [3]int{1, 2, 3}},
		{"1.2", [3]int{1, 2, 0}},
		{"1", [3]int{1, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			// Strip v prefix for parseVersion as isNewerVersion does
			v := tt.version
			if len(v) > 0 && v[0] == 'v' {
				v = v[1:]
			}
			result := parseVersion(v)
			if result != tt.expected {
				t.Errorf("parseVersion(%s) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestNewChecker(t *testing.T) {
	checker := NewChecker("v1.0.0")
	if checker.currentVersion != "v1.0.0" {
		t.Errorf("NewChecker() currentVersion = %s, want v1.0.0", checker.currentVersion)
	}
	if checker.modulePath != ModulePath {
		t.Errorf("NewChecker() modulePath = %s, want %s", checker.modulePath, ModulePath)
	}
}
