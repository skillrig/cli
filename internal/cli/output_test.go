package cli

import (
	"strings"
	"testing"
)

// TestWriteAlignedColumns_NeutralizesTabsInCells guards the PR #22 review bug: a
// tab embedded in a cell must not be read by tabwriter as a column separator
// (which would corrupt alignment). The renderer sanitizes \t/\n/\r to spaces, so
// a tabbed description stays a single cell.
func TestWriteAlignedColumns_NeutralizesTabsInCells(t *testing.T) {
	t.Parallel()

	var b strings.Builder

	rows := [][]string{
		{"name", "desc with\ttab"},
		{"longer-name", "second"},
	}

	if err := writeAlignedColumns(&b, rows); err != nil {
		t.Fatalf("writeAlignedColumns: %v", err)
	}

	out := b.String()

	// Exactly two rows render — the embedded tab did not split a cell into a new
	// column or inject a row.
	if lines := strings.Split(strings.TrimRight(out, "\n"), "\n"); len(lines) != 2 {
		t.Fatalf("want 2 rendered lines, got %d:\n%q", len(lines), out)
	}

	// The tab became a space inside the cell, so the description survives as one
	// contiguous token. If the tab had leaked into tabwriter as a separator, the
	// two halves would be split across padded columns and this substring (single
	// space) would not appear.
	if !strings.Contains(out, "desc with tab") {
		t.Errorf("embedded tab not sanitized to a space; got:\n%q", out)
	}
}
