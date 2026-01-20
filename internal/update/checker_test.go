package update

import (
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		{"same version", "v1.0.0", "v1.0.0", false},
		{"patch update", "v1.0.1", "v1.0.0", true},
		{"minor update", "v1.1.0", "v1.0.0", true},
		{"major update", "v2.0.0", "v1.0.0", true},
		{"current newer patch", "v1.0.0", "v1.0.1", false},
		{"current newer minor", "v1.0.0", "v1.1.0", false},
		{"current newer major", "v1.0.0", "v2.0.0", false},
		{"without v prefix", "1.0.1", "1.0.0", true},
		{"mixed prefix", "v1.0.1", "1.0.0", true},
		{"pre-release ignored", "v1.0.1-beta", "v1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    [3]int
	}{
		{"full version", "1.2.3", [3]int{1, 2, 3}},
		{"major only", "1", [3]int{1, 0, 0}},
		{"major.minor", "1.2", [3]int{1, 2, 0}},
		{"with v prefix", "v1.2.3", [3]int{1, 2, 3}},
		{"with pre-release", "1.2.3-beta", [3]int{1, 2, 3}},
		{"with build metadata", "1.2.3+build", [3]int{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Strip v prefix as parseVersion expects
			v := tt.version
			if len(v) > 0 && v[0] == 'v' {
				v = v[1:]
			}
			got := parseVersion(v)
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
