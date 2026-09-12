package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newShellInitCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init [bash|zsh|fish|powershell]",
		Short: "Print shell functions that can change your directory",
		Hidden:    true,
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MaximumNArgs(1),
		Long: "Print shell functions to evaluate in your shell profile.\n\n" +
			"A command cannot change its parent shell's directory, so `sp open`\n" +
			"can only print a path. These wrappers do the `cd` for you:\n\n" +
			"  spo <name>   open a project and cd into it\n" +
			"  spn <name>   create a project and cd into it\n\n" +
			"Add to your profile:\n\n" +
			"  eval \"$(sp shell-init)\"          # bash and zsh\n" +
			"  sp shell-init fish | source      # fish\n" +
			"  sp shell-init powershell | Out-String | Invoke-Expression",
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
	case "powershell", "pwsh":
		return powershellShellInit, nil
	default:
		return "", fmt.Errorf("unsupported shell %q: use bash, zsh, fish or powershell", shell)
	}
}

// detectShell falls back to bash, whose syntax zsh also accepts, so an
// unrecognised $SHELL still produces something usable. Windows sets no $SHELL
// at all, and there PowerShell is the one shell certain to be present.
func detectShell() string {
	shell := os.Getenv("SHELL")
	base := filepath.Base(shell)
	switch {
	case strings.Contains(base, "fish"):
		return "fish"
	case strings.Contains(base, "zsh"):
		return "zsh"
	case strings.Contains(base, "pwsh"), strings.Contains(base, "powershell"):
		return "powershell"
	case shell == "" && runtime.GOOS == "windows":
		return "powershell"
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

const powershellShellInit = `# Scratchpad shell integration.
# Add to your profile with: sp shell-init powershell | Out-String | Invoke-Expression

# spo <name> — open a project and cd into it.
function spo {
    $dir = & sp open --print-path @args
    if ($LASTEXITCODE -ne 0) { return }
    # -LiteralPath so a project named with [brackets] is not read as a pattern.
    if ($dir) { Set-Location -LiteralPath ($dir | Select-Object -Last 1) }
}

# spn <name> — create a project and cd into it.
function spn {
    $dir = & sp new --path @args
    if ($LASTEXITCODE -ne 0) { return }
    if ($dir) { Set-Location -LiteralPath ($dir | Select-Object -Last 1) }
}`
