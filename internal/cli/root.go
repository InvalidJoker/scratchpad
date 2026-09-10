package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/colorprofile"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/spf13/cobra"
)

// sprintf is a thin alias so app.go does not need to import fmt.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// NewRootCommand builds the full command tree. version is stamped at build
// time and surfaced through `sp --version`.
func NewRootCommand(version string) *cobra.Command {
	app := &App{out: os.Stdout}
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
			app.store = store.New(cfg)
			// Downsample colour to whatever the terminal supports, and strip
			// it entirely when output is piped to a file or another command.
			app.out = colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())
			return nil
		},
	}

	root.PersistentFlags().StringVar(&configPath, "config", "",
		"path to config file (default: OS config dir, or $SCRATCHPAD_CONFIG)")

	root.AddCommand(
		newNewCommand(app),
		newListCommand(app),
	)
	return root
}
