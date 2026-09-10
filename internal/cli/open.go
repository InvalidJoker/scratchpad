package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

type openOptions struct {
	printPath bool
	reveal    bool
	noEditor  bool
}

func newOpenCommand(app *App) *cobra.Command {
	var opts openOptions

	cmd := &cobra.Command{
		Use:     "open <name>",
		Aliases: []string{"o"},
		Short:   "Open a project and mark it active",
		Long: "Open a project in your editor and record the visit.\n\n" +
			"The name can be a prefix: `sp open weath` finds weather-app.",
		Args: cobra.ExactArgs(1),
		Example: "  sp open weather-app\n" +
			"  cd \"$(sp open weather-app --print-path)\"\n" +
			"  sp open weather-app --reveal",
		ValidArgsFunction: app.completeProjects(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runOpen(cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&opts.printPath, "print-path", "p", false, "print the path instead of opening an editor")
	f.BoolVar(&opts.reveal, "reveal", false, "reveal the project in the file manager")
	f.BoolVar(&opts.noEditor, "no-editor", false, "record the visit without opening anything")

	return cmd
}

func (a *App) runOpen(cmd *cobra.Command, name string, opts openOptions) error {
	p, err := a.resolve(name, store.Scratch, store.Kept)
	if err != nil {
		return err
	}

	// The visit is recorded before launching, so a long-running editor does
	// not delay the signal and a crash does not lose it.
	p.Touch(a.store.Now())
	if err := a.store.Save(p); err != nil {
		return err
	}

	switch {
	case opts.printPath:
		a.println(p.Dir())
		return nil
	case opts.reveal:
		return a.reveal(cmd, p.Dir())
	case opts.noEditor:
		a.println(ui.Check("Marked %s as opened", ui.Bold.Render(p.Name)))
		return nil
	}

	editor := a.cfg.EditorCommand()
	if editor == "" {
		a.println(
			ui.Warn("no editor configured (set $EDITOR, $VISUAL, or editor in your config)"),
			ui.Accent.Render(p.Dir()),
		)
		return nil
	}
	return a.launch(cmd, editor, p.Dir())
}

// launch runs the editor with the project directory attached to the real
// terminal, so a terminal editor takes over the screen as expected.
func (a *App) launch(cmd *cobra.Command, editor, dir string) error {
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return fmt.Errorf("editor command is empty")
	}
	bin, args := fields[0], append(fields[1:], dir)

	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("editor %q not found on PATH", bin)
	}

	child := exec.CommandContext(cmd.Context(), bin, args...)
	child.Dir = dir
	child.Stdin, child.Stdout, child.Stderr = a.stdio()
	if err := child.Run(); err != nil {
		return fmt.Errorf("run %s: %w", bin, err)
	}
	return nil
}

func (a *App) reveal(cmd *cobra.Command, dir string) error {
	var bin string
	switch runtime.GOOS {
	case "darwin":
		bin = "open"
	case "windows":
		bin = "explorer"
	default:
		bin = "xdg-open"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("cannot reveal: %s not found on PATH", bin)
	}
	return exec.CommandContext(cmd.Context(), bin, dir).Run()
}
