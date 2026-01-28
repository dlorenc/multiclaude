package names

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	// Generate multiple names to verify format and randomness
	generated := make(map[string]bool)

	for i := 0; i < 100; i++ {
		name := Generate()

		// Check format: adjective-animal
		parts := strings.Split(name, "-")
		if len(parts) != 2 {
			t.Errorf("Generate() = %q, expected format 'adjective-animal'", name)
		}

		// Verify adjective is valid
		adj := parts[0]
		validAdj := false
		for _, a := range adjectives {
			if a == adj {
				validAdj = true
				break
			}
		}
		if !validAdj {
			t.Errorf("Generate() produced invalid adjective: %q", adj)
		}

		// Verify animal is valid
		animal := parts[1]
		validAnimal := false
		for _, a := range animals {
			if a == animal {
				validAnimal = true
				break
			}
		}
		if !validAnimal {
			t.Errorf("Generate() produced invalid animal: %q", animal)
		}

		generated[name] = true
	}

	// With 20 adjectives and 20 animals (400 combinations),
	// generating 100 names should have some variety
	if len(generated) < 10 {
		t.Errorf("Generate() produced too few unique names: %d unique out of 100 calls", len(generated))
	}
}

func TestGenerateFormat(t *testing.T) {
	name := Generate()

	// Should contain exactly one hyphen
	if strings.Count(name, "-") != 1 {
		t.Errorf("Generate() = %q, expected exactly one hyphen", name)
	}

	// Should not be empty
	if name == "" {
		t.Error("Generate() returned empty string")
	}

	// Should be lowercase
	if name != strings.ToLower(name) {
		t.Errorf("Generate() = %q, expected lowercase", name)
	}

	// Should not contain spaces
	if strings.Contains(name, " ") {
		t.Errorf("Generate() = %q, should not contain spaces", name)
	}
}

func TestAdjectives(t *testing.T) {
	// Verify adjectives list has expected properties
	if len(adjectives) == 0 {
		t.Error("adjectives list is empty")
	}

	// All adjectives should be lowercase single words
	for _, adj := range adjectives {
		if adj == "" {
			t.Error("adjectives contains empty string")
		}
		if adj != strings.ToLower(adj) {
			t.Errorf("adjective %q is not lowercase", adj)
		}
		if strings.Contains(adj, " ") {
			t.Errorf("adjective %q contains space", adj)
		}
		if strings.Contains(adj, "-") {
			t.Errorf("adjective %q contains hyphen", adj)
		}
	}
}

func TestAnimals(t *testing.T) {
	// Verify animals list has expected properties
	if len(animals) == 0 {
		t.Error("animals list is empty")
	}

	// All animals should be lowercase single words
	for _, animal := range animals {
		if animal == "" {
			t.Error("animals contains empty string")
		}
		if animal != strings.ToLower(animal) {
			t.Errorf("animal %q is not lowercase", animal)
		}
		if strings.Contains(animal, " ") {
			t.Errorf("animal %q contains space", animal)
		}
		if strings.Contains(animal, "-") {
			t.Errorf("animal %q contains hyphen", animal)
		}
	}
}

func TestGenerateUniqueness(t *testing.T) {
	// Generate names and check for reasonable distribution
	counts := make(map[string]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		name := Generate()
		counts[name]++
	}

	// With 400 possible combinations, no single name should appear too often
	maxCount := 0
	for _, count := range counts {
		if count > maxCount {
			maxCount = count
		}
	}

	// Maximum expected with uniform distribution would be ~2.5 (1000/400)
	// Allow for some variance, but flag if one name appears more than 10 times
	if maxCount > 20 {
		t.Errorf("Generate() shows poor distribution: one name appeared %d times in %d iterations", maxCount, iterations)
	}
}

// Tests for task-based naming

func TestFromTask(t *testing.T) {
	tests := []struct {
		name     string
		task     string
		expected string
	}{
		{
			name:     "basic task with meaningful words",
			task:     "Fix the session ID bug in authentication",
			expected: "fix-session-id-bug",
		},
		{
			name:     "task with action verb",
			task:     "Add user profile editing feature",
			expected: "add-user-profile-editing",
		},
		{
			name:     "task with technical terms",
			task:     "Refactor the database connection logic",
			expected: "refactor-database-connection-logic",
		},
		{
			name:     "simple task",
			task:     "Update README documentation",
			expected: "update-readme-documentation",
		},
		{
			name:     "task with acronym",
			task:     "Implement OAuth2 login flow",
			expected: "implement-oauth2-login-flow",
		},
		{
			name:     "task with punctuation",
			task:     "Fix bug: session expires too quickly!",
			expected: "fix-bug-session-expires",
		},
		{
			name:     "task with special characters",
			task:     "Update API (v2) endpoint configuration",
			expected: "update-api-v2-endpoint",
		},
		{
			name:     "task with multiple spaces",
			task:     "Fix    the    spacing    issue",
			expected: "fix-spacing-issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromTask(tt.task)
			if result != tt.expected {
				t.Errorf("FromTask(%q) = %q, expected %q", tt.task, result, tt.expected)
			}
		})
	}
}

