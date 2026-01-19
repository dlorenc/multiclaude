// Package tmux provides tmux operations for internal use.
//
// This package re-exports functionality from pkg/tmux for backward compatibility.
// New code should prefer importing pkg/tmux directly.
package tmux

import (
	pkgtmux "github.com/dlorenc/multiclaude/pkg/tmux"
)

// Client wraps tmux operations.
// This is an alias to pkg/tmux.Client for backward compatibility.
type Client = pkgtmux.Client

// NewClient creates a new tmux client.
// This is an alias to pkg/tmux.NewClient for backward compatibility.
func NewClient(opts ...pkgtmux.ClientOption) *Client {
	return pkgtmux.NewClient(opts...)
}

// ClientOption is a functional option for configuring a Client.
// This is an alias to pkg/tmux.ClientOption for backward compatibility.
type ClientOption = pkgtmux.ClientOption

// WithTmuxPath sets a custom path to the tmux binary.
// This is an alias to pkg/tmux.WithTmuxPath for backward compatibility.
var WithTmuxPath = pkgtmux.WithTmuxPath

// PaneInfo is kept for backward compatibility.
// Deprecated: This type was unused and is kept only for API compatibility.
type PaneInfo struct {
	PID int
}
