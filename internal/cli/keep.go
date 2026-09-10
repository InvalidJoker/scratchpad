package cli

import (
	"errors"
	"fmt"

	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

func newKeepCommand(app *App) *cobra.Command {
	var dest string

	cmd := &cobra.Command{
		Use:     "keep <name>",
		Aliases: []string{"k", "promote"},
		Short:   "Promote a project out of scratch, permanently",
		Long: "Move a project into your permanent projects directory.\n\n" +
			"It stops expiring, stops appearing in scratch listings, and is never\n" +
			"offered up by `sp clean` again.",
		Args:              cobra.ExactArgs(1),
		Example:           "  sp keep awesome-app\n  sp keep awesome-app --to ~/work",
		ValidArgsFunction: app.completeProjects(store.Scratch),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.runKeep(args[0], dest)
		},
	}

	cmd.Flags().StringVar(&dest, "to", "", "parent directory to move it into (default: projects_dir)")
	return cmd
}

func (a *App) runKeep(name, dest string) error {
	p, err := a.resolve(name, store.Scratch, store.Trash)
	if err != nil {
		return err
	}

	from := p.Dir()
	if err := a.store.Keep(p, dest); err != nil {
		var occupied *store.OccupiedError
		if errors.As(err, &occupied) {
			return fmt.Errorf("cannot keep %s: something already exists at %s",
				p.Name, ui.Tildify(occupied.Path))
		}
		return err
	}

	a.println("",
		ui.Check("Kept %s", ui.Bold.Render(p.Name)),
		"",
		ui.Field("From", ui.Muted.Render(ui.Tildify(from))),
		ui.Field("To", ui.Accent.Render(ui.Tildify(p.Dir()))),
		ui.Field("Status", ui.StatusBadge(a.store.StatusOf(p))),
		"",
		ui.Muted.Render("It no longer expires."),
	)
	return nil
}
