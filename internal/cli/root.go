package cli

import (
	"os"

	"github.com/charmbracelet/colorprofile"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/spf13/cobra"
)

// selfSufficient are the commands that must work before Scratchpad is
// configured, either because they are the setup itself or because a shell is
// calling them non-interactively.
var selfSufficient = map[string]bool{
	"setup":      true,
	"init":       true,
	"help":       true,
	"completion": true,
	"shell-init":       true,
	"__complete":       true,
	"__completeNoDesc": true,
	"bash":             true,
	"zsh":              true,
	"fish":             true,
	"powershell":       true,
}

// wantsSetup reports whether running the wizard before this command makes
// sense.
func wantsSetup(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if selfSufficient[c.Name()] {
			return false
		}
	}
	return true
}

// NewRootCommand builds the full command tree. version is stamped at build
// time and surfaced through `sp --version`.
func NewRootCommand(version string) *cobra.Command {
	app := &App{out: os.Stdout, in: os.Stdin}
	var configPath string

	root := &cobra.Command{
		Use:   "sp",
		Short: "A lifecycle manager for projects you're not sure deserve to be real yet",
		Long: "Scratchpad makes experimentation cheap.\n\n" +
			"Start something without deciding whether it matters, then keep it,\n" +
			"archive it, or throw it away once you know.",
		Version:      version,
		SilenceUsage: true,
		// Commands resolve their own arguments; an unknown one should not
		// silently do nothing.
		Args: cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			app.cfg = cfg
			app.rebuildStore()
			// Downsample colour to whatever the terminal supports, and strip
			// it entirely when output is piped to a file or another command.
			app.out = colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())

			// A first run gets the wizard, then carries on with whatever the
			// user actually typed. Non-interactive runs stay silent on
			// defaults so scripts and CI are never blocked on a prompt.
			if !cfg.Exists() && app.interactive() && wantsSetup(cmd) {
				if err := app.runSetup(setupOptions{firstRun: true}); err != nil {
					return err
				}
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&configPath, "config", "",
		"path to config file (default: OS config dir, or $SCRATCHPAD_CONFIG)")

	root.CompletionOptions.HiddenDefaultCmd = true
	root.AddCommand(
		newSetupCommand(app),
		newNewCommand(app),
		newListCommand(app),
		newOpenCommand(app),
		newInfoCommand(app),
		newKeepCommand(app),
		newTrashCommand(app),
		newRestoreCommand(app),
		newCleanCommand(app),
		newConfigCommand(app),
		newShellInitCommand(app),
	)
	return root
}
