package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dlorenc/multiclaude/internal/format"
)

// SelectableItem represents an item that can be selected from a list
type SelectableItem struct {
	Name        string
	Description string
}

// SelectFromList displays a list of items and prompts the user to select one.
// Returns the selected item name, or empty string if cancelled.
// If there's only one item, it's auto-selected without prompting.
func SelectFromList(prompt string, items []SelectableItem) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items available")
	}

	// Auto-select if only one item
	if len(items) == 1 {
		fmt.Printf("Auto-selecting: %s\n", items[0].Name)
		return items[0].Name, nil
	}

	// Display prompt
	format.Header("%s", prompt)
	fmt.Println()

	// Calculate widths for alignment
	maxNumWidth := len(fmt.Sprintf("%d", len(items)))
	maxNameWidth := 0
	for _, item := range items {
		if len(item.Name) > maxNameWidth {
			maxNameWidth = len(item.Name)
		}
	}

	// Display numbered list
	for i, item := range items {
		numStr := fmt.Sprintf("%*d", maxNumWidth, i+1)
		if item.Description != "" {
			format.Cyan.Printf("  [%s]", numStr)
			fmt.Printf("  %-*s  ", maxNameWidth, item.Name)
			format.Dim.Printf("%s\n", item.Description)
		} else {
			format.Cyan.Printf("  [%s]", numStr)
			fmt.Printf("  %s\n", item.Name)
		}
	}

	fmt.Println()
	fmt.Print("Enter number (or press Enter to cancel): ")

	// Read input
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)

	// Cancel on empty input
	if input == "" {
		return "", nil
	}

	// Parse number
	num, err := strconv.Atoi(input)
	if err != nil {
		return "", fmt.Errorf("invalid selection: %q is not a number", input)
	}

	// Validate range
	if num < 1 || num > len(items) {
		return "", fmt.Errorf("invalid selection: %d is out of range (1-%d)", num, len(items))
	}

	return items[num-1].Name, nil
}

// SelectFromListWithDefault is like SelectFromList but returns the default value
// when selection is cancelled instead of returning empty string.
func SelectFromListWithDefault(prompt string, items []SelectableItem, defaultValue string) (string, error) {
	selected, err := SelectFromList(prompt, items)
	if err != nil {
		return "", err
	}
	if selected == "" {
		return defaultValue, nil
	}
	return selected, nil
}

// agentsToSelectableItems converts a list of agents to selectable items,
// filtering by the specified types. If types is empty, all agents are included.
func agentsToSelectableItems(agents []interface{}, types []string) []SelectableItem {
	var items []SelectableItem
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	for _, agent := range agents {
		if agentMap, ok := agent.(map[string]interface{}); ok {
			agentType, _ := agentMap["type"].(string)
			name, _ := agentMap["name"].(string)

			// Filter by type if specified
			if len(typeSet) > 0 && !typeSet[agentType] {
				continue
			}

			// Build description from available fields
			var desc string
			if task, ok := agentMap["task"].(string); ok && task != "" {
				desc = format.Truncate(task, 50)
			} else if status, ok := agentMap["status"].(string); ok {
				desc = status
			}

			items = append(items, SelectableItem{
				Name:        name,
				Description: desc,
			})
		}
	}
	return items
}

// reposToSelectableItems converts a list of repos to selectable items.
func reposToSelectableItems(repos []interface{}) []SelectableItem {
	var items []SelectableItem
	for _, repo := range repos {
		if repoMap, ok := repo.(map[string]interface{}); ok {
			name, _ := repoMap["name"].(string)

			// Build description from agent count
			var desc string
			if totalAgents, ok := repoMap["total_agents"].(float64); ok && totalAgents > 0 {
				desc = fmt.Sprintf("%d agents", int(totalAgents))
			}

			items = append(items, SelectableItem{
				Name:        name,
				Description: desc,
			})
		}
	}
	return items
}
