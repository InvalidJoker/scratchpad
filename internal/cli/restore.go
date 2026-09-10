package cli

import (
	"errors"
	"fmt"

	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

func newRestoreCommand(app *App) *cobra.Command {
	var list bool

	cmd := &cobra.Command{
		Use:     "restore <name>",
		Aliases: []string{"undelete"},
		Short:   "Bring a project back from the trash",
		Long: "Restore a trashed project to where it came from.\n\n" +
			"If that location is taken, it comes back to the scratch directory\n" +
			"with a fresh expiry window.",
		Args:              cobra.MaximumNArgs(1),
		Example:           "  sp restore old-website\n  sp restore --list",
		ValidArgsFunction: app.completeProjects(store.Trash),
		RunE: func(_ *cobra.Command, args []string) error {
			if list || len(args) == 0 {
				return app.runList(listOptions{trashed: true, sortKey: "used"})
			}
			return app.runRestore(args[0])
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "show what is in the trash")
	return cmd
}

func (a *App) runRestore(name string) error {
	p, err := a.resolve(name, store.Trash)
	if err != nil {
		return err
	}

	from := p.Dir()
	original := p.OriginalPath
	if err := a.store.Restore(p); err != nil {
		var occupied *store.OccupiedError
		if errors.As(err, &occupied) {
			return fmt.Errorf("cannot restore %s: %s already exists — rename or move it first",
				p.Name, ui.Tildify(occupied.Path))
		}
		return err
	}

	a.println("",
		ui.Check("Restored %s", ui.Bold.Render(p.Name)),
		"",
		ui.Field("From", ui.Muted.Render(ui.Tildify(from))),
		ui.Field("To", ui.Accent.Render(ui.Tildify(p.Dir()))),
	)
	if original != "" && original != p.Dir() {
		a.println(ui.Warn("its original location was taken, so it came back to scratch"))
	}
	if !p.ExpiresAt.IsZero() {
		a.println(ui.Field("Expires", ui.RelativeFuture(p.ExpiresAt, a.store.Now())))
	}
	a.println("")
	return nil
}
