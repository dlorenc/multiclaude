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
