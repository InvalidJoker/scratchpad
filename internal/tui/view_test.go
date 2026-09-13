package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
)

var testNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

// testModel builds a dashboard over a temp scratch directory holding the given
// projects, sized as if it were on a normal terminal.
func testModel(t *testing.T, names ...string) Model {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		ScratchDir:        filepath.Join(root, "scratch"),
		ProjectsDir:       filepath.Join(root, "projects"),
		TrashDir:          filepath.Join(root, "scratch", ".trash"),
		DefaultExpiration: config.Duration(30 * 24 * time.Hour),
		StaleAfter:        config.Duration(14 * 24 * time.Hour),
	}
	st := store.New(cfg)
	st.SetClock(func() time.Time { return testNow })
	if err := st.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	m := newModel(st, cfg)
	m.width, m.height = 110, 26
	m.loaded = true

	for i, name := range names {
		p, err := st.Create(store.CreateOptions{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		// Enough variety that the detail pane changes height between rows,
		// which is what the fixed-frame rule exists to survive.
		if i%2 == 0 {
			p.Description = "a description that is quite long, long enough to need truncating"
			p.Tags = []string{"experiment", "ui"}
			p.Note = "a note"
		}
		m.rows = append(m.rows, row{project: p})
	}
	m.refilter()
	return m
}

// widths returns the display width of every line in a frame.
func widths(frame string) []int {
	out := []int{}
	for _, l := range strings.Split(frame, "\n") {
		out = append(out, lipgloss.Width(l))
	}
	return out
}

func TestFrameIsAlwaysTheSameShape(t *testing.T) {
	m := testModel(t, "alpha", "beta", "gamma")
	m.git[m.rows[0].project.Dir()] = &gitx.Status{Branch: "main", Commits: 3, Dirty: []string{"?? draft.txt"}}

	// A frame that changes height or overflows the terminal leaves the
	// difference on screen, and the leftovers read as real content.
	var height int
	for _, step := range []struct {
		label string
		apply func(m *Model)
	}{
		{"first row", func(m *Model) { m.cursor = 0 }},
		{"second row, no git yet", func(m *Model) { m.cursor = 1 }},
		{"third row", func(m *Model) { m.cursor = 2 }},
		{"searching", func(m *Model) { m.mode, m.query = modeSearch, "al"; m.refilter() }},
		{"no matches", func(m *Model) { m.mode, m.query = modeSearch, "zzz"; m.refilter() }},
		{"back to the list", func(m *Model) { m.mode, m.query = modeList, ""; m.refilter() }},
		{"help", func(m *Model) { m.mode = modeHelp }},
		{"confirming", func(m *Model) {
			m.mode = modeConfirm
			m.ask = m.trashQuestion(m.rows[0].project)
		}},
		{"an error", func(m *Model) { m.mode, m.failure = modeList, "something went wrong" }},
	} {
		step.apply(&m)
		frame := m.render()
		got := widths(frame)

		if height == 0 {
			height = len(got)
		}
		if len(got) != height {
			t.Errorf("%s: frame is %d lines, want %d", step.label, len(got), height)
		}
		for i, w := range got {
			if w > m.width {
				t.Errorf("%s: line %d is %d wide, want at most %d:\n%q",
					step.label, i, w, m.width, strings.Split(frame, "\n")[i])
			}
		}
	}
}

func TestFrameFitsANarrowTerminal(t *testing.T) {
	m := testModel(t, "alpha", "beta")
	m.width, m.height = 60, 20

	// Below minSplit the detail pane is dropped rather than squeezed, but the
	// listing still has to fit.
	for _, l := range strings.Split(m.render(), "\n") {
		if w := lipgloss.Width(l); w > m.width {
			t.Errorf("line is %d wide, want at most %d: %q", w, m.width, l)
		}
	}
}

func TestListScrollsToKeepTheCursorVisible(t *testing.T) {
	names := make([]string, 40)
	for i := range names {
		names[i] = "project-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	m := testModel(t, names...)
	m.cursor = len(m.visible) - 1

	frame := m.render()
	if !strings.Contains(frame, m.rows[m.visible[m.cursor]].project.Name) {
		t.Error("the selected project is off screen, want the list scrolled to it")
	}
}

func TestTrashQuestionFollowsTheGitStatus(t *testing.T) {
	m := testModel(t, "alpha")
	p := m.rows[0].project

	m.git[p.Dir()] = &gitx.Status{Branch: "main", Commits: 1, HasRemote: true, HasUpstream: true}
	if q := m.trashQuestion(p); !q.def || q.risk != "" {
		t.Errorf("clean project: def = %v, risk = %q, want true and no risk", q.def, q.risk)
	}

	// Uncommitted work must move the highlight onto the safe answer, so that a
	// reflexive Enter cannot bin it.
	m.git[p.Dir()] = &gitx.Status{Branch: "main", Commits: 1, Dirty: []string{"M main.go"}}
	q := m.trashQuestion(p)
	if q.def {
		t.Error("a project with uncommitted work defaults to trashing it, want the safe answer")
	}
	if q.risk == "" {
		t.Error("the risk was not named in the question")
	}
	if len(q.detail) == 0 || !strings.Contains(strings.Join(q.detail, " "), "main.go") {
		t.Errorf("detail = %v, want the file at risk listed", q.detail)
	}
}

func TestSearchFiltersOnMoreThanTheName(t *testing.T) {
	m := testModel(t, "alpha", "beta")
	m.rows[1].project.Tags = []string{"rust"}

	m.query = "rust"
	m.refilter()
	if len(m.visible) != 1 || m.rows[m.visible[0]].project.Name != "beta" {
		t.Errorf("visible = %v, want just beta", m.visible)
	}

	m.query = ""
	m.refilter()
	if len(m.visible) != 2 {
		t.Errorf("visible = %v, want everything back", m.visible)
	}
}

func TestFitMeasuresDisplayWidth(t *testing.T) {
	tests := []struct {
		in    string
		width int
	}{
		{"short", 12},
		{"exactly-12ch", 12},
		{"far too long to fit in here", 12},
		{project.StatusActive.Icon() + " Active", 12},
	}
	for _, tc := range tests {
		if got := lipgloss.Width(fit(tc.in, tc.width)); got != tc.width {
			t.Errorf("fit(%q, %d) is %d wide, want %d", tc.in, tc.width, got, tc.width)
		}
	}
}
