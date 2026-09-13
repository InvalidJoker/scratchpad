package cli

import (
	"os"

	"github.com/charmbracelet/colorprofile"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/spf13/cobra"
)

// commands that don't need any configuration or state to run, and therefore
// don't need to run the setup wizard first.
var selfSufficient = map[string]bool{
	"setup":            true,
	"init":             true,
	"help":             true,
	"completion":       true,
	"shell-init":       true,
	"__complete":       true,
	"__completeNoDesc": true,
	"bash":             true,
	"zsh":              true,
	"fish":             true,
	"powershell":       true,
}

func wantsSetup(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if selfSufficient[c.Name()] {
			return false
		}
	}
	return true
}

func NewRootCommand(version string) *cobra.Command {
	app := &App{out: os.Stdout, in: os.Stdin}
	var configPath string

	root := &cobra.Command{
		Use:   "sp",
		Short: "A lifecycle manager for projects you're not sure deserve to be real yet",
		Long: "Scratchpad makes experimentation cheap.\n\n" +
			"Start something without deciding whether it matters, then keep it,\n" +
			"archive it, or throw it away once you know.\n\n" +
			"Run `sp` on its own to open the dashboard.",
		Version:      version,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runDashboard(cmd.Context())
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			app.cfg = cfg
			app.rebuildStore()
			app.out = colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())

			// run setup automatically if the config file doesn't exist, we're in a terminal,
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
