package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

type trashOptions struct {
	yes   bool
	force bool
}

func newTrashCommand(app *App) *cobra.Command {
	var opts trashOptions

	cmd := &cobra.Command{
		Use:     "trash <name>...",
		Aliases: []string{"rm", "delete"},
		Short:   "Move a project to the trash",
		Long: "Move a project into the recovery area, where `sp restore` can bring\n" +
			"it back. Nothing is erased unless you pass --force.",
		Args:              cobra.MinimumNArgs(1),
		Example:           "  sp trash old-website\n  sp trash old-api rust-test --yes\n  sp trash junk --force",
		ValidArgsFunction: app.completeProjects(store.Scratch, store.Kept),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runTrash(cmd.Context(), args, opts)
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&opts.yes, "yes", "y", false, "skip the confirmation prompt")
	f.BoolVar(&opts.force, "force", false, "delete permanently instead of moving to the trash")

	return cmd
}

func (a *App) runTrash(ctx context.Context, names []string, opts trashOptions) error {
	for _, name := range names {
		if err := a.trashOne(ctx, name, opts); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) trashOne(ctx context.Context, name string, opts trashOptions) error {
	locs := []store.Location{store.Scratch, store.Kept}
	if opts.force {
		// Emptying something out of the trash for good is still `sp trash
		// --force`, so the trash has to be searched too.
		locs = append(locs, store.Trash)
	}
	p, err := a.resolve(name, locs...)
	if err != nil {
		return err
	}

	status, err := gitx.Read(ctx, p.Dir())
	if err != nil {
		return err
	}
	a.printTrashReport(p, status, opts.force)

	if !opts.yes {
		question := fmt.Sprintf("Move %s to the trash?", ui.Bold.Render(p.Name))
		if opts.force {
			question = fmt.Sprintf("%s %s permanently? This cannot be undone.",
				ui.Danger.Render("Delete"), ui.Bold.Render(p.Name))
		}
		// Anything with work at risk starts at "no", so a reflexive Enter is
		// never the destructive answer.
		ok, err := a.confirm(question, status.Clean() && !opts.force)
		if err != nil {
			return err
		}
		if !ok {
			a.println(ui.Muted.Render("Cancelled."))
			return nil
		}
	}

	if opts.force {
		if err := a.store.Destroy(p); err != nil {
			return err
		}
		a.println(ui.Check("Deleted %s permanently", ui.Bold.Render(p.Name)))
		return nil
	}

	if err := a.store.Trash(p); err != nil {
		var occupied *store.OccupiedError
		if errors.As(err, &occupied) {
			return fmt.Errorf("cannot trash %s: %s is in the way", p.Name, ui.Tildify(occupied.Path))
		}
		return err
	}

	a.println(
		ui.Check("Trashed %s", ui.Bold.Render(p.Name)),
		ui.Muted.Render(fmt.Sprintf("Restore it with `sp restore %s`.", p.Name)),
	)
	return nil
}

// printTrashReport shows what is about to be lost. It is printed before the
// prompt, never after, so the decision is made with the facts on screen.
func (a *App) printTrashReport(p *project.Project, status *gitx.Status, permanent bool) {
	now := a.store.Now()

	a.println("", ui.Bold.Render(p.Name))
	if p.Description != "" {
		a.println(ui.Muted.Render(p.Description))
	}
	a.println("")
	a.println(ui.Field("Location", ui.Muted.Render(ui.Tildify(p.Dir()))))
	a.println(ui.Field("Last used", ui.RelativeTime(p.LastActivity(), now)))
	a.println(ui.Field("Age", ui.Duration(p.Age(now))))

	if status != nil {
		branch := status.Branch
		if branch == "" {
			branch = "no commits yet"
		}
		a.println(ui.Field("Git", fmt.Sprintf("%s · %s", branch, plural(status.Commits, "commit"))))
	}

	if risk := status.Summary(); risk != "" {
		a.println("", ui.Warn("%s", ui.Warning.Render(risk)))
		for _, f := range firstN(status.Dirty, 5) {
			a.println("  " + ui.Muted.Render(f))
		}
		if extra := len(status.Dirty) - 5; extra > 0 {
			a.println("  " + ui.Muted.Render(fmt.Sprintf("… and %d more", extra)))
		}
	}
	if permanent {
		a.println("", ui.Danger.Render("This will not go to the trash. It will be gone."))
	}
	a.println("")
}

func firstN(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}
