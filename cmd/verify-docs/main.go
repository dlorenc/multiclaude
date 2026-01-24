// verify-docs verifies that extension documentation stays in sync with code.
//
// This tool checks:
// - State schema fields match documentation
// - Event types match documentation
// - Socket API commands match documentation
// - File paths in docs exist and are correct
//
// Usage:
//
//	go run cmd/verify-docs/main.go
//	go run cmd/verify-docs/main.go --fix  # Auto-update docs (future)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

var (
	// fix is reserved for future auto-fix functionality
	_ = flag.Bool("fix", false, "Automatically fix documentation (not yet implemented)")

	verbose = flag.Bool("v", false, "Verbose output")
)

type Verification struct {
	Name    string
	Passed  bool
	Message string
}

func main() {
	flag.Parse()

	verifications := []Verification{
		verifyStateSchema(),
		verifyEventTypes(),
		verifySocketCommands(),
		verifyFilePaths(),
	}

	fmt.Println("Extension Documentation Verification")
	fmt.Println("====================================")
	fmt.Println()

	passed := 0
	failed := 0

	for _, v := range verifications {
		status := "✓"
		if !v.Passed {
			status = "✗"
			failed++
		} else {
			passed++
		}

		fmt.Printf("%s %s\n", status, v.Name)
		if v.Message != "" {
			fmt.Printf("  %s\n", v.Message)
		}
	}

	fmt.Println()
	fmt.Printf("Passed: %d, Failed: %d\n", passed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

// verifyStateSchema checks that state.State fields are documented
func verifyStateSchema() Verification {
	v := Verification{Name: "State schema documentation"}

	// Parse internal/state/state.go
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "internal/state/state.go", nil, parser.ParseComments)
	if err != nil {
		v.Message = fmt.Sprintf("Failed to parse state.go: %v", err)
		return v
	}

	// Find struct definitions
	structs := make(map[string][]string)
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		fields := []string{}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				// Skip private fields
				if !ast.IsExported(name.Name) {
					continue
				}
				fields = append(fields, name.Name)
			}
		}

		structs[typeSpec.Name.Name] = fields
		return true
	})

	// Check important structs are documented
	importantStructs := []string{
		"State",
		"Repository",
		"Agent",
		"TaskHistoryEntry",
		"MergeQueueConfig",
		"HookConfig",
	}

	docFile := "docs/extending/STATE_FILE_INTEGRATION.md"
	docContent, err := os.ReadFile(docFile)
	if err != nil {
		v.Message = fmt.Sprintf("Failed to read %s: %v", docFile, err)
		return v
	}

	missing := []string{}
	for _, structName := range importantStructs {
		if *verbose {
			fmt.Printf("  Checking struct: %s\n", structName)
		}

		// Check if struct name appears in docs
		if !strings.Contains(string(docContent), structName) {
			missing = append(missing, structName)
			continue
		}

		// Check if fields are documented (basic check)
		fields := structs[structName]
		for _, field := range fields {
			// Convert field name to JSON format (snake_case)
			jsonField := toSnakeCase(field)
			if !strings.Contains(string(docContent), fmt.Sprintf(`"%s"`, jsonField)) {
				missing = append(missing, fmt.Sprintf("%s.%s", structName, field))
			}
		}
	}

	if len(missing) > 0 {
		v.Message = fmt.Sprintf("Missing or incomplete: %s", strings.Join(missing, ", "))
		return v
	}

	v.Passed = true
	return v
}

// verifyEventTypes checks that all event types are documented
func verifyEventTypes() Verification {
	v := Verification{Name: "Event types documentation"}

	// Parse internal/events/events.go
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "internal/events/events.go", nil, parser.ParseComments)
	if err != nil {
		v.Message = fmt.Sprintf("Failed to parse events.go: %v", err)
		return v
	}

	// Find EventType constants
	eventTypes := []string{}
	ast.Inspect(node, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range valueSpec.Names {
				if strings.HasPrefix(name.Name, "Event") {
					eventTypes = append(eventTypes, name.Name)
				}
			}
		}

		return true
	})

	// Check if documented
	docFile := "docs/extending/EVENT_HOOKS.md"
	docContent, err := os.ReadFile(docFile)
	if err != nil {
		v.Message = fmt.Sprintf("Failed to read %s: %v", docFile, err)
		return v
	}

	missing := []string{}
	for _, eventType := range eventTypes {
		// Extract the actual event type string (e.g., EventAgentStarted -> agent_started)
		// This is a simplified check - we just check if the constant name appears
		if !strings.Contains(string(docContent), eventType) {
			missing = append(missing, eventType)
		}
	}

	if len(missing) > 0 {
		v.Message = fmt.Sprintf("Undocumented event types: %s", strings.Join(missing, ", "))
		return v
	}

	v.Passed = true
	return v
}

// verifySocketCommands checks that all socket commands are documented
func verifySocketCommands() Verification {
	v := Verification{Name: "Socket commands documentation"}

	// Find all case statements in handleRequest
	commands := []string{}

	file, err := os.Open("internal/daemon/daemon.go")
	if err != nil {
		v.Message = fmt.Sprintf("Failed to open daemon.go: %v", err)
		return v
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inSwitch := false
	casePattern := regexp.MustCompile(`case\s+"([^"]+)":`)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "switch req.Command") {
			inSwitch = true
			continue
		}

		if inSwitch {
			if strings.Contains(line, "default:") {
				break
			}

			matches := casePattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				commands = append(commands, matches[1])
			}
		}
	}

	// Check if documented
	docFile := "docs/extending/SOCKET_API.md"
	docContent, err := os.ReadFile(docFile)
	if err != nil {
		v.Message = fmt.Sprintf("Failed to read %s: %v", docFile, err)
		return v
	}

	missing := []string{}
	for _, cmd := range commands {
		// Check for command in documentation (should appear as "#### command_name")
		if !strings.Contains(string(docContent), cmd) {
			missing = append(missing, cmd)
		}
	}

	if len(missing) > 0 {
		v.Message = fmt.Sprintf("Undocumented commands: %s", strings.Join(missing, ", "))
		return v
	}

	v.Passed = true
	return v
}

// verifyFilePaths checks that file paths mentioned in docs exist
func verifyFilePaths() Verification {
	v := Verification{Name: "File path references"}

	// Check all extension docs
	docFiles := []string{
		"docs/EXTENSIBILITY.md",
		"docs/extending/STATE_FILE_INTEGRATION.md",
		"docs/extending/EVENT_HOOKS.md",
		"docs/extending/WEB_UI_DEVELOPMENT.md",
		"docs/extending/SOCKET_API.md",
	}

	// Patterns to find file references
	// Looking for things like:
	// - `internal/state/state.go`
	// - `cmd/multiclaude-web/main.go`
	// - `pkg/config/config.go`
	filePattern := regexp.MustCompile("`((?:internal|pkg|cmd)/[^`]+\\.go)`")

	missing := []string{}

	for _, docFile := range docFiles {
		content, err := os.ReadFile(docFile)
		if err != nil {
			continue // Skip missing docs
		}

		matches := filePattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) > 1 {
				filePath := match[1]

				// Check if file exists
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					missing = append(missing, fmt.Sprintf("%s (referenced in %s)", filePath, docFile))
				}
			}
		}
	}

	if len(missing) > 0 {
		v.Message = fmt.Sprintf("Missing files:\n    %s", strings.Join(missing, "\n    "))
		return v
	}

	v.Passed = true
	return v
}

// toSnakeCase converts PascalCase to snake_case
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}
