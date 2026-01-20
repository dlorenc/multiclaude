package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Updater handles the update process
type Updater struct {
	modulePath string
}

// NewUpdater creates a new updater
func NewUpdater() *Updater {
	return &Updater{
		modulePath: ModulePath,
	}
}

// UpdateResult contains the result of an update operation
type UpdateResult struct {
	PreviousVersion string
	NewVersion      string
	BinaryPath      string
	Success         bool
	Error           error
}

// Update installs the latest version of multiclaude using `go install`
func (u *Updater) Update(ctx context.Context) (*UpdateResult, error) {
	result := &UpdateResult{}

	// Get current executable path
	currentExe, err := os.Executable()
	if err != nil {
		result.Error = fmt.Errorf("failed to get current executable: %w", err)
		return result, result.Error
	}
	currentExe, _ = filepath.EvalSymlinks(currentExe)

	// Check if this looks like a go-installed binary
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	goBin := filepath.Join(gopath, "bin")

	if !strings.HasPrefix(currentExe, goBin) {
		result.Error = fmt.Errorf("multiclaude does not appear to be installed via 'go install' (binary at %s, expected under %s). Please update using your package manager or installation method", currentExe, goBin)
		return result, result.Error
	}

	// Run go install to update
	installCmd := exec.CommandContext(ctx, "go", "install", u.modulePath+"/cmd/multiclaude@latest")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr

	if err := installCmd.Run(); err != nil {
		result.Error = fmt.Errorf("failed to install update: %w", err)
		return result, result.Error
	}

	result.Success = true
	result.BinaryPath = filepath.Join(goBin, "multiclaude")

	return result, nil
}

// UpdateWithRetry attempts the update with retries
func (u *Updater) UpdateWithRetry(ctx context.Context, maxRetries int) (*UpdateResult, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		result, err := u.Update(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Wait before retry (exponential backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<uint(i)) * time.Second):
			// Continue to retry
		}
	}

	return nil, fmt.Errorf("update failed after %d attempts: %w", maxRetries, lastErr)
}

// CanUpdate checks if we can perform an update (i.e., installed via go install)
func (u *Updater) CanUpdate() (bool, string) {
	currentExe, err := os.Executable()
	if err != nil {
		return false, "cannot determine executable path"
	}
	currentExe, _ = filepath.EvalSymlinks(currentExe)

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	goBin := filepath.Join(gopath, "bin")

	if strings.HasPrefix(currentExe, goBin) {
		return true, ""
	}

	return false, fmt.Sprintf("binary at %s is not under GOPATH/bin (%s)", currentExe, goBin)
}
