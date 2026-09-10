package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type cleanOptions struct {
	olderThan string
	dryRun    bool
	yes       bool
	force     bool
}

// candidate is one project up for cleanup, with the facts needed to decide.
type candidate struct {
	project *project.Project
	status  project.Status
	git     *gitx.Status
	size    int64
}

// risky reports whether trashing this would bin unsaved edits. Commits that
// exist only locally do not count: `sp clean` moves projects to the trash,
// where they stay recoverable, so the gate here is unsaved work rather than
// unpushed work.
func (c candidate) risky() bool { return c.git.HasUncommitted() }

func newCleanCommand(app *App) *cobra.Command {
	var opts cleanOptions

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Review stale and expired projects",
		Long: "Review the projects that have gone quiet and decide what to do.\n\n" +
			"This is a review, not a purge: nothing is selected for you if it has\n" +
			"uncommitted or unpushed work, and everything goes to the trash rather\n" +
			"than being deleted.",
		Args:    cobra.NoArgs,
		Example: "  sp clean\n  sp clean --older-than 60d\n  sp clean --dry-run",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runClean(cmd.Context(), opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.olderThan, "older-than", "", "only consider projects untouched for longer than this, e.g. 60d")
	f.BoolVar(&opts.dryRun, "dry-run", false, "show what would happen and change nothing")
	f.BoolVarP(&opts.yes, "yes", "y", false, "trash every safe candidate without asking")
	f.BoolVar(&opts.force, "force", false, "with --yes, also trash projects that have work at risk")

	return cmd
}

func (a *App) runClean(ctx context.Context, opts cleanOptions) error {
	candidates, err := a.cleanCandidates(ctx, opts)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		a.println("", ui.Check("Nothing to clean up"), "")
		return nil
	}

	a.printCandidates(candidates)

	// Without a terminal there is nobody to review anything, so the only
	// honest behaviour is to report and stop.
	if opts.dryRun || (!opts.yes && !a.interactive()) {
		return a.reportDryRun(candidates, opts)
	}

	selected, err := a.selectForCleanup(candidates, opts)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			a.println(ui.Muted.Render("Cancelled. Nothing was touched."))
			return nil
		}
		return err
	}
	if len(selected) == 0 {
		a.println(ui.Muted.Render("Nothing selected."))
		return nil
	}

	return a.trashSelected(selected)
}

// cleanCandidates gathers stale and expired projects, with their git status
// and size attached.
func (a *App) cleanCandidates(ctx context.Context, opts cleanOptions) ([]candidate, error) {
	filter := store.Filter{
		Locations: []store.Location{store.Scratch},
		Statuses:  []project.Status{project.StatusStale, project.StatusExpired},
		Sort:      store.SortActivity,
	}
	if opts.olderThan != "" {
		idle, err := config.ParseDuration(opts.olderThan)
		if err != nil {
			return nil, err
		}
		filter.IdleFor = idle
	}

	projects, err := a.store.Query(filter)
	if err != nil {
		return nil, err
	}

	candidates := make([]candidate, 0, len(projects))
	for _, p := range projects {
		status, err := gitx.Read(ctx, p.Dir())
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate{
			project: p,
			status:  a.store.StatusOf(p),
			git:     status,
			size:    store.DirSize(p.Dir()),
		})
	}
	return candidates, nil
}

func (a *App) printCandidates(candidates []candidate) {
	now := a.store.Now()

	table := ui.NewTable("name", "idle", "size", "status", "git")
	for _, c := range candidates {
		table.Row(
			ui.Bold.Render(c.project.Name),
			ui.Duration(c.project.Idle(now)),
			ui.Bytes(c.size),
			ui.StatusBadge(c.status),
			gitColumn(c),
		)
	}

	a.println("", ui.Bold.Render(fmt.Sprintf("%s gone quiet", plural(len(candidates), "project"))), "")
	a.println(table.Render(), "")
	a.println(ui.Muted.Render(fmt.Sprintf("%s in total", ui.Bytes(totalSize(candidates)))), "")
}

