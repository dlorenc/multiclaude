// Package agents provides infrastructure for reading and managing
// configurable agent definitions from markdown files.
package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Definition represents a parsed agent definition from a markdown file.
type Definition struct {
	// Name is the agent name, derived from the filename (without .md extension)
	Name string

	// Content is the full markdown content of the agent definition
	Content string

	// SourcePath is the absolute path to the source file
	SourcePath string

	// Source indicates where this definition came from
	Source DefinitionSource
}

// DefinitionSource indicates the origin of an agent definition
type DefinitionSource string

const (
	// SourceLocal indicates the definition came from ~/.multiclaude/repos/<repo>/agents/
	SourceLocal DefinitionSource = "local"

	// SourceRepo indicates the definition came from <repo>/.multiclaude/agents/
	SourceRepo DefinitionSource = "repo"

	// SourceMerged indicates the definition is a merge of local (base) and repo (custom) content
	SourceMerged DefinitionSource = "merged"
)

// Reader reads agent definitions from the filesystem.
type Reader struct {
	// localAgentsDir is ~/.multiclaude/repos/<repo>/agents/
	localAgentsDir string

	// repoAgentsDir is <repo>/.multiclaude/agents/
	repoAgentsDir string
}

// NewReader creates a new agent definition reader.
// localAgentsDir is the path to ~/.multiclaude/repos/<repo>/agents/
// repoPath is the path to the cloned repository (will look for .multiclaude/agents/ inside)
func NewReader(localAgentsDir, repoPath string) *Reader {
	repoAgentsDir := ""
	if repoPath != "" {
		repoAgentsDir = filepath.Join(repoPath, ".multiclaude", "agents")
	}

	return &Reader{
		localAgentsDir: localAgentsDir,
		repoAgentsDir:  repoAgentsDir,
	}
}

// ReadLocalDefinitions reads agent definitions from ~/.multiclaude/repos/<repo>/agents/*.md
func (r *Reader) ReadLocalDefinitions() ([]Definition, error) {
	return readDefinitionsFromDir(r.localAgentsDir, SourceLocal)
}

// ReadRepoDefinitions reads agent definitions from <repo>/.multiclaude/agents/*.md
// Returns an empty slice (not an error) if the directory doesn't exist.
func (r *Reader) ReadRepoDefinitions() ([]Definition, error) {
	if r.repoAgentsDir == "" {
		return nil, nil
	}
	return readDefinitionsFromDir(r.repoAgentsDir, SourceRepo)
}

// ReadAllDefinitions reads and merges definitions from both local and repo directories.
// Checked-in repo definitions win over local definitions on filename conflict.
// Returns definitions sorted alphabetically by name.
func (r *Reader) ReadAllDefinitions() ([]Definition, error) {
	localDefs, err := r.ReadLocalDefinitions()
	if err != nil {
		return nil, fmt.Errorf("failed to read local definitions: %w", err)
	}

	repoDefs, err := r.ReadRepoDefinitions()
	if err != nil {
		return nil, fmt.Errorf("failed to read repo definitions: %w", err)
	}

	return MergeDefinitions(localDefs, repoDefs), nil
}

// MergeDefinitions merges local and repo definitions.
// When a repo definition has the same name as a local definition, the repo content
// is appended to the local content (preserving critical base instructions).
// New repo-only definitions are added as-is.
func MergeDefinitions(local, repo []Definition) []Definition {
	// Build a map with local definitions first
	merged := make(map[string]Definition, len(local)+len(repo))

	for _, def := range local {
		merged[def.Name] = def
	}

	// For repo definitions: append to local if exists, otherwise add as new
	for _, repoDef := range repo {
		if localDef, exists := merged[repoDef.Name]; exists {
			// Append repo content to local base template
			merged[repoDef.Name] = Definition{
				Name:       repoDef.Name,
				Content:    mergeContent(localDef.Content, repoDef.Content),
				SourcePath: localDef.SourcePath, // Keep local path as primary
				Source:     SourceMerged,
			}
		} else {
			// New repo-only definition, add as-is
			merged[repoDef.Name] = repoDef
		}
	}

	// Convert to sorted slice
	result := make([]Definition, 0, len(merged))
	for _, def := range merged {
		result = append(result, def)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// mergeContent appends custom content to base content with a clear separator.
func mergeContent(base, custom string) string {
	// Trim trailing whitespace from base and leading whitespace from custom
	base = strings.TrimRight(base, "\n\r\t ")
	custom = strings.TrimLeft(custom, "\n\r\t ")

	return base + "\n\n---\n\n## Custom Instructions\n\n" + custom
}

// readDefinitionsFromDir reads all .md files from a directory and returns them as definitions.
// Returns an empty slice (not an error) if the directory doesn't exist.
func readDefinitionsFromDir(dir string, source DefinitionSource) ([]Definition, error) {
	if dir == "" {
		return nil, nil
	}

	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var definitions []Definition

	for _, entry := range entries {
		// Skip directories and non-.md files
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		// Read file content
		filePath := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Extract name from filename (without .md extension)
		name := strings.TrimSuffix(entry.Name(), ".md")

		definitions = append(definitions, Definition{
			Name:       name,
			Content:    string(content),
			SourcePath: filePath,
			Source:     source,
		})
	}

	return definitions, nil
}

// ParseTitle extracts the title from a markdown definition.
// It looks for the first H1 heading (# Title) in the content.
// Returns the name as-is if no H1 heading is found.
func (d *Definition) ParseTitle() string {
	lines := strings.Split(d.Content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return d.Name
}

// ParseDescription extracts the first paragraph after the title as a description.
// Returns an empty string if no description is found.
func (d *Definition) ParseDescription() string {
	lines := strings.Split(d.Content, "\n")
	foundTitle := false
	var descLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip until we find the title
		if strings.HasPrefix(trimmed, "# ") {
			foundTitle = true
			continue
		}

		if !foundTitle {
			continue
		}

		// Skip empty lines before the description starts
		if len(descLines) == 0 && trimmed == "" {
			continue
		}

		// Stop at the next heading or empty line after content
		if strings.HasPrefix(trimmed, "#") || (len(descLines) > 0 && trimmed == "") {
			break
		}

		descLines = append(descLines, trimmed)
	}

	return strings.Join(descLines, " ")
}
