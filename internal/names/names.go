package names

import (
	"math/rand"
	"regexp"
	"strings"
	"time"
)

var (
	adjectives = []string{
		"happy", "clever", "brave", "calm", "eager",
		"fancy", "gentle", "jolly", "kind", "lively",
		"nice", "proud", "silly", "witty", "zealous",
		"bright", "swift", "bold", "cool", "wise",
	}

	animals = []string{
		"platypus", "elephant", "dolphin", "penguin", "koala",
		"otter", "panda", "tiger", "lion", "bear",
		"fox", "wolf", "eagle", "hawk", "owl",
		"deer", "rabbit", "squirrel", "badger", "raccoon",
	}

	// Stop words to filter out when extracting task names
	stopWords = map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "are": true, "am": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "should": true, "could": true, "may": true,
		"might": true, "must": true, "can": true, "to": true, "for": true, "of": true,
		"in": true, "on": true, "at": true, "by": true, "with": true, "from": true,
		"as": true, "into": true, "through": true, "this": true, "that": true,
		"these": true, "those": true, "it": true, "its": true, "they": true,
		"their": true, "there": true, "here": true, "and": true, "or": true,
		"but": true, "if": true, "because": true, "when": true, "where": true,
		"how": true, "what": true, "which": true, "who": true, "why": true,
	}

	rng *rand.Rand

	// Regex for sanitizing names
	invalidCharsRegex    = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// Generate creates a Docker-style name (adjective-animal)
func Generate() string {
	adj := adjectives[rng.Intn(len(adjectives))]
	animal := animals[rng.Intn(len(animals))]
	return adj + "-" + animal
}

// FromTask generates a descriptive worker name from a task description.
// It extracts 3-4 meaningful keywords, sanitizes them to lowercase-hyphenated format,
// and falls back to Generate() if extraction fails.
func FromTask(task string) string {
	// Extract keywords
	keywords := extractKeywords(task)
	if len(keywords) == 0 {
		return Generate()
	}

	// Limit to 3-4 words
	if len(keywords) > 4 {
		keywords = keywords[:4]
	}

	// Join and sanitize
	name := strings.Join(keywords, "-")
	name = sanitizeName(name)

	// Validate
	if !isValidName(name) {
		return Generate()
	}

	return name
}

// extractKeywords extracts meaningful keywords from a task description
func extractKeywords(task string) []string {
	// Normalize to lowercase
	task = strings.ToLower(task)

	// Split into words
	words := strings.Fields(task)

	var keywords []string
	for _, word := range words {
		// Remove punctuation from word boundaries
		word = strings.Trim(word, ".,!?;:\"'`()[]{}/<>")

		// Skip if empty after trimming
		if word == "" {
			continue
		}

		// Skip stop words
		if stopWords[word] {
			continue
		}

		// Skip very short words (likely not meaningful)
		if len(word) < 2 {
			continue
		}

		keywords = append(keywords, word)
	}

	return keywords
}

// sanitizeName converts a name to a valid worker name format
func sanitizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace invalid characters with hyphens
	name = invalidCharsRegex.ReplaceAllString(name, "-")

	// Collapse multiple hyphens
	name = multipleHyphensRegex.ReplaceAllString(name, "-")

	// Trim hyphens from edges
	name = strings.Trim(name, "-")

	// Truncate if too long
	const maxLength = 50
	if len(name) > maxLength {
		name = name[:maxLength]
		// Ensure we don't end with a hyphen after truncation
		name = strings.TrimRight(name, "-")
	}

	return name
}

// isValidName checks if a generated name meets validation criteria
func isValidName(name string) bool {
	// Must be between 3 and 50 characters
	if len(name) < 3 || len(name) > 50 {
		return false
	}

	// Must contain at least one alphabetic character
	hasAlpha := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasAlpha = true
			break
		}
	}
	if !hasAlpha {
		return false
	}

	// Must not start or end with hyphen
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}

	// Must only contain valid characters
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}

	return true
}

// EnsureUnique adds a numeric suffix to the name if it already exists in the given list
func EnsureUnique(name string, existingNames []string) string {
	// Create a map for O(1) lookup
	exists := make(map[string]bool)
	for _, n := range existingNames {
		exists[n] = true
	}

	// If name is unique, return as-is
	if !exists[name] {
		return name
	}

	// Try numeric suffixes
	for i := 2; i < 1000; i++ {
		candidate := name + "-" + intToString(i)
		if !exists[candidate] {
			return candidate
		}
	}

	// Fallback to random name if we somehow exhaust numeric suffixes
	return Generate()
}

// intToString converts an integer to a string without importing fmt or strconv
func intToString(n int) string {
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
