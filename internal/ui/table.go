package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Table renders aligned columns. It measures with lipgloss.Width so styled
// cells and emoji line up, which plain len() gets wrong.
type Table struct {
	headers []string
	rows    [][]string
	// gap is the spacing between columns.
	gap int
}

func NewTable(headers ...string) *Table {
	return &Table{headers: headers, gap: 2}
}

func (t *Table) Row(cells ...string) *Table {
	t.rows = append(t.rows, cells)
	return t
}

func (t *Table) Len() int { return len(t.rows) }

func (t *Table) Render() string {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) && lipgloss.Width(cell) > widths[i] {
				widths[i] = lipgloss.Width(cell)
			}
		}
	}

	var b strings.Builder
	b.WriteString(t.line(headerStyleAll(t.headers), widths))
	for _, row := range t.rows {
		b.WriteString("\n")
		b.WriteString(t.line(row, widths))
	}
	return b.String()
}

// line pads every cell but the last, so trailing whitespace never ends up in
// piped output.
func (t *Table) line(cells []string, widths []int) string {
	var b strings.Builder
	for i, cell := range cells {
		if i == len(cells)-1 {
			b.WriteString(cell)
			break
		}
		b.WriteString(cell)
		b.WriteString(strings.Repeat(" ", widths[i]-lipgloss.Width(cell)+t.gap))
	}
	return strings.TrimRight(b.String(), " ")
}

func headerStyleAll(headers []string) []string {
	out := make([]string, len(headers))
	for i, h := range headers {
		out[i] = Header.Render(strings.ToUpper(h))
	}
	return out
}
