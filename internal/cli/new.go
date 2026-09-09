package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/scaffold"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

type newOptions struct {
	description string
	tags        []string
	ttl         string
	keep        bool
	noGit       bool
	noReadme    bool
	pathOnly    bool
}

func newNewCommand(app *App) *cobra.Command {
	var opts newOptions

	cmd := &cobra.Command{
		Use:     "new <name>",
		Aliases: []string{"n"},
		Short:   "Create a scratch project",
		Long: "Create a temporary project in the scratch directory.\n\n" +
			"The project expires after the configured default unless you keep it.",
		Args: cobra.ExactArgs(1),
		Example: "  sp new weather-app\n" +
			"  sp new api-test --ttl 7d --tag experiment\n" +
			"  sp new portfolio --keep\n" +
			"  cd \"$(sp new quick-idea --path)\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runNew(cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&opts.description, "description", "d", "", "one-line description of the idea")
	f.StringSliceVarP(&opts.tags, "tag", "t", nil, "tag the project (repeatable)")
	f.StringVar(&opts.ttl, "ttl", "", "time before the project expires, e.g. 7d, 2w, never")
	f.BoolVarP(&opts.keep, "keep", "k", false, "create it as a permanent project right away")
	f.BoolVar(&opts.noGit, "no-git", false, "skip git initialisation")
	f.BoolVar(&opts.noReadme, "no-readme", false, "skip the starter README")
	f.BoolVarP(&opts.pathOnly, "path", "p", false, "print only the project path, for use with cd")

	return cmd
}

func (a *App) runNew(cmd *cobra.Command, name string, opts newOptions) error {
	createOpts := store.CreateOptions{
		Name:        name,
		Description: strings.TrimSpace(opts.description),
		Tags:        normalizeTags(opts.tags),
		Permanent:   opts.keep,
	}
	if opts.ttl != "" {
		if opts.keep {
			return errors.New("--ttl and --keep conflict: a kept project does not expire")
		}
		ttl, err := config.ParseDuration(opts.ttl)
		if err != nil {
			return err
		}
		createOpts.TTL, createOpts.TTLSet = ttl, true
	}

	if err := a.store.EnsureDirs(); err != nil {
		return err
	}

	p, err := a.store.Create(createOpts)
	if err != nil {
		if errors.Is(err, store.ErrExists) {
			loc := store.Scratch
			if opts.keep {
				loc = store.Kept
			}
			return fmt.Errorf("%q already exists at %s", name, ui.Tildify(a.store.PathFor(loc, name)))
		}
		return err
	}

	scaffoldErr := scaffold.Apply(cmd.Context(), p, scaffold.Options{
		Git:    !opts.noGit && a.cfg.InitGit,
		Readme: !opts.noReadme,
	})

	if opts.pathOnly {
		a.println(p.Dir())
		return scaffoldErr
	}

	a.printNewSummary(p)
	if scaffoldErr != nil {
		// The project exists and is usable; scaffolding is a nicety.
		a.println("", ui.Warn("scaffolding incomplete: %v", scaffoldErr))
	}
	return nil
}

func (a *App) printNewSummary(p *project.Project) {
	now := a.store.Now()

	a.println("", ui.Check("Created %s", ui.Bold.Render(p.Name)), "")
	if p.Description != "" {
		a.println(ui.Muted.Render(p.Description), "")
	}

	if p.IsTemporary() {
		a.println(ui.Muted.Render("This project is temporary."), "")
		if p.ExpiresAt.IsZero() {
			a.println(ui.Field("Expires", ui.Muted.Render("never")))
		} else {
			a.println(ui.Field("Expires", fmt.Sprintf("%s %s",
				ui.RelativeFuture(p.ExpiresAt, now),
				ui.Muted.Render("("+p.ExpiresAt.Format("Jan 2, 2006")+")"))))
		}
	} else {
		a.println(ui.Muted.Render("This project is permanent."), "")
		a.println(ui.Field("Status", p.Status(now, 0).Icon()+" kept"))
	}

	a.println(ui.Field("Location", ui.Accent.Render(ui.Tildify(p.Dir()))))
	if len(p.Tags) > 0 {
		a.println(ui.Field("Tags", strings.Join(p.Tags, ", ")))
	}

	if p.IsTemporary() {
		a.println("", ui.Muted.Render(fmt.Sprintf("Keep it with `sp keep %s`, drop it with `sp trash %s`.", p.Name, p.Name)))
	}
}

// normalizeTags trims, lowercases and de-duplicates tags so filtering later
// does not have to.
func normalizeTags(tags []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
