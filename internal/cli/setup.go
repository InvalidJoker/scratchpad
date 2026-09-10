package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newSetupCommand(app *App) *cobra.Command {
	var reset bool

	cmd := &cobra.Command{
		Use:     "setup",
		Aliases: []string{"init"},
		Short:   "Configure Scratchpad",
		Long: "Walk through where Scratchpad keeps things and how long scratch\n" +
			"projects live before they are offered up for cleanup.\n\n" +
			"Runs automatically the first time you use Scratchpad.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.runSetup(setupOptions{reset: reset})
		},
	}

	cmd.Flags().BoolVar(&reset, "reset", false, "start from defaults instead of the current config")
	return cmd
}

type setupOptions struct {
	// firstRun changes the wording for someone who has never used Scratchpad.
	firstRun bool
	reset    bool
}

// setupAnswers holds the form state. huh binds to strings, so durations are
// parsed back out after the form completes.
type setupAnswers struct {
	scratchDir  string
	projectsDir string
	archiveDir  string
	trashDir    string
	expiration  string
	staleAfter  string
	editor      string
	initGit     bool
	advanced    bool
}

func (a *App) runSetup(opts setupOptions) error {
	if !a.interactive() {
		return errors.New("setup needs a terminal: edit the config file directly, or run `sp setup` from a shell")
	}

	base := a.cfg
	if opts.reset || base == nil {
		base = config.Default()
	}
	answers := answersFrom(base)

	if err := a.runSetupForm(&answers, opts); err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			a.println("", ui.Muted.Render("Setup cancelled. Nothing was written."))
			return nil
		}
		return err
	}

	cfg, err := answers.toConfig(a.cfg.Path())
	if err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	a.cfg = cfg
	a.rebuildStore()
	if err := a.store.EnsureDirs(); err != nil {
		return err
	}

	a.printSetupSummary(cfg, opts)
	return nil
}

func (a *App) runSetupForm(ans *setupAnswers, opts setupOptions) error {
	intro := "Scratchpad keeps experimental projects in one place and makes\n" +
		"deciding their fate explicit: keep them, archive them, or bin them."
	if !opts.firstRun {
		intro = "Update where Scratchpad keeps things and how long projects live."
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Welcome to Scratchpad").
				Description(intro+"\n"),
			huh.NewInput().
				Title("Scratch directory").
				Description("Where new experiments are created.").
				Placeholder("~/Downloads/Scratchpad").
				Value(&ans.scratchDir).
				Validate(validateDir("scratch directory")),
			huh.NewInput().
				Title("Permanent projects directory").
				Description("Where `sp keep` promotes a project that turned out to matter.").
				Placeholder("~/Projects").
				Value(&ans.projectsDir).
				Validate(validateDir("projects directory")),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default lifespan").
				Description("How long a new project lives before `sp clean` offers it up.\nNothing is ever deleted automatically.").
				Options(
					huh.NewOption("7 days", "7d"),
					huh.NewOption("14 days", "14d"),
					huh.NewOption("30 days", "30d"),
					huh.NewOption("90 days", "90d"),
					huh.NewOption("Never expire", "never"),
				).
				Value(&ans.expiration),
			huh.NewSelect[string]().
				Title("Mark as stale after").
				Description("How long without activity before a project looks abandoned.").
				Options(
					huh.NewOption("7 days", "7d"),
					huh.NewOption("14 days", "14d"),
					huh.NewOption("30 days", "30d"),
					huh.NewOption("Never", "never"),
				).
				Value(&ans.staleAfter),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Editor").
				Description("What `sp open` launches.").
				Options(editorOptions(ans.editor)...).
				Value(&ans.editor),
			huh.NewConfirm().
				Title("Run `git init` in new projects?").
				Description("Scratchpad uses git to tell you what you would lose before deleting anything.").
				Affirmative("Yes").
				Negative("No").
				Value(&ans.initGit),
			huh.NewConfirm().
				Title("Customise the trash and archive locations?").
				Description("The defaults sit alongside your scratch directory.").
				Affirmative("Customise").
				Negative("Use defaults").
				Value(&ans.advanced),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Trash directory").
				Description("Where `sp trash` moves projects, and `sp restore` finds them.").
				Value(&ans.trashDir).
				Validate(validateDir("trash directory")),
			huh.NewInput().
				Title("Archive directory").
				Description("Where `sp archive` writes compressed projects.").
				Value(&ans.archiveDir).
				Validate(validateDir("archive directory")),
		).WithHideFunc(func() bool { return !ans.advanced }),
	).WithTheme(ui.FormTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ui.ErrCancelled
		}
		return err
	}

	// Derived paths follow the scratch directory unless the user took control
	// of them, so changing the scratch directory does not strand the trash.
	if !ans.advanced {
		expanded, err := config.Expand(ans.scratchDir)
		if err != nil {
			return err
		}
		ans.trashDir = filepath.Join(expanded, ".trash")
	}
	return nil
}

