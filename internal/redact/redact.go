package redact

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

// Redactor maintains consistent mappings for redacted values
type Redactor struct {
	mu           sync.Mutex
	repoNames    map[string]string
	agentNames   map[string]string
	repoCounter  int
	agentCounter map[string]int // per-type counters
	homeDir      string
}

// New creates a new Redactor instance
func New() *Redactor {
	home, _ := os.UserHomeDir()
	return &Redactor{
		repoNames:    make(map[string]string),
		agentNames:   make(map[string]string),
		agentCounter: make(map[string]int),
		homeDir:      home,
	}
}

// RepoName redacts a repository name with a consistent mapping
func (r *Redactor) RepoName(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if redacted, ok := r.repoNames[name]; ok {
		return redacted
	}

	r.repoCounter++
	redacted := "repo-" + itoa(r.repoCounter)
	r.repoNames[name] = redacted
	return redacted
}

// AgentName redacts an agent name with a consistent mapping based on type
func (r *Redactor) AgentName(name, agentType string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := agentType + ":" + name
	if redacted, ok := r.agentNames[key]; ok {
		return redacted
	}

	r.agentCounter[agentType]++
	redacted := agentType + "-" + itoa(r.agentCounter[agentType])
	r.agentNames[key] = redacted
	return redacted
}

// Path redacts file paths by replacing home directory and sensitive parts
func (r *Redactor) Path(path string) string {
	if r.homeDir != "" && strings.HasPrefix(path, r.homeDir) {
		path = "/Users/<user>" + path[len(r.homeDir):]
	}

	// Redact repo names in paths (e.g., /Users/<user>/.multiclaude/repos/myrepo)
	r.mu.Lock()
	for original, redacted := range r.repoNames {
		path = strings.ReplaceAll(path, "/"+original+"/", "/"+redacted+"/")
		path = strings.ReplaceAll(path, "/"+original, "/"+redacted)
	}
	r.mu.Unlock()

	return path
}

// GitHubURL redacts GitHub URLs to hide owner/repo info
func (r *Redactor) GitHubURL(url string) string {
	// Match patterns like https://github.com/owner/repo or git@github.com:owner/repo
	httpsPattern := regexp.MustCompile(`https://github\.com/[^/]+/[^/\s]+`)
	sshPattern := regexp.MustCompile(`git@github\.com:[^/]+/[^/\s]+`)

	url = httpsPattern.ReplaceAllString(url, "https://github.com/<owner>/<repo>")
	url = sshPattern.ReplaceAllString(url, "git@github.com:<owner>/<repo>")

	return url
}

// Text redacts all sensitive information in a block of text
func (r *Redactor) Text(text string) string {
	// Redact home directory paths
	if r.homeDir != "" {
		text = strings.ReplaceAll(text, r.homeDir, "/Users/<user>")
	}

	// Redact GitHub URLs
	httpsPattern := regexp.MustCompile(`https://github\.com/[^/\s]+/[^/\s]+`)
	sshPattern := regexp.MustCompile(`git@github\.com:[^/\s]+/[^/\s]+`)
	text = httpsPattern.ReplaceAllString(text, "https://github.com/<owner>/<repo>")
	text = sshPattern.ReplaceAllString(text, "git@github.com:<owner>/<repo>")

	// Redact known repo names in text
	r.mu.Lock()
	for original, redacted := range r.repoNames {
		// Only replace whole words to avoid partial matches
		wordPattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(original) + `\b`)
		text = wordPattern.ReplaceAllString(text, redacted)
	}
	r.mu.Unlock()

	return text
}

// itoa converts an int to string without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
