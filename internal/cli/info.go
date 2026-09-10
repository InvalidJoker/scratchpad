package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/gitx"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

func newInfoCommand(app *App) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:               "info <name>",
		Aliases:           []string{"show"},
		Short:             "Show everything known about a project",
		Args:              cobra.ExactArgs(1),
		Example:           "  sp info weather-app\n  sp info weather-app --json",
		ValidArgsFunction: app.completeProjects(store.Scratch, store.Kept, store.Trash),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runInfo(cmd.Context(), args[0], asJSON)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func (a *App) runInfo(ctx context.Context, name string, asJSON bool) error {
	p, err := a.resolve(name)
	if err != nil {
		return err
	}
	if asJSON {
		return a.printJSON([]*project.Project{p})
	}

	now := a.store.Now()
	a.println("", ui.Bold.Render(p.Name))
	if p.Description != "" {
		a.println(ui.Muted.Render(p.Description))
	}
	a.println("")

	a.println(ui.Field("Status", ui.StatusBadge(a.store.StatusOf(p))))
	a.println(ui.Field("Location", ui.Accent.Render(ui.Tildify(p.Dir()))))
	a.println(ui.Field("Created", fmt.Sprintf("%s %s",
		p.Created.Format("Jan 2, 2006"), ui.Muted.Render("("+ui.RelativeTime(p.Created, now)+")"))))
	a.println(ui.Field("Last used", ui.RelativeTime(p.LastActivity(), now)))
	a.println(ui.Field("Opened", plural(p.OpenCount, "time")))

	switch {
	case p.State == project.StateTrashed:
		a.println(ui.Field("Trashed", ui.RelativeTime(p.TrashedAt, now)))
		if p.OriginalPath != "" {
			a.println(ui.Field("Came from", ui.Muted.Render(ui.Tildify(p.OriginalPath))))
		}
	case p.ExpiresAt.IsZero():
		a.println(ui.Field("Expires", ui.Muted.Render("never")))
	default:
		a.println(ui.Field("Expires", fmt.Sprintf("%s %s",
			ui.RelativeFuture(p.ExpiresAt, now), ui.Muted.Render("("+p.ExpiresAt.Format("Jan 2, 2006")+")"))))
	}

	if len(p.Tags) > 0 {
		a.println(ui.Field("Tags", strings.Join(p.Tags, ", ")))
	}

	status, err := gitx.Read(ctx, p.Dir())
	if err != nil {
		return err
	}
	a.printGitBlock(status)

	if p.Note != "" {
		a.println("", ui.Header.Render("NOTE"), p.Note)
	}
	a.println("")
	return nil
}

func (a *App) printGitBlock(status *gitx.Status) {
	if status == nil {
		a.println(ui.Field("Git", ui.Muted.Render("not a repository")))
		return
	}

	branch := status.Branch
	if branch == "" {
		branch = ui.Muted.Render("no commits yet")
	}
	a.println("", ui.Header.Render("GIT"))
	a.println(ui.Field("  branch", branch))
	a.println(ui.Field("  commits", fmt.Sprintf("%d", status.Commits)))
	if !status.LastCommit.IsZero() {
		a.println(ui.Field("  last", ui.RelativeTime(status.LastCommit, a.store.Now())))
	}

	changes := ui.Success.Render("clean")
	if risk := status.Summary(); risk != "" {
		changes = ui.Warning.Render(risk)
	}
	a.println(ui.Field("  changes", changes))
}
