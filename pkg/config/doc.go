package config

// PathDoc describes a single path for documentation purposes
type PathDoc struct {
	Path        string // Relative path from ~/.multiclaude/
	Description string // What this path is used for
	Type        string // "file" or "directory"
	Notes       string // Additional implementation notes
}

// DirectoryDocs returns documentation for all paths in ~/.multiclaude/
// This is the single source of truth for directory structure documentation.
func DirectoryDocs() []PathDoc {
	return []PathDoc{
		{
			Path:        "daemon.pid",
			Description: "Contains the process ID of the running multiclaude daemon",
			Type:        "file",
			Notes:       "Text file with a single integer. Deleted on clean daemon shutdown.",
		},
		{
			Path:        "daemon.sock",
			Description: "Unix domain socket for CLI-to-daemon communication",
			Type:        "file",
			Notes:       "Created with mode 0600 for security. The CLI connects here to send commands.",
		},
		{
			Path:        "daemon.log",
			Description: "Append-only log of daemon activity",
			Type:        "file",
			Notes:       "Useful for debugging daemon issues. Check this when agents behave unexpectedly.",
		},
		{
			Path:        "state.json",
			Description: "Central state file containing all tracked repositories and agents",
			Type:        "file",
			Notes:       "Written atomically via temp file + rename. See StateDoc() for format details.",
		},
		{
			Path:        "repos/",
			Description: "Contains cloned git repositories (bare or working)",
			Type:        "directory",
			Notes:       "Each repository is stored in repos/<repo-name>/",
		},
		{
			Path:        "repos/<repo-name>/",
			Description: "A cloned git repository",
			Type:        "directory",
			Notes:       "Full git clone of the tracked repository.",
		},
		{
			Path:        "wts/",
			Description: "Git worktrees for isolated agent working directories",
			Type:        "directory",
			Notes:       "Each agent gets its own worktree to work independently.",
		},
		{
			Path:        "wts/<repo-name>/",
			Description: "Worktrees directory for a specific repository",
			Type:        "directory",
			Notes:       "Contains subdirectories for each agent working on this repo.",
		},
		{
			Path:        "wts/<repo-name>/<agent-name>/",
			Description: "An agent's isolated git worktree",
			Type:        "directory",
			Notes:       "Agent types: supervisor, merge-queue, or worker names like happy-platypus.",
		},
		{
			Path:        "messages/",
			Description: "Inter-agent message files for coordination",
			Type:        "directory",
			Notes:       "Agents communicate via JSON message files in this directory.",
		},
		{
			Path:        "messages/<repo-name>/",
			Description: "Messages directory for a specific repository",
			Type:        "directory",
			Notes:       "Contains subdirectories for each agent that can receive messages.",
		},
		{
			Path:        "messages/<repo-name>/<agent-name>/",
			Description: "Inbox directory for a specific agent",
			Type:        "directory",
			Notes:       "Contains msg-<uuid>.json files addressed to this agent.",
		},
		{
			Path:        "prompts/",
			Description: "Generated prompt files for agents",
			Type:        "directory",
			Notes:       "Created on-demand. Contains <agent-name>.md prompt files.",
		},
	}
}

// StateDoc returns documentation for the state.json file format
type StateFieldDoc struct {
	Field       string // JSON field path
	Type        string // Go type
	Description string // What this field represents
}

// StateDocs returns documentation for state.json fields
func StateDocs() []StateFieldDoc {
	return []StateFieldDoc{
		// Top level
		{Field: "repos", Type: "map[string]*Repository", Description: "Map of repository name to repository state"},

		// Repository fields
		{Field: "repos.<name>.github_url", Type: "string", Description: "GitHub URL of the repository"},
		{Field: "repos.<name>.tmux_session", Type: "string", Description: "Name of the tmux session for this repo"},
		{Field: "repos.<name>.agents", Type: "map[string]Agent", Description: "Map of agent name to agent state"},

		// Agent fields
		{Field: "repos.<name>.agents.<name>.type", Type: "string", Description: "Agent type: supervisor, worker, merge-queue, or workspace"},
		{Field: "repos.<name>.agents.<name>.worktree_path", Type: "string", Description: "Absolute path to the agent's git worktree"},
		{Field: "repos.<name>.agents.<name>.tmux_window", Type: "string", Description: "Tmux window name for this agent"},
		{Field: "repos.<name>.agents.<name>.session_id", Type: "string", Description: "UUID for Claude session context"},
		{Field: "repos.<name>.agents.<name>.pid", Type: "int", Description: "Process ID of the Claude process"},
		{Field: "repos.<name>.agents.<name>.task", Type: "string", Description: "Task description (workers only, omitempty)"},
		{Field: "repos.<name>.agents.<name>.created_at", Type: "time.Time", Description: "When the agent was created"},
		{Field: "repos.<name>.agents.<name>.last_nudge", Type: "time.Time", Description: "Last time agent was nudged (omitempty)"},
		{Field: "repos.<name>.agents.<name>.ready_for_cleanup", Type: "bool", Description: "Whether worker is ready to be cleaned up (workers only, omitempty)"},
	}
}

// MessageDoc returns documentation for message file format
type MessageFieldDoc struct {
	Field       string
	Type        string
	Description string
}

// MessageDocs returns documentation for message JSON files
func MessageDocs() []MessageFieldDoc {
	return []MessageFieldDoc{
		{Field: "id", Type: "string", Description: "Message ID in format msg-<uuid>"},
		{Field: "from", Type: "string", Description: "Sender agent name"},
		{Field: "to", Type: "string", Description: "Recipient agent name"},
		{Field: "timestamp", Type: "time.Time", Description: "When the message was sent"},
		{Field: "body", Type: "string", Description: "Message content (markdown text)"},
		{Field: "status", Type: "string", Description: "Message status: pending, delivered, read, or acked"},
		{Field: "acked_at", Type: "time.Time", Description: "When the message was acknowledged (omitempty)"},
	}
}
