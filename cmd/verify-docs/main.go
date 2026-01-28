// verify-docs verifies that extension documentation stays in sync with code.
//
// This tool checks:
// - State schema fields match documentation
// - Socket API commands match documentation
// - File paths in docs exist and are correct
//
// Usage:
//
//	go run cmd/verify-docs/main.go
//	go run cmd/verify-docs/main.go --fix  // Auto-update docs (future)
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
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

// verifyStateSchema checks that state structs/fields match the docs list.
func verifyStateSchema() Verification {
	v := Verification{Name: "State schema documentation"}

	codeStructs, err := parseStateStructsFromCode()
	if err != nil {
		v.Message = err.Error()
		return v
	}

	docStructs, err := parseStateStructsFromDocs()
	if err != nil {
		v.Message = err.Error()
		return v
	}

	missingStructs := diffKeys(codeStructs, docStructs)
	extraStructs := diffKeys(docStructs, codeStructs)

	var missingFields []string
	var extraFields []string

	for name, fields := range codeStructs {
		if *verbose {
			fmt.Printf("Verifying struct: %s\n", name)
		}
		docFields := docStructs[name]
		missingFields = append(missingFields, diffListPrefixed(fields, docFields, name)...)
		extraFields = append(extraFields, diffListPrefixed(docFields, fields, name)...)
	}

	if len(missingStructs) > 0 || len(extraStructs) > 0 || len(missingFields) > 0 || len(extraFields) > 0 {
		var parts []string
		if len(missingStructs) > 0 {
			parts = append(parts, fmt.Sprintf("missing structs: %s", strings.Join(missingStructs, ", ")))
		}
		if len(extraStructs) > 0 {
			parts = append(parts, fmt.Sprintf("undocumented structs removed from code: %s", strings.Join(extraStructs, ", ")))
		}
		if len(missingFields) > 0 {
			parts = append(parts, fmt.Sprintf("missing fields: %s", strings.Join(missingFields, ", ")))
		}
		if len(extraFields) > 0 {
			parts = append(parts, fmt.Sprintf("fields documented but not in code: %s", strings.Join(extraFields, ", ")))
		}
		v.Message = strings.Join(parts, "; ")
		return v
	}

	v.Passed = true
	return v
}

// verifySocketCommands checks that socket commands in code and docs are aligned.
func verifySocketCommands() Verification {
	v := Verification{Name: "Socket commands documentation"}

	codeCommands, err := parseSocketCommandsFromCode()
	if err != nil {
		v.Message = err.Error()
		return v
	}

	docCommands, err := parseSocketCommandsFromDocs()
	if err != nil {
		v.Message = err.Error()
		return v
	}

	if *verbose {
		fmt.Printf("Found %d commands in code, %d in docs\n", len(codeCommands), len(docCommands))
	}

	missing := diffList(codeCommands, docCommands)
	extra := diffList(docCommands, codeCommands)

	if len(missing) > 0 || len(extra) > 0 {
		var parts []string
		if len(missing) > 0 {
			parts = append(parts, fmt.Sprintf("missing commands: %s", strings.Join(missing, ", ")))
		}
		if len(extra) > 0 {
			parts = append(parts, fmt.Sprintf("commands documented but not in code: %s", strings.Join(extra, ", ")))
		}
		v.Message = strings.Join(parts, "; ")
		return v
	}

	v.Passed = true
	return v
}

// verifyFilePaths checks that file paths mentioned in docs exist.
func verifyFilePaths() Verification {
	v := Verification{Name: "File path references"}

	docFiles := []string{
		"docs/extending/STATE_FILE_INTEGRATION.md",
		"docs/extending/SOCKET_API.md",
	}

	// Use double-quoted string with explicit escapes for safety
	filePattern := regexp.MustCompile("((?:internal|pkg|cmd)/[^`]+\\.go)")

	missing := []string{}

	for _, docFile := range docFiles {
		if *verbose {
			fmt.Printf("Checking references in %s\n", docFile)
		}
		content, err := os.ReadFile(docFile)
		if err != nil {
			continue // Skip missing docs
		}

		matches := filePattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) > 1 {
				filePath := match[1]

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

// parseStateStructsFromCode extracts json field names for tracked structs.
func parseStateStructsFromCode() (map[string][]string, error) {
	tracked := map[string]struct{}{
		"State":            {},
		"Repository":       {},
		"Agent":            {},
		"TaskHistoryEntry": {},
		"MergeQueueConfig": {},
		"PRShepherdConfig": {},
		"ForkConfig":       {},
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "internal/state/state.go", nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse state.go: %w", err)
	}

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

		if _, wanted := tracked[typeSpec.Name.Name]; !wanted {
			return true
		}

		var fields []string
		for _, field := range structType.Fields.List {
			// skip embedded or unexported fields
			if len(field.Names) == 0 {
				continue
			}
			for _, name := range field.Names {
				if !ast.IsExported(name.Name) {
					continue
				}

				jsonName := jsonTag(field)
				if jsonName == "" {
					jsonName = toSnakeCase(name.Name)
				}
				if jsonName == "-" || jsonName == "" {
					continue
				}
				fields = append(fields, jsonName)
			}
		}

		structs[typeSpec.Name.Name] = uniqueSorted(fields)
		return true
	})

	return structs, nil
}