func (a *App) reportDryRun(candidates []candidate, opts cleanOptions) error {
	safe := filterCandidates(candidates, false)

	if len(safe) == 0 {
		a.println(ui.Warn("every candidate has uncommitted work, so nothing would be trashed automatically"))
	} else {
		a.println(ui.Muted.Render(fmt.Sprintf("Would trash %s, freeing %s:",
			plural(len(safe), "project"), ui.Bytes(totalSize(safe)))))
		for _, c := range safe {
			a.println("  " + c.project.Name)
		}
	}
	if risky := filterCandidates(candidates, true); len(risky) > 0 {
		a.println("", ui.Warn("%s left alone because of uncommitted work:", plural(len(risky), "project")))
		for _, c := range risky {
			a.println("  " + c.project.Name + ui.Muted.Render(" — "+c.git.Summary()))
		}
	}

	if !opts.dryRun {
		a.println("", ui.Muted.Render("Run `sp clean` from a terminal to review these, or add --yes."))
	}
	a.println("")
	return nil
}

// selectForCleanup decides which projects to trash, either from the user's
// review or, with --yes, from the safe ones alone.
func (a *App) selectForCleanup(candidates []candidate, opts cleanOptions) ([]candidate, error) {
	if opts.yes {
		if opts.force {
			return candidates, nil
		}
		safe := filterCandidates(candidates, false)
		if risky := len(candidates) - len(safe); risky > 0 {
			a.println(ui.Warn("skipping %s with uncommitted work (use --force to include them)", plural(risky, "project")))
		}
		return safe, nil
	}

	options := make([]huh.Option[string], 0, len(candidates))
	byName := make(map[string]candidate, len(candidates))
	for _, c := range candidates {
		label := fmt.Sprintf("%-24s %8s  %s", c.project.Name, ui.Bytes(c.size), ui.Duration(c.project.Idle(a.store.Now())))
		if c.git.HasUncommitted() {
			label += "  ⚠ " + c.git.Summary()
		}
		byName[c.project.Name] = c
		// Only projects with nothing at risk start ticked, so the fast path
		// through this form can never bin work by default.
		options = append(options, huh.NewOption(label, c.project.Name).Selected(!c.risky()))
	}

	var chosen []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which of these should go to the trash?").
				Description("Space toggles · anything with uncommitted work starts unticked.").
				Options(options...).
				Value(&chosen),
		),
	).WithTheme(ui.FormTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ui.ErrCancelled
		}
		return nil, err
	}

	selected := make([]candidate, 0, len(chosen))
	for _, name := range chosen {
		selected = append(selected, byName[name])
	}

	if len(selected) == 0 {
		return nil, nil
	}
	ok, err := ui.Confirm(
		fmt.Sprintf("Trash %s, freeing %s?", plural(len(selected), "project"), ui.Bytes(totalSize(selected))),
		"Trash them", "Cancel", true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return selected, nil
}

func (a *App) trashSelected(selected []candidate) error {
	var freed int64
	var failed int

	for _, c := range selected {
		if err := a.store.Trash(c.project); err != nil {
			a.println(ui.Warn("could not trash %s: %v", c.project.Name, err))
			failed++
			continue
		}
		freed += c.size
	}

	moved := len(selected) - failed
	a.println("", ui.Check("Trashed %s, freeing %s", plural(moved, "project"), ui.Bytes(freed)))
	a.println(ui.Muted.Render("They are recoverable with `sp restore <name>` until you empty the trash."), "")

	if failed > 0 {
		return fmt.Errorf("%s could not be trashed", plural(failed, "project"))
	}
	return nil
}

// gitColumn describes the git state at the level of detail the decision needs:
// unsaved edits are a warning, local-only commits are a note, anything else is
// fine.
func gitColumn(c candidate) string {
	switch {
	case c.git == nil:
		return ui.Muted.Render("no repo")
	case c.git.HasUncommitted():
		return ui.Warning.Render(c.git.Summary())
	case c.git.LocalOnly():
		return ui.Muted.Render(fmt.Sprintf("%s, local only", plural(c.git.Commits, "commit")))
	default:
		return ui.Success.Render("clean")
	}
}

func filterCandidates(candidates []candidate, risky bool) []candidate {
	out := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.risky() == risky {
			out = append(out, c)
		}
	}
	return out
}

func totalSize(candidates []candidate) int64 {
	var total int64
	for _, c := range candidates {
		total += c.size
	}
	return total
}
