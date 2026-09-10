package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newShellInitCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:       "shell-init [bash|zsh|fish]",
		Short:     "Print shell functions that can change your directory",
		ValidArgs: []string{"bash", "zsh", "fish"},
		Args:      cobra.MaximumNArgs(1),
		Long: "Print shell functions to evaluate in your shell profile.\n\n" +
			"A command cannot change its parent shell's directory, so `sp open`\n" +
			"can only print a path. These wrappers do the `cd` for you:\n\n" +
			"  spo <name>   open a project and cd into it\n" +
			"  spn <name>   create a project and cd into it\n\n" +
			"Add to your profile:\n\n" +
			"  eval \"$(sp shell-init)\"          # bash and zsh\n" +
			"  sp shell-init fish | source      # fish",
		Example: "  eval \"$(sp shell-init)\"\n  sp shell-init zsh >> ~/.zshrc",
		RunE: func(_ *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			}
			script, err := shellInitScript(shell)
			if err != nil {
				return err
			}
			app.println(script)
			return nil
		},
	}
}

func shellInitScript(shell string) (string, error) {
	if shell == "" {
		shell = detectShell()
	}
	switch shell {
	case "bash", "zsh":
		return posixShellInit, nil
	case "fish":
		return fishShellInit, nil
	default:
		return "", fmt.Errorf("unsupported shell %q: use bash, zsh or fish", shell)
	}
}

// detectShell falls back to bash, whose syntax zsh also accepts, so an
// unrecognised $SHELL still produces something usable.
func detectShell() string {
	base := filepath.Base(os.Getenv("SHELL"))
	switch {
	case strings.Contains(base, "fish"):
		return "fish"
	case strings.Contains(base, "zsh"):
		return "zsh"
	default:
		return "bash"
	}
}

const posixShellInit = `# Scratchpad shell integration.
# Add to your profile with: eval "$(sp shell-init)"

# spo <name> — open a project and cd into it.
spo() {
  local dir
  dir="$(sp open --print-path "$@")" || return $?
  [ -n "$dir" ] && cd "$dir" || return 1
}

# spn <name> — create a project and cd into it.
spn() {
  local dir
  dir="$(sp new --path "$@")" || return $?
  [ -n "$dir" ] && cd "$dir" || return 1
}`

const fishShellInit = `# Scratchpad shell integration.
# Add to your config with: sp shell-init fish | source

# spo <name> — open a project and cd into it.
function spo
    set -l dir (sp open --print-path $argv); or return $status
    test -n "$dir"; and cd $dir
end

# spn <name> — create a project and cd into it.
function spn
    set -l dir (sp new --path $argv); or return $status
    test -n "$dir"; and cd $dir
end`
