// Package tui is the dashboard `sp` opens when you run it with no arguments.
//
// It drives the same store the commands do and asks the same safety questions
// before anything destructive, so nothing here can do something `sp trash`
// would have refused.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/InvalidJoker/scratchpad/internal/activity"
	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
)

// Run opens the dashboard and blocks until the user quits.
func Run(ctx context.Context, st *store.Store, cfg *config.Config) error {
	p := tea.NewProgram(newModel(st, cfg), tea.WithContext(ctx))
	_, err := p.Run()
	// Quitting with Ctrl-C is how people leave a TUI, not a failure worth an
	// exit code.
	if errors.Is(err, tea.ErrInterrupted) {
		return nil
	}
	return err
}

// mode is what the dashboard is currently asking of the user. Only one thing
// is ever in flight, so it is a single value rather than a set of flags.
type mode int

const (
	modeList mode = iota
	modeSearch
	modeNew
	modeRename
	modeConfirm
	modeHelp
)

// row is one project plus everything already known about it.
type row struct {
	project *project.Project
	signals activity.Signals
}

// pending is a destructive action waiting on a yes or no.
type pending struct {
	title string
	// risk is the work that would be put at stake, empty when there is none.
	risk    string
	detail  []string
	confirm string
	cancel  string
	// def is the highlighted answer. It follows the CLI's rule: anything with
	// work at risk starts on the safe one, so a reflexive Enter is never the
	// destructive answer.
	def bool
	run tea.Cmd
}

type Model struct {
	store *store.Store
	cfg   *config.Config

	keys  keyMap
	help  help.Model
	input textinput.Model

	rows    []row
	visible []int
	cursor  int
	query   string

	// git is filled in lazily, one project at a time, and keyed by directory
	// so a rename cannot leave a stale entry attached to the wrong project.
	git map[string]*gitx.Status

	mode mode
	ask  pending
	// awaiting is the directory whose git status a pending trash question is
	// blocked on.
	awaiting string
	status   string
	failure  string

	width, height int
	loaded        bool
}

func newModel(st *store.Store, cfg *config.Config) Model {
	in := textinput.New()
	in.Prompt = "› "
	in.CharLimit = 100

	return Model{
		store: st,
		cfg:   cfg,
		keys:  defaultKeys(),
		help:  help.New(),
		input: in,
		git:   map[string]*gitx.Status{},
	}
}

func (m Model) Init() tea.Cmd {
	return m.load("")
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		return m, nil

	case loadedMsg:
		return m.applyLoad(msg)

	case gitMsg:
		m.git[msg.dir] = msg.status
		return m.afterGit(msg.dir)

	case doneMsg:
		if msg.err != nil {
			m.failure, m.status = msg.err.Error(), ""
			// Reload anyway: a half-finished move leaves the listing wrong,
			// and showing the truth matters more than showing the error alone.
			return m, m.load("")
		}
		m.failure, m.status = "", msg.text
		return m, m.load(msg.focus)

	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) applyLoad(msg loadedMsg) (tea.Model, tea.Cmd) {
	m.loaded = true
	if msg.err != nil {
		m.failure = msg.err.Error()
		return m, nil
	}

	m.rows = msg.rows
	m.refilter()
	if msg.focus != "" {
		m.focusOn(msg.focus)
	}
	m.clampCursor()
	return m, m.fetchGit()
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSearch:
		return m.onSearchKey(msg)
	case modeNew, modeRename:
		return m.onInputKey(msg)
	case modeConfirm:
		return m.onConfirmKey(msg)
	case modeHelp:
		// Any key leaves help; it is a reference card, not a place to be.
		m.mode = modeList
		return m, nil
	}
	return m.onListKey(msg)
}

func (m Model) onListKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// A message from the last action is stale the moment the user acts again.
	clear := func() { m.status, m.failure = "", "" }

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
		return m, nil

	case key.Matches(msg, m.keys.Up):
		clear()
		return m.moveCursor(-1)

	case key.Matches(msg, m.keys.Down):
		clear()
		return m.moveCursor(1)

	case key.Matches(msg, m.keys.Top):
		clear()
		m.cursor = 0
		return m, m.fetchGit()

	case key.Matches(msg, m.keys.Bottom):
		clear()
		m.cursor = len(m.visible) - 1
		m.clampCursor()
		return m, m.fetchGit()

	case key.Matches(msg, m.keys.Search):
		clear()
		m.mode = modeSearch
		m.input.Prompt = "search › "
		m.input.SetValue(m.query)
		m.input.CursorEnd()
		return m, m.input.Focus()

	case key.Matches(msg, m.keys.Clear):
		clear()
		if m.query == "" {
			return m, nil
		}
		m.query = ""
		m.refilter()
		m.clampCursor()
		return m, m.fetchGit()

	case key.Matches(msg, m.keys.New):
		clear()
		m.mode = modeNew
		m.input.Prompt = "name › "
		m.input.SetValue("")
		return m, m.input.Focus()
	}

	// Everything below acts on a project, so there has to be one.
	p, ok := m.selected()
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Open):
		clear()
		return m, m.openProject(p)

	case key.Matches(msg, m.keys.Rename):
		clear()
		m.mode = modeRename
		m.input.Prompt = "rename › "
		m.input.SetValue(p.Name)
		m.input.CursorEnd()
		return m, m.input.Focus()

	case key.Matches(msg, m.keys.Keep):
		clear()
		m.mode = modeConfirm
		m.ask = m.keepQuestion(p)
		return m, nil

	case key.Matches(msg, m.keys.Trash):
		clear()
		// The question cannot be asked until git has answered: defaulting to
		// "yes" on a project whose status has not loaded yet would be exactly
		// the assumed consent the safety rules forbid.
		if _, known := m.git[p.Dir()]; !known {
			m.awaiting = p.Dir()
			m.status = "Checking " + p.Name + " for work at risk…"
			return m, readGit(p.Dir())
		}
		m.mode = modeConfirm
		m.ask = m.trashQuestion(p)
		return m, nil
	}
	return m, nil
}