func answersFrom(cfg *config.Config) setupAnswers {
	return setupAnswers{
		scratchDir:  ui.Tildify(cfg.ScratchDir),
		projectsDir: ui.Tildify(cfg.ProjectsDir),
		archiveDir:  ui.Tildify(cfg.ArchiveDir),
		trashDir:    ui.Tildify(cfg.TrashDir),
		expiration:  cfg.DefaultExpiration.String(),
		staleAfter:  cfg.StaleAfter.String(),
		editor:      cfg.Editor,
		initGit:     cfg.InitGit,
	}
}

func (ans setupAnswers) toConfig(path string) (*config.Config, error) {
	cfg := config.Default()
	cfg.SetPath(path)

	for _, field := range []struct {
		value  string
		target *string
	}{
		{ans.scratchDir, &cfg.ScratchDir},
		{ans.projectsDir, &cfg.ProjectsDir},
		{ans.archiveDir, &cfg.ArchiveDir},
		{ans.trashDir, &cfg.TrashDir},
	} {
		expanded, err := config.Expand(field.value)
		if err != nil {
			return nil, err
		}
		*field.target = expanded
	}

	expiration, err := config.ParseDuration(ans.expiration)
	if err != nil {
		return nil, err
	}
	stale, err := config.ParseDuration(ans.staleAfter)
	if err != nil {
		return nil, err
	}

	cfg.DefaultExpiration = config.Duration(expiration)
	cfg.StaleAfter = config.Duration(stale)
	cfg.InitGit = ans.initGit
	cfg.Editor = ans.editor
	return cfg, nil
}

func (a *App) printSetupSummary(cfg *config.Config, opts setupOptions) {
	a.println("", ui.Check("Scratchpad is configured"), "")
	a.println(ui.Field("Scratch", ui.Accent.Render(ui.Tildify(cfg.ScratchDir))))
	a.println(ui.Field("Projects", ui.Muted.Render(ui.Tildify(cfg.ProjectsDir))))
	a.println(ui.Field("Trash", ui.Muted.Render(ui.Tildify(cfg.TrashDir))))
	a.println(ui.Field("Expires", ui.Muted.Render(cfg.DefaultExpiration.String())))
	a.println(ui.Field("Config", ui.Muted.Render(ui.Tildify(cfg.Path()))))

	if opts.firstRun {
		a.println("", ui.Muted.Render("Start your first one with ")+ui.Accent.Render("sp new <name>")+ui.Muted.Render("."), "")
		return
	}
	a.println("")
}

// validateDir rejects paths that cannot become a usable directory, while the
// form is still open and the user can fix them.
func validateDir(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", label)
		}
		expanded, err := config.Expand(value)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
			return fmt.Errorf("%s exists and is a file", label)
		}
		return nil
	}
}

// knownEditors are offered when they are actually installed, so the list is
// never a menu of things that will fail to launch.
var knownEditors = []struct{ label, command string }{
	{"VS Code", "code"},
	{"Cursor", "cursor"},
	{"Zed", "zed"},
	{"Neovim", "nvim"},
	{"Vim", "vim"},
	{"Helix", "hx"},
	{"Emacs", "emacs"},
	{"Sublime Text", "subl"},
	{"Nano", "nano"},
}

func editorOptions(current string) []huh.Option[string] {
	options := []huh.Option[string]{
		huh.NewOption("Use $EDITOR / $VISUAL", ""),
	}
	seen := map[string]bool{"": true}

	for _, e := range knownEditors {
		if seen[e.command] {
			continue
		}
		if _, err := exec.LookPath(e.command); err != nil {
			continue
		}
		seen[e.command] = true
		options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", e.label, e.command), e.command))
	}

	// A configured editor that is not on the list still has to be selectable,
	// otherwise reopening setup would silently discard it.
	if current != "" && !seen[current] {
		options = append(options, huh.NewOption(current, current))
	}
	return options
}