func TestFromTaskFallback(t *testing.T) {
	tests := []struct {
		name string
		task string
	}{
		{"empty string", ""},
		{"only stop words", "the a an is are to for"},
		{"only punctuation", "!@#$%^&*()"},
		{"only spaces", "     "},
		{"very short", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromTask(tt.task)
			// Should fall back to random name (adjective-animal format)
			parts := strings.Split(result, "-")
			if len(parts) != 2 {
				t.Errorf("FromTask(%q) = %q, expected fallback to adjective-animal format", tt.task, result)
			}
		})
	}
}

func TestFromTaskTruncation(t *testing.T) {
	// Very long task should be truncated to max 50 characters
	longTask := "Fix the extremely long and verbose task description that goes on and on with many words"
	result := FromTask(longTask)

	if len(result) > 50 {
		t.Errorf("FromTask() produced name longer than 50 chars: %q (%d chars)", result, len(result))
	}

	// Should still be valid
	if !isValidName(result) {
		t.Errorf("FromTask() produced invalid name after truncation: %q", result)
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		task     string
		expected []string
	}{
		{
			name:     "basic extraction",
			task:     "Fix the bug in the system",
			expected: []string{"fix", "bug", "system"},
		},
		{
			name:     "filters stop words",
			task:     "The user can login to the system",
			expected: []string{"user", "login", "system"},
		},
		{
			name:     "handles punctuation",
			task:     "Fix bug: system crashes!",
			expected: []string{"fix", "bug", "system", "crashes"},
		},
		{
			name:     "empty string",
			task:     "",
			expected: []string{},
		},
		{
			name:     "only stop words",
			task:     "the a an is",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKeywords(tt.task)
			if len(result) != len(tt.expected) {
				t.Errorf("extractKeywords(%q) = %v, expected %v", tt.task, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("extractKeywords(%q) = %v, expected %v", tt.task, result, tt.expected)
					return
				}
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic sanitization",
			input:    "Fix Bug System",
			expected: "fix-bug-system",
		},
		{
			name:     "removes special chars",
			input:    "fix@bug#system",
			expected: "fix-bug-system",
		},
		{
			name:     "collapses multiple hyphens",
			input:    "fix---bug---system",
			expected: "fix-bug-system",
		},
		{
			name:     "trims edge hyphens",
			input:    "-fix-bug-system-",
			expected: "fix-bug-system",
		},
		{
			name:     "handles mixed case",
			input:    "FixBugSystem",
			expected: "fixbugsystem",
		},
		{
			name:     "truncates long names",
			input:    "this-is-a-very-long-name-that-exceeds-the-maximum-length-limit-for-worker-names",
			expected: "this-is-a-very-long-name-that-exceeds-the-maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"valid name", "fix-bug-system", true},
		{"valid short name", "fix", true},
		{"valid with numbers", "fix-bug-v2", true},
		{"too short", "ab", false},
		{"too long", strings.Repeat("a", 51), false},
		{"starts with hyphen", "-fix-bug", false},
		{"ends with hyphen", "fix-bug-", false},
		{"no alphabetic chars", "123-456", false},
		{"has uppercase", "Fix-Bug", false},
		{"has special chars", "fix_bug", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidName(tt.input)
			if result != tt.valid {
				t.Errorf("isValidName(%q) = %v, expected %v", tt.input, result, tt.valid)
			}
		})
	}
}

func TestEnsureUnique(t *testing.T) {
	existing := []string{"fix-bug", "add-feature", "update-docs"}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unique name",
			input:    "new-feature",
			expected: "new-feature",
		},
		{
			name:     "duplicate gets suffix",
			input:    "fix-bug",
			expected: "fix-bug-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureUnique(tt.input, existing)
			if result != tt.expected {
				t.Errorf("EnsureUnique(%q, existing) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEnsureUniqueMultipleDuplicates(t *testing.T) {
	// Test that multiple duplicates get incrementing suffixes
	existing := []string{"fix-bug", "fix-bug-2", "fix-bug-3"}

	result := EnsureUnique("fix-bug", existing)
	expected := "fix-bug-4"

	if result != expected {
		t.Errorf("EnsureUnique(fix-bug) = %q, expected %q", result, expected)
	}
}

func TestIntToString(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{99, "99"},
		{100, "100"},
		{999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := intToString(tt.input)
			if result != tt.expected {
				t.Errorf("intToString(%d) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFromTaskWordLimit(t *testing.T) {
	// Task with more than 4 keywords should be limited to 4
	task := "Fix the critical bug in user authentication system database connection"
	result := FromTask(task)

	// Count words in result
	words := strings.Split(result, "-")
	if len(words) > 4 {
		t.Errorf("FromTask() produced more than 4 words: %q (%d words)", result, len(words))
	}
}