// afterGit opens the trash question that was waiting on this status.
func (m Model) afterGit(dir string) (tea.Model, tea.Cmd) {
	if m.awaiting != dir {
		return m, nil
	}
	m.awaiting = ""
	m.status = ""

	// The selection may have moved while git was running; asking about a
	// project the user is no longer looking at would be worse than silence.
	p, ok := m.selected()
	if !ok || p.Dir() != dir {
		return m, nil
	}
	m.mode = modeConfirm
	m.ask = m.trashQuestion(p)
	return m, nil
}

func (m Model) onSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		m.query = ""
		m.refilter()
		m.clampCursor()
		return m, m.fetchGit()
	case "enter":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	// Filtering as you type is the point: the listing is small enough that
	// there is nothing to gain by waiting for Enter.
	m.query = m.input.Value()
	m.refilter()
	m.clampCursor()
	return m, tea.Batch(cmd, m.fetchGit())
}

func (m Model) onInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.input.Value())
		mode := m.mode
		m.mode = modeList
		m.input.Blur()

		if err := project.ValidateName(name); err != nil {
			m.failure = err.Error()
			return m, nil
		}
		if mode == modeNew {
			return m, m.createProject(name)
		}
		p, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m, m.renameProject(p, name)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) onConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	answer := m.ask.def
	switch msg.String() {
	case "y", "Y":
		answer = true
	case "n", "N", "esc", "q":
		answer = false
	case "enter":
		// Falls through on the default, which is the safe answer whenever
		// there is work at risk.
	case "left", "right", "tab":
		m.ask.def = !m.ask.def
		return m, nil
	default:
		return m, nil
	}

	run := m.ask.run
	m.mode, m.ask = modeList, pending{}
	if !answer {
		m.status = "Cancelled. Nothing was touched."
		return m, nil
	}
	return m, run
}

func (m Model) keepQuestion(p *project.Project) pending {
	return pending{
		title:   fmt.Sprintf("Keep %s?", p.Name),
		detail:  []string{"It moves to " + ui.Tildify(m.store.PathFor(store.Kept, p.Name)) + " and stops expiring."},
		confirm: "Keep it",
		cancel:  "Cancel",
		def:     true,
		run:     m.keepProject(p),
	}
}

// trashQuestion asks the same question `sp trash` asks, from the same facts:
// the git status decides what is shown and which answer is highlighted.
func (m Model) trashQuestion(p *project.Project) pending {
	status := m.git[p.Dir()]

	q := pending{
		title:   fmt.Sprintf("Move %s to the trash?", p.Name),
		risk:    status.Summary(),
		confirm: "Trash it",
		cancel:  "Keep it",
		def:     status.Clean(),
		run:     m.trashProject(p),
	}
	dirty := dirtyOf(status)
	q.detail = append(q.detail, firstN(dirty, 5)...)
	if extra := len(dirty) - 5; extra > 0 {
		q.detail = append(q.detail, fmt.Sprintf("… and %d more", extra))
	}
	if q.risk == "" {
		q.detail = append(q.detail, "Nothing is at risk. It stays recoverable with `sp restore`.")
	}
	return q
}

func dirtyOf(s *gitx.Status) []string {
	if s == nil {
		return nil
	}
	return s.Dirty
}

func firstN(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

// moveCursor walks the listing and fetches git for whatever it lands on.
func (m Model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	if len(m.visible) == 0 {
		return m, nil
	}
	m.cursor += delta
	m.clampCursor()
	return m, m.fetchGit()
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) focusOn(name string) {
	for i, idx := range m.visible {
		if m.rows[idx].project.Name == name {
			m.cursor = i
			return
		}
	}
}

// refilter recomputes which rows the search leaves visible.
func (m *Model) refilter() {
	m.visible = m.visible[:0]
	q := strings.ToLower(strings.TrimSpace(m.query))
	for i, r := range m.rows {
		if q == "" || matchesQuery(r.project, q) {
			m.visible = append(m.visible, i)
		}
	}
}

func matchesQuery(p *project.Project, q string) bool {
	for _, field := range []string{p.Name, p.Description, p.Note} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	for _, t := range p.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

func (m Model) selectedRow() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return row{}, false
	}
	return m.rows[m.visible[m.cursor]], true
}

func (m Model) selected() (*project.Project, bool) {
	r, ok := m.selectedRow()
	if !ok {
		return nil, false
	}
	return r.project, true
}

// fetchGit reads the selected project's git status, unless it is already known.
func (m Model) fetchGit() tea.Cmd {
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	if _, known := m.git[r.project.Dir()]; known {
		return nil
	}
	return readGit(r.project.Dir())
}
