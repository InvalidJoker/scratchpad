package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/ui"
)

// detailWidth is how much of the screen the detail pane takes, and minSplit is
// the width below which it is dropped entirely rather than squeezed into
// something unreadable.
const (
	detailWidth = 38
	minSplit    = 88
	// chrome is the rows the header, footer and their spacing occupy.
	chrome = 7
)

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Scratchpad"
	return v
}

func (m Model) render() string {
	if m.width == 0 {
		// The first frame arrives before the terminal has reported its size.
		return ""
	}
	return strings.Join(append(append([]string{m.header()}, m.body()...), m.footer()), "\n")
}

// body returns exactly bodyHeight lines, always.
//
// The fixed shape is the point: a frame that shrinks when the selected project
// has fewer details than the last one leaves the difference on screen, and the
// leftovers read as real content.
func (m Model) body() []string {
	height := m.bodyHeight()

	switch {
	case m.mode == modeHelp:
		return resize(lines(ui.Panel.Render(m.help.FullHelpView(m.keys.FullHelp()))), height)
	case m.mode == modeConfirm:
		return resize(lines(m.question()), height)
	case !m.loaded:
		return resize([]string{"", ui.Muted.Render("Reading your scratch directory…")}, height)
	case len(m.rows) == 0:
		return resize(append([]string{""}, lines(m.empty())...), height)
	case len(m.visible) == 0:
		return resize([]string{"", ui.Muted.Render(fmt.Sprintf("Nothing matches %q.", m.query))}, height)
	}

	listWidth := m.listWidth()
	left := resize(m.list(listWidth), height)
	if m.width < minSplit {
		return left
	}

	right := resize(m.detail(), height)
	rule := ui.Rule.Render("│")
	for i := range left {
		left[i] = fit(left[i], listWidth) + rule + " " + right[i]
	}
	return left
}

func (m Model) listWidth() int {
	if m.width < minSplit {
		return m.width
	}
	return m.width - detailWidth - 2
}

// resize forces a block to an exact number of lines.
func resize(block []string, height int) []string {
	if len(block) > height {
		return block[:height]
	}
	for len(block) < height {
		block = append(block, "")
	}
	return block
}

func lines(s string) []string { return strings.Split(s, "\n") }

func (m Model) header() string {
	title := ui.Title.Render("Scratchpad")
	summary := ui.Muted.Render(m.summary())

	pad := m.width - lipgloss.Width(title) - lipgloss.Width(summary)
	if pad < 1 {
		return "\n" + clip(title, m.width) + "\n"
	}
	return "\n" + title + strings.Repeat(" ", pad) + summary + "\n"
}

