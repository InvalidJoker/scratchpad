package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/InvalidJoker/scratchpad/internal/activity"
	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/scaffold"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
)

// loadedMsg carries a full refresh of the listing.
type loadedMsg struct {
	rows []row
	err  error
	// focus is the project to put the cursor on afterwards, when an action
	// just created or renamed one.
	focus string
}

// gitMsg is the git status of one project, fetched lazily for the detail pane.
type gitMsg struct {
	dir    string
	status *gitx.Status
}

// doneMsg reports the outcome of an action.
type doneMsg struct {
	text  string
	err   error
	focus string
}

// load reads the scratch directory and folds in activity, off the UI thread.
// It is the only place the dashboard learns what exists.
func (m Model) load(focus string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		projects, err := m.store.Collect([]store.Location{store.Scratch})
		if err != nil {
			return loadedMsg{err: err}
		}

		dirs := make([]string, 0, len(projects))
		for _, p := range projects {
			dirs = append(dirs, p.Dir())
		}
		signals := activity.Collect(ctx, dirs, false)

		rows := make([]row, 0, len(projects))
		for _, p := range projects {
			if p.RecordActivity(signals[p.Dir()].Latest()) {
				_ = m.store.Save(p)
			}
			rows = append(rows, row{
				project: p,
				signals: signals[p.Dir()],
			})
		}

		// Sorting after the refresh, not before: the activity that decides the
		// order is only known once the scan has run.
		ordered := m.store.Match(collectProjects(rows), store.Filter{Sort: store.SortActivity})
		return loadedMsg{rows: reorder(rows, ordered), focus: focus}
	}
}

// readGit fetches one project's git status. It runs per selection rather than
// per listing: a git subprocess for every project on every refresh is exactly
// the cost the dashboard cannot afford.
func readGit(dir string) tea.Cmd {
	return func() tea.Msg {
		status, err := gitx.Read(context.Background(), dir)
		if err != nil {
			// A project whose git is unreadable still has a detail pane worth
			// showing; the block just says nothing.
			return gitMsg{dir: dir}
		}
		return gitMsg{dir: dir, status: status}
	}
}

func (m Model) createProject(name string) tea.Cmd {
	return func() tea.Msg {
		if err := m.store.EnsureDirs(); err != nil {
			return doneMsg{err: err}
		}
		p, err := m.store.Create(store.CreateOptions{Name: name})
		if err != nil {
			return doneMsg{err: err}
		}
		// Scaffolding is a nicety: the project exists and is usable either way,
		// so a failure is reported without being treated as a failed create.
		if err := scaffold.Apply(context.Background(), p, scaffold.Options{
			Git:    m.cfg.InitGit,
			Readme: true,
		}); err != nil {
			return doneMsg{text: fmt.Sprintf("Created %s — scaffolding incomplete: %v", p.Name, err), focus: p.Name}
		}
		return doneMsg{text: "Created " + p.Name, focus: p.Name}
	}
}

func (m Model) keepProject(p *project.Project) tea.Cmd {
	return func() tea.Msg {
		if err := m.store.Keep(p, ""); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{text: fmt.Sprintf("Kept %s — it now lives in %s", p.Name, ui.Tildify(p.Dir()))}
	}
}

func (m Model) trashProject(p *project.Project) tea.Cmd {
	return func() tea.Msg {
		name := p.Name
		if err := m.store.Trash(p); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{text: fmt.Sprintf("Trashed %s — restore it with `sp restore %s`", name, name)}
	}
}

func (m Model) renameProject(p *project.Project, name string) tea.Cmd {
	return func() tea.Msg {
		was := p.Name
		if err := m.store.Rename(p, name); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{text: fmt.Sprintf("Renamed %s to %s", was, p.Name), focus: p.Name}
	}
}

// openProject hands the terminal to the editor and takes it back afterwards.
// The visit is recorded before launching, so a long editing session does not
// delay the signal and a crash does not lose it.
func (m Model) openProject(p *project.Project) tea.Cmd {
	p.Touch(m.store.Now())
	if err := m.store.Save(p); err != nil {
		return func() tea.Msg { return doneMsg{err: err} }
	}

	editor := m.cfg.EditorCommand()
	if editor == "" {
		return func() tea.Msg {
			return doneMsg{err: fmt.Errorf("no editor configured: set $EDITOR, $VISUAL, or editor in your config")}
		}
	}

	fields := strings.Fields(editor)
	bin, args := fields[0], append(fields[1:], p.Dir())
	if _, err := exec.LookPath(bin); err != nil {
		return func() tea.Msg { return doneMsg{err: fmt.Errorf("editor %q not found on PATH", bin)} }
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = p.Dir()
	name := p.Name
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return doneMsg{err: fmt.Errorf("run %s: %w", bin, err)}
		}
		return doneMsg{text: "Closed " + name, focus: name}
	})
}

func collectProjects(rows []row) []*project.Project {
	out := make([]*project.Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.project)
	}
	return out
}

// reorder rearranges rows to follow the order the store sorted the projects
// into, keeping each row's scanned signals attached.
func reorder(rows []row, ordered []*project.Project) []row {
	byDir := make(map[string]row, len(rows))
	for _, r := range rows {
		byDir[r.project.Dir()] = r
	}
	out := make([]row, 0, len(ordered))
	for _, p := range ordered {
		out = append(out, byDir[p.Dir()])
	}
	return out
}
