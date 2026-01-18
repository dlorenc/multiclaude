package format

import (
	"strings"
	"testing"
	"time"
)

func TestStatusColor(t *testing.T) {
	tests := []struct {
		status   Status
		wantNil  bool
	}{
		{StatusHealthy, false},
		{StatusRunning, false},
		{StatusCompleted, false},
		{StatusWarning, false},
		{StatusIdle, false},
		{StatusPending, false},
		{StatusError, false},
		{Status("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := StatusColor(tt.status)
			if tt.wantNil && got != nil {
				t.Errorf("StatusColor(%v) = %v, want nil", tt.status, got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("StatusColor(%v) = nil, want non-nil", tt.status)
			}
		})
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusHealthy, "✓"},
		{StatusCompleted, "✓"},
		{StatusRunning, "●"},
		{StatusIdle, "○"},
		{StatusWarning, "⚠"},
		{StatusError, "✗"},
		{StatusPending, "◦"},
		{Status("unknown"), "-"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := StatusIcon(tt.status)
			if got != tt.want {
				t.Errorf("StatusIcon(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		time time.Time
		want string
	}{
		{"zero time", time.Time{}, "never"},
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"1 min ago", now.Add(-1 * time.Minute), "1 min ago"},
		{"5 mins ago", now.Add(-5 * time.Minute), "5 mins ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1 hour ago"},
		{"3 hours ago", now.Add(-3 * time.Hour), "3 hours ago"},
		{"1 day ago", now.Add(-24 * time.Hour), "1 day ago"},
		{"5 days ago", now.Add(-5 * 24 * time.Hour), "5 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimeAgo(tt.time)
			if got != tt.want {
				t.Errorf("TimeAgo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hello", 5, "hello"},
		{"hi", 2, "hi"},
		{"hello", 3, "hel"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTable(t *testing.T) {
	table := NewTable("Name", "Age", "City")
	table.AddRow("Alice", "30", "NYC")
	table.AddRow("Bob", "25", "LA")

	output := table.String()

	// Check headers are present
	if !strings.Contains(output, "Name") {
		t.Error("Table output missing 'Name' header")
	}
	if !strings.Contains(output, "Age") {
		t.Error("Table output missing 'Age' header")
	}
	if !strings.Contains(output, "City") {
		t.Error("Table output missing 'City' header")
	}

	// Check data is present
	if !strings.Contains(output, "Alice") {
		t.Error("Table output missing 'Alice'")
	}
	if !strings.Contains(output, "Bob") {
		t.Error("Table output missing 'Bob'")
	}

	// Check separator line
	if !strings.Contains(output, "---") {
		t.Error("Table output missing separator line")
	}
}

func TestColoredTable(t *testing.T) {
	table := NewColoredTable("Status", "Name")
	table.AddRow(
		ColorCell("running", Green),
		Cell("worker-1"),
	)
	table.AddRow(
		ColorCell("stopped", Red),
		Cell("worker-2"),
	)

	// Just ensure it doesn't panic
	// Actual color output is hard to test without capturing stdout
	if len(table.rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(table.rows))
	}
}

func TestCell(t *testing.T) {
	cell := Cell("hello")
	if cell.Text != "hello" {
		t.Errorf("Cell.Text = %q, want %q", cell.Text, "hello")
	}
	if cell.Color != nil {
		t.Errorf("Cell.Color should be nil for plain cell")
	}
}

func TestColorCell(t *testing.T) {
	cell := ColorCell("hello", Green)
	if cell.Text != "hello" {
		t.Errorf("ColorCell.Text = %q, want %q", cell.Text, "hello")
	}
	if cell.Color != Green {
		t.Errorf("ColorCell.Color should be Green")
	}
}

func TestColoredStatus(t *testing.T) {
	tests := []struct {
		status Status
	}{
		{StatusHealthy},
		{StatusRunning},
		{StatusError},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := ColoredStatus(tt.status)
			if got == "" {
				t.Errorf("ColoredStatus(%v) returned empty string", tt.status)
			}
			// Should contain the status icon and name
			icon := StatusIcon(tt.status)
			if !strings.Contains(got, icon) {
				t.Errorf("ColoredStatus(%v) = %q, should contain icon %q", tt.status, got, icon)
			}
		})
	}
}