// parseStateStructsFromDocs reads state struct definitions from marker comments.
func parseStateStructsFromDocs() (map[string][]string, error) {
	docFile := "docs/extending/STATE_FILE_INTEGRATION.md"
	content, err := os.ReadFile(docFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", docFile, err)
	}

	pattern := regexp.MustCompile(`(?m)<!--\s*state-struct:\s*([A-Za-z0-9_]+)\s+([^>]+?)-->`)
	matches := pattern.FindAllStringSubmatch(string(content), -1)

	structs := make(map[string][]string)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		name := strings.TrimSpace(m[1])
		fields := uniqueSorted(strings.Fields(m[2]))
		structs[name] = fields
	}

	if len(structs) == 0 {
		return nil, fmt.Errorf("no state-struct markers found in %s", docFile)
	}

	return structs, nil
}

// parseSocketCommandsFromCode extracts socket commands from handleRequest.
func parseSocketCommandsFromCode() ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "internal/daemon/daemon.go", nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse daemon.go: %w", err)
	}

	var commands []string

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "handleRequest" {
			return true
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || !isReqCommand(sw.Tag) {
				return true
			}

			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					cmd, err := strconv.Unquote(lit.Value)
					if err == nil && cmd != "" {
						commands = append(commands, cmd)
					}
				}
			}
			return true
		})
		return false
	})

	return uniqueSorted(commands), nil
}

// parseSocketCommandsFromDocs reads socket command list from marker comments.
func parseSocketCommandsFromDocs() ([]string, error) {
	docFile := "docs/extending/SOCKET_API.md"
	content, err := os.ReadFile(docFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", docFile, err)
	}

	list := parseListFromComment(string(content), "socket-commands")
	if len(list) == 0 {
		return nil, fmt.Errorf("no socket-commands marker found in %s", docFile)
	}
	return list, nil
}

// parseListFromComment extracts a newline-delimited list from an HTML comment label.
func parseListFromComment(content, label string) []string {
	// Use fmt.Sprintf with double-quoted strings and explicit escapes
	// (?s) dot matches newline
	// <!-- \s* label : \s* (.*?) -->
	pattern := fmt.Sprintf("(?s)<!--\\s*%s:\\s*(.*?)-->", regexp.QuoteMeta(label))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(content)
	if len(matches) < 2 {
		return nil
	}

	var items []string
	for _, line := range strings.Split(matches[1], "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, line)
	}
	return uniqueSorted(items)
}

// isReqCommand checks if the switch tag is req.Command.
func isReqCommand(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return id.Name == "req" && sel.Sel != nil && sel.Sel.Name == "Command"
}

// jsonTag returns the json tag value if present.
func jsonTag(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	raw := strings.Trim(field.Tag.Value, "`")
	tag := reflect.StructTag(raw).Get("json")
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// diffList returns items in a but not in b.
func diffList(a, b []string) []string {
	setB := make(map[string]struct{}, len(b))
	for _, item := range b {
		setB[item] = struct{}{}
	}
	var diff []string
	for _, item := range a {
		if _, ok := setB[item]; !ok {
			diff = append(diff, item)
		}
	}
	return uniqueSorted(diff)
}

// diffListPrefixed returns items in a but not in b, prefixed with struct name.
func diffListPrefixed(a, b []string, prefix string) []string {
	items := diffList(a, b)
	for i, item := range items {
		items[i] = fmt.Sprintf("%s.%s", prefix, item)
	}
	return items
}

// diffKeys returns keys in a but not in b.
func diffKeys(a, b map[string][]string) []string {
	keysB := make(map[string]struct{}, len(b))
	for k := range b {
		keysB[k] = struct{}{}
	}
	var diff []string
	for k := range a {
		if _, ok := keysB[k]; !ok {
			diff = append(diff, k)
		}
	}
	return uniqueSorted(diff)
}

// toSnakeCase converts PascalCase to snake_case.
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

// uniqueSorted returns a sorted unique copy of the slice.
func uniqueSorted(items []string) []string {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}

	sort.Strings(out)
	return out
}
