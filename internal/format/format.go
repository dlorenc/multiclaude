// Package format provides utilities for rich terminal output formatting.
package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Status represents the status of an agent or resource
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusRunning   Status = "running"
	StatusIdle      Status = "idle"
	StatusCompleted Status = "completed"
	StatusWarning   Status = "warning"
	StatusError     Status = "error"
	StatusPending   Status = "pending"
)

// Colors for different statuses
var (
	Green  = color.New(color.FgGreen)
	Yellow = color.New(color.FgYellow)
	Red    = color.New(color.FgRed)
	Cyan   = color.New(color.FgCyan)
	Bold   = color.New(color.Bold)
	Dim    = color.New(color.Faint)
)

// StatusColor returns the appropriate color for a status
func StatusColor(status Status) *color.Color {
	switch status {
	case StatusHealthy, StatusRunning, StatusCompleted:
		return Green
	case StatusWarning, StatusIdle, StatusPending:
		return Yellow
	case StatusError:
		return Red
	default:
		return color.New()
	}
}

// StatusIcon returns an icon for a status
func StatusIcon(status Status) string {
	switch status {
	case StatusHealthy, StatusCompleted:
		return "✓"
	case StatusRunning:
		return "●"
	case StatusIdle:
		return "○"
	case StatusWarning:
		return "⚠"
	case StatusError:
		return "✗"
	case StatusPending:
		return "◦"
	default:
		return "-"
	}
}

// ColoredStatus returns a colored status string with icon
func ColoredStatus(status Status) string {
	c := StatusColor(status)
	icon := StatusIcon(status)
	return c.Sprintf("%s %s", icon, status)
}

// Header prints a bold header line
func Header(format string, args ...interface{}) {
	Bold.Printf(format+"\n", args...)
}

// Dimmed prints dimmed/muted text
func Dimmed(format string, args ...interface{}) {
	Dim.Printf(format+"\n", args...)
}

// TimeAgo formats a time as a human-readable relative time
func TimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// Truncate truncates a string to maxLen, adding "..." if truncated
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Table provides a simple table formatter
type Table struct {
	headers []string
	rows    [][]string
	widths  []int
}

// NewTable creates a new table with the given headers
func NewTable(headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	return &Table{
		headers: headers,
		widths:  widths,
	}
}

// AddRow adds a row to the table
func (t *Table) AddRow(cells ...string) {
	// Pad or truncate to match header count
	row := make([]string, len(t.headers))
	for i := range row {
		if i < len(cells) {
			row[i] = cells[i]
		}
		if len(row[i]) > t.widths[i] {
			t.widths[i] = len(row[i])
		}
	}
	t.rows = append(t.rows, row)
}

// String returns the formatted table as a string
func (t *Table) String() string {
	var sb strings.Builder

	// Header
	for i, h := range t.headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(fmt.Sprintf("%-*s", t.widths[i], h))
	}
	sb.WriteString("\n")

	// Separator
	for i, w := range t.widths {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(strings.Repeat("-", w))
	}
	sb.WriteString("\n")

	// Rows
	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(fmt.Sprintf("%-*s", t.widths[i], cell))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ColoredTable is a table that supports colored cells
type ColoredTable struct {
	headers      []string
	rows         [][]ColoredCell
	widths       []int
	headerColors []*color.Color
}

// ColoredCell represents a cell with optional color
type ColoredCell struct {
	Text  string
	Color *color.Color
}

// Cell creates a plain cell
func Cell(text string) ColoredCell {
	return ColoredCell{Text: text}
}

// ColorCell creates a colored cell
func ColorCell(text string, c *color.Color) ColoredCell {
	return ColoredCell{Text: text, Color: c}
}

// NewColoredTable creates a new colored table
func NewColoredTable(headers ...string) *ColoredTable {
	widths := make([]int, len(headers))
	headerColors := make([]*color.Color, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
		headerColors[i] = Bold
	}
	return &ColoredTable{
		headers:      headers,
		widths:       widths,
		headerColors: headerColors,
	}
}

// AddRow adds a row to the colored table
func (t *ColoredTable) AddRow(cells ...ColoredCell) {
	row := make([]ColoredCell, len(t.headers))
	for i := range row {
		if i < len(cells) {
			row[i] = cells[i]
		}
		if len(row[i].Text) > t.widths[i] {
			t.widths[i] = len(row[i].Text)
		}
	}
	t.rows = append(t.rows, row)
}

// Print prints the colored table
func (t *ColoredTable) Print() {
	// Header
	for i, h := range t.headers {
		if i > 0 {
			fmt.Print("  ")
		}
		t.headerColors[i].Printf("%-*s", t.widths[i], h)
	}
	fmt.Println()

	// Separator
	Dim.Print(strings.Repeat("-", t.totalWidth()))
	fmt.Println()

	// Rows
	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Print("  ")
			}
			text := fmt.Sprintf("%-*s", t.widths[i], cell.Text)
			if cell.Color != nil {
				cell.Color.Print(text)
			} else {
				fmt.Print(text)
			}
		}
		fmt.Println()
	}
}

func (t *ColoredTable) totalWidth() int {
	total := 0
	for i, w := range t.widths {
		total += w
		if i > 0 {
			total += 2 // spacing
		}
	}
	return total
}

// MessageBadge formats a message count badge
func MessageBadge(pending, total int) string {
	if total == 0 {
		return Dim.Sprint("-")
	}
	if pending > 0 {
		return Yellow.Sprintf("%d/%d", pending, total)
	}
	return Dim.Sprintf("0/%d", total)
}
