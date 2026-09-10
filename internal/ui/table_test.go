package ui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/InvalidJoker/scratchpad/internal/ui"
)

// displayCol reports the terminal column where sub starts in line, counting
// display width rather than bytes so emoji and ANSI do not skew it.
func displayCol(t *testing.T, line, sub string) int {
	t.Helper()
	i := strings.Index(line, sub)
	if i < 0 {
		t.Fatalf("%q not found in %q", sub, line)
	}
	return lipgloss.Width(line[:i])
}

func TestTableAlignsColumns(t *testing.T) {
	table := ui.NewTable("name", "status")
	table.Row("a", "Active")
	table.Row("much-longer-name", "Stale")

	lines := strings.Split(table.Render(), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want header + 2 rows", len(lines))
	}

	if got, want := displayCol(t, lines[1], "Active"), displayCol(t, lines[2], "Stale"); got != want {
		t.Errorf("status column starts at %d and %d, want the same", got, want)
	}
}

func TestTableMeasuresEmojiByDisplayWidth(t *testing.T) {
	table := ui.NewTable("a", "b")
	table.Row("🟢", "x")
	table.Row("ab", "y")

	lines := strings.Split(table.Render(), "\n")
	if got, want := displayCol(t, lines[1], "x"), displayCol(t, lines[2], "y"); got != want {
		t.Errorf("column starts at %d and %d: emoji width measured wrong", got, want)
	}
}

func TestTableHasNoTrailingWhitespace(t *testing.T) {
	table := ui.NewTable("name", "status")
	table.Row("a", "Active")

	for _, line := range strings.Split(table.Render(), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing whitespace in %q: it shows up in piped output", line)
		}
	}
}
