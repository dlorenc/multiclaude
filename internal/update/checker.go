// Package update provides functionality for checking and applying updates to multiclaude.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ModulePath is the Go module path for multiclaude
const ModulePath = "github.com/dlorenc/multiclaude"

// ModuleInfo represents the JSON output from `go list -m -u -json`
type ModuleInfo struct {
	Path    string       `json:"Path"`
	Version string       `json:"Version"`
	Update  *UpdateInfo  `json:"Update,omitempty"`
	Error   *ModuleError `json:"Error,omitempty"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
}

// ModuleError represents an error from go list
type ModuleError struct {
	Err string `json:"Err"`
}

// Result represents the result of an update check
type Result struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	LastChecked     time.Time
	Error           error
}

// Checker checks for available updates
type Checker struct {
	modulePath     string
	currentVersion string
}

// NewChecker creates a new update checker
func NewChecker(currentVersion string) *Checker {
	return &Checker{
		modulePath:     ModulePath,
		currentVersion: currentVersion,
	}
}

// Check checks for available updates using `go list -m -u -json`
func (c *Checker) Check(ctx context.Context) (*Result, error) {
	result := &Result{
		CurrentVersion: c.currentVersion,
		LastChecked:    time.Now(),
	}

	// Run: go list -m -u -json github.com/dlorenc/multiclaude@latest
	// The @latest suffix ensures we check the proxy for the latest version
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-u", "-json", c.modulePath+"@latest")

	output, err := cmd.Output()
	if err != nil {
		// If go list fails, try alternative approach
		// This can happen if the module isn't in the local cache
		result.Error = fmt.Errorf("failed to check for updates: %w", err)
		return result, result.Error
	}

	var info ModuleInfo
	if err := json.Unmarshal(output, &info); err != nil {
		result.Error = fmt.Errorf("failed to parse update info: %w", err)
		return result, result.Error
	}

	if info.Error != nil {
		result.Error = fmt.Errorf("go list error: %s", info.Error.Err)
		return result, result.Error
	}

	result.LatestVersion = info.Version

	// Compare versions - if current version is "dev", always show latest as available
	if c.currentVersion == "dev" || c.currentVersion == "" {
		// Development version - show what's available but don't flag as update
		result.UpdateAvailable = false
	} else {
		// Compare semantic versions
		result.UpdateAvailable = isNewerVersion(info.Version, c.currentVersion)
	}

	return result, nil
}

// CheckWithFallback tries to check for updates, falling back to a simpler approach if needed
func (c *Checker) CheckWithFallback(ctx context.Context) (*Result, error) {
	result, err := c.Check(ctx)
	if err == nil {
		return result, nil
	}

	// Fallback: try using go list -m -versions to list available versions
	return c.checkViaVersionList(ctx)
}

// checkViaVersionList uses `go list -m -versions` to check available versions
func (c *Checker) checkViaVersionList(ctx context.Context) (*Result, error) {
	result := &Result{
		CurrentVersion: c.currentVersion,
		LastChecked:    time.Now(),
	}

	// Try to get version info from proxy directly
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-versions", c.modulePath)
	output, err := cmd.Output()
	if err != nil {
		result.Error = fmt.Errorf("failed to list versions: %w", err)
		return result, result.Error
	}

	// Output format: "github.com/dlorenc/multiclaude v0.1.0 v0.2.0 v0.3.0"
	parts := strings.Fields(string(output))
	if len(parts) < 2 {
		result.Error = fmt.Errorf("no versions found for module")
		return result, result.Error
	}

	// Last version in the list is the latest
	result.LatestVersion = parts[len(parts)-1]

	if c.currentVersion == "dev" || c.currentVersion == "" {
		result.UpdateAvailable = false
	} else {
		result.UpdateAvailable = isNewerVersion(result.LatestVersion, c.currentVersion)
	}

	return result, nil
}

// isNewerVersion compares two semantic version strings
// Returns true if latest is newer than current
func isNewerVersion(latest, current string) bool {
	// Strip 'v' prefix if present
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	// Handle special cases
	if latest == current {
		return false
	}

	// Parse versions
	latestParts := parseVersion(latest)
	currentParts := parseVersion(current)

	// Compare major.minor.patch
	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}

	return false
}

// parseVersion parses a semantic version string into major, minor, patch
func parseVersion(v string) [3]int {
	var parts [3]int

	// Handle pre-release suffix (e.g., v1.2.3-beta)
	if idx := strings.Index(v, "-"); idx > 0 {
		v = v[:idx]
	}

	segments := strings.Split(v, ".")
	for i := 0; i < len(segments) && i < 3; i++ {
		var n int
		fmt.Sscanf(segments[i], "%d", &n)
		parts[i] = n
	}

	return parts
}