// summary is the same tally `sp list` prints, in the same fixed order so it
// does not shuffle as projects change state.
func (m Model) summary() string {
	counts := map[project.Status]int{}
	for _, i := range m.visible {
		counts[m.store.StatusOf(m.rows[i].project)]++
	}

	parts := []string{plural(len(m.visible), "project")}
	for _, s := range []project.Status{
		project.StatusActive, project.StatusStale, project.StatusExpired,
	} {
		if n := counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	return strings.Join(parts, " · ")
}

// list renders the project table, scrolled to keep the cursor on screen.
func (m Model) list(width int) []string {
	height := m.rowsVisible()
	first := m.scrollOffset(height)

	out := []string{ui.Header.Render(m.columns("NAME", "LAST USED", "SIZE", "STATUS", width))}
	for i := first; i < first+height && i < len(m.visible); i++ {
		r := m.rows[m.visible[i]]
		status := m.store.StatusOf(r.project)
		line := m.columns(
			r.project.Name,
			ui.RelativeTime(r.project.LastActivity(), m.store.Now()),
			ui.Bytes(r.signals.Size),
			ui.StatusLabel(status),
			width,
		)
		if i == m.cursor {
			out = append(out, ui.Selected.Render(line))
			continue
		}
		out = append(out, statusColour(status).Render(line))
	}
	return out
}

// columns lays the listing out by hand rather than through ui.Table: the table
// sizes itself to its content, and a pane that resizes under the cursor as you
// scroll is worse than one with fixed columns.
func (m Model) columns(name, used, size, status string, width int) string {
	statusW, sizeW, usedW := 11, 9, 14
	// One leading space, three between the columns, one trailing so a full
	// row never runs into the pane divider.
	nameW := width - statusW - sizeW - usedW - 5
	if nameW < 8 {
		nameW = 8
	}
	return fmt.Sprintf(" %s %s %s %s ",
		fit(name, nameW), fit(used, usedW), fit(size, sizeW), fit(status, statusW))
}

func (m Model) detail() []string {
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	p := r.project
	now := m.store.Now()

	out := []string{ui.Bold.Render(p.Name)}
	if p.Description != "" {
		out = append(out, ui.Muted.Render(p.Description))
	}
	out = append(out,
		"",
		field("status", ui.StatusBadge(m.store.StatusOf(p))),
		field("where", ui.Tildify(p.Dir())),
		field("last used", ui.RelativeTime(p.LastActivity(), now)),
		field("age", ui.Duration(p.Age(now))),
		field("opened", plural(p.OpenCount, "time")),
		field("size", ui.Bytes(r.signals.Size)),
	)
	if r.signals.Deps > 0 {
		out = append(out, field("of which", ui.Bytes(r.signals.Deps)+" deps"))
	}
	if p.ExpiresAt.IsZero() {
		out = append(out, field("expires", "never"))
	} else {
		out = append(out, field("expires", ui.RelativeFuture(p.ExpiresAt, now)))
	}
	if len(p.Tags) > 0 {
		out = append(out, field("tags", strings.Join(p.Tags, ", ")))
	}

	out = append(out, "", ui.Header.Render("GIT"))
	out = append(out, m.gitLines(p.Dir())...)

	if p.Note != "" {
		out = append(out, "", ui.Header.Render("NOTE"), ui.Muted.Render(p.Note))
	}

	// Every line is squared off to the pane width here rather than at each
	// call site, so a long path or note cannot push the divider sideways.
	for i, l := range out {
		out[i] = fit(l, detailWidth)
	}
	return out
}

func (m Model) gitLines(dir string) []string {
	status, loaded := m.git[dir]
	if !loaded {
		return []string{ui.Muted.Render("reading…")}
	}
	if status == nil {
		return []string{ui.Muted.Render("not a repository")}
	}

	branch := status.Branch
	if branch == "" {
		branch = "no commits yet"
	}
	out := []string{
		field("branch", branch),
		field("commits", strconv.Itoa(status.Commits)),
	}
	if !status.LastCommit.IsZero() {
		out = append(out, field("last", ui.RelativeTime(status.LastCommit, m.store.Now())))
	}
	if risk := status.Summary(); risk != "" {
		out = append(out, field("at risk", ui.Warning.Render(risk)))
	} else {
		out = append(out, field("changes", ui.Success.Render("clean")))
	}
	return out
}

// question renders a confirmation, with the facts above the answers so the
// decision is made with them on screen.
func (m Model) question() string {
	var b strings.Builder
	b.WriteString(ui.Bold.Render(m.ask.title))
	if m.ask.risk != "" {
		b.WriteString("\n\n" + ui.Warn("%s", ui.Warning.Render(m.ask.risk)))
	}
	for _, d := range m.ask.detail {
		b.WriteString("\n" + ui.Muted.Render("  "+d))
	}

	yes, no := "  "+m.ask.confirm+"  ", "  "+m.ask.cancel+"  "
	if m.ask.def {
		yes, no = ui.Selected.Render(yes), ui.Muted.Render(no)
	} else {
		yes, no = ui.Muted.Render(yes), ui.Selected.Render(no)
	}
	b.WriteString("\n\n" + yes + "  " + no)
	b.WriteString("\n" + ui.Muted.Render("y / n · enter takes the highlighted answer · esc cancels"))

	frame := ui.Panel
	if m.ask.risk != "" {
		frame = ui.Alarm
	}
	return frame.Render(b.String())
}

func (m Model) empty() string {
	return strings.Join([]string{
		ui.Muted.Render("No scratch projects yet."),
		"",
		"Press " + ui.Accent.Render("N") + " to start one.",
	}, "\n")
}

func (m Model) footer() string {
	line := m.help.ShortHelpView(m.keys.ShortHelp())
	switch {
	case m.mode == modeSearch, m.mode == modeNew, m.mode == modeRename:
		line = m.input.View() + "  " + ui.Muted.Render("enter confirms · esc cancels")
	case m.mode == modeConfirm:
		// The panel states its own keys; repeating the list's would only
		// suggest they still work.
		line = ""
	case m.mode == modeHelp:
		line = ui.Muted.Render("any key returns to the list")
	case m.failure != "":
		line = ui.Danger.Render("✗ " + m.failure)
	case m.status != "":
		line = ui.Check("%s", m.status)
	}
	// Clipped rather than trusted: the help view sizes itself from a width the
	// model is only told about on resize, and one long line would wrap and
	// push the whole frame up by a row.
	return "\n" + ui.Rule.Render(strings.Repeat("─", m.width)) + "\n" + clip(line, m.width)
}

// rowsVisible is how many project rows fit between the header and the footer.
func (m Model) rowsVisible() int {
	n := m.height - chrome
	if n < 1 {
		return 1
	}
	return n
}

// scrollOffset keeps the cursor on screen without moving the window more than
// it has to.
func (m Model) scrollOffset(height int) int {
	if m.cursor < height {
		return 0
	}
	first := m.cursor - height + 1
	if max := len(m.visible) - height; first > max && max > 0 {
		first = max
	}
	return first
}

// bodyHeight is the block between the header and the footer: the listing plus
// its column header.
func (m Model) bodyHeight() int { return m.rowsVisible() + 1 }

func field(label, value string) string {
	return ui.Muted.Render(fmt.Sprintf("%-10s", label)) + value
}

func statusColour(s project.Status) lipgloss.Style {
	switch s {
	case project.StatusStale:
		return ui.Warning
	case project.StatusExpired:
		return ui.Danger
	default:
		return lipgloss.NewStyle()
	}
}

// fit pads or truncates a cell to an exact display width, measured the way the
// terminal will draw it rather than by counting bytes.
func fit(s string, width int) string {
	if width < 1 {
		return ""
	}
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return clip(s, width)
}

// clip truncates to a display width without padding, marking the cut.
func clip(s string, width int) string {
	if width < 1 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(s) + "…"
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
