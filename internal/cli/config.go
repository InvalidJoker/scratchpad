package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

func newConfigCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or change Scratchpad's settings",
		Long: "Print the resolved configuration, or change one setting.\n\n" +
			"Run `sp setup` for the guided version.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.runConfigShow()
		},
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:               "set <key> <value>",
			Short:             "Change one setting",
			Args:              cobra.ExactArgs(2),
			Example:           "  sp config set scratch_dir ~/scratch\n  sp config set default_expiration 14d\n  sp config set init_git false",
			ValidArgsFunction: completeConfigKeys,
			RunE: func(_ *cobra.Command, args []string) error {
				return app.runConfigSet(args[0], args[1])
			},
		},
		&cobra.Command{
			Use:               "get <key>",
			Short:             "Print one setting",
			Args:              cobra.ExactArgs(1),
			ValidArgsFunction: completeConfigKeys,
			RunE: func(_ *cobra.Command, args []string) error {
				value, err := configValue(app.cfg, args[0])
				if err != nil {
					return err
				}
				app.println(value)
				return nil
			},
		},
		&cobra.Command{
			Use:   "path",
			Short: "Print the config file location",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				app.println(app.cfg.Path())
				return nil
			},
		},
		&cobra.Command{
			Use:   "edit",
			Short: "Open the config file in your editor",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return app.runConfigEdit(cmd)
			},
		},
	)
	return cmd
}

func (a *App) runConfigShow() error {
	cfg := a.cfg

	a.println("", ui.Bold.Render("Scratchpad configuration"), "")
	for _, key := range configKeys() {
		value, err := configValue(cfg, key)
		if err != nil {
			return err
		}
		display := value
		if strings.HasSuffix(key, "_dir") {
			display = ui.Tildify(value)
		}
		if display == "" {
			display = ui.Muted.Render("(unset)")
		}
		a.println(ui.KeyValue(key, display))
	}

	a.println("", ui.Field("Config file", ui.Muted.Render(ui.Tildify(cfg.Path()))))
	if !cfg.Exists() {
		a.println(ui.Muted.Render("No config file yet — these are the defaults. Run `sp setup` to write them."))
	}
	a.println("")
	return nil
}

func (a *App) runConfigSet(key, value string) error {
	if err := setConfigValue(a.cfg, key, value); err != nil {
		return err
	}
	if err := a.cfg.Save(); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	stored, err := configValue(a.cfg, key)
	if err != nil {
		return err
	}
	a.println(ui.Check("%s = %s", key, ui.Accent.Render(stored)))
	return nil
}

func (a *App) runConfigEdit(cmd *cobra.Command) error {
	editor := a.cfg.EditorCommand()
	if editor == "" {
		return fmt.Errorf("no editor configured: set $EDITOR, or run `sp config set editor <command>`")
	}
	// Writing the file first means the editor never opens on nothing.
	if !a.cfg.Exists() {
		if err := a.cfg.Save(); err != nil {
			return err
		}
	}
	return a.launch(cmd, editor, a.cfg.Path())
}

// configField describes one setting for reading and writing by name, so the
// CLI does not need a switch per key in three different places.
type configField struct {
	get func(*config.Config) string
	set func(*config.Config, string) error
}

func configFields() map[string]configField {
	dirField := func(target func(*config.Config) *string) configField {
		return configField{
			get: func(c *config.Config) string { return *target(c) },
			set: func(c *config.Config, v string) error {
				expanded, err := config.Expand(v)
				if err != nil {
					return err
				}
				*target(c) = expanded
				return nil
			},
		}
	}
	durationField := func(target func(*config.Config) *config.Duration) configField {
		return configField{
			get: func(c *config.Config) string { return target(c).String() },
			set: func(c *config.Config, v string) error {
				d, err := config.ParseDuration(v)
				if err != nil {
					return err
				}
				*target(c) = config.Duration(d)
				return nil
			},
		}
	}

	return map[string]configField{
		"scratch_dir":  dirField(func(c *config.Config) *string { return &c.ScratchDir }),
		"projects_dir": dirField(func(c *config.Config) *string { return &c.ProjectsDir }),
		"archive_dir":  dirField(func(c *config.Config) *string { return &c.ArchiveDir }),
		"trash_dir":    dirField(func(c *config.Config) *string { return &c.TrashDir }),
		"default_expiration": durationField(func(c *config.Config) *config.Duration {
			return &c.DefaultExpiration
		}),
		"stale_after": durationField(func(c *config.Config) *config.Duration { return &c.StaleAfter }),
		"init_git": {
			get: func(c *config.Config) string { return strconv.FormatBool(c.InitGit) },
			set: func(c *config.Config, v string) error {
				b, err := strconv.ParseBool(v)
				if err != nil {
					return fmt.Errorf("init_git must be true or false, not %q", v)
				}
				c.InitGit = b
				return nil
			},
		},
		"editor": {
			get: func(c *config.Config) string { return c.Editor },
			set: func(c *config.Config, v string) error { c.Editor = v; return nil },
		},
	}
}

func configKeys() []string {
	fields := configFields()
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func configValue(cfg *config.Config, key string) (string, error) {
	field, ok := configFields()[key]
	if !ok {
		return "", unknownKey(key)
	}
	return field.get(cfg), nil
}

func setConfigValue(cfg *config.Config, key, value string) error {
	field, ok := configFields()[key]
	if !ok {
		return unknownKey(key)
	}
	return field.set(cfg, value)
}

func unknownKey(key string) error {
	return fmt.Errorf("unknown setting %q: try one of %s", key, strings.Join(configKeys(), ", "))
}

func completeConfigKeys(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, key := range configKeys() {
		if strings.HasPrefix(key, toComplete) {
			out = append(out, key)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
