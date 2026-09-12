package cli

import (
	"strings"
	"testing"
)

func TestShellInitScript(t *testing.T) {
	// Every shell gets the same two wrappers; only the call that changes
	// directory — the one thing the binary cannot do for its parent — differs.
	changeDir := map[string]string{
		"bash":       "cd ",
		"zsh":        "cd ",
		"fish":       "cd ",
		"powershell": "Set-Location",
	}

	for shell, cd := range changeDir {
		script, err := shellInitScript(shell)
		if err != nil {
			t.Errorf("shellInitScript(%q): %v", shell, err)
			continue
		}
		for _, fn := range []string{"spo", "spn"} {
			if !strings.Contains(script, fn) {
				t.Errorf("%s script is missing the %s wrapper", shell, fn)
			}
		}
		if !strings.Contains(script, cd) {
			t.Errorf("%s script never calls %s", shell, cd)
		}
	}

	if _, err := shellInitScript("nushell"); err == nil {
		t.Error("shellInitScript(nushell) = nil error, want an unsupported-shell error")
	}
}

func TestShellInitFishUsesFishSyntax(t *testing.T) {
	script, err := shellInitScript("fish")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "function spo") || !strings.Contains(script, "end") {
		t.Error("fish script does not use fish function syntax")
	}
	if strings.Contains(script, "local dir") {
		t.Error("fish script contains bash-only syntax")
	}
}

func TestShellInitPowerShellUsesPowerShellSyntax(t *testing.T) {
	script, err := shellInitScript("powershell")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "function spo {") {
		t.Error("powershell script does not use powershell function syntax")
	}
	// A POSIX $(...) substitution in a PowerShell profile is a silent no-op:
	// the function would be defined but never cd anywhere.
	for _, posixism := range []string{"local dir", "$(sp ", "; and "} {
		if strings.Contains(script, posixism) {
			t.Errorf("powershell script contains POSIX-only syntax %q", posixism)
		}
	}
	// pwsh is the same shell under a different name.
	alias, err := shellInitScript("pwsh")
	if err != nil {
		t.Fatalf("shellInitScript(pwsh): %v", err)
	}
	if alias != script {
		t.Error("pwsh and powershell produced different scripts")
	}
}

func TestDetectShellFromEnv(t *testing.T) {
	for _, tc := range []struct{ shell, want string }{
		{"/bin/bash", "bash"},
		{"/bin/zsh", "zsh"},
		{"/opt/homebrew/bin/fish", "fish"},
		{"/usr/local/bin/pwsh", "powershell"},
		{"/usr/bin/nonsense", "bash"},
	} {
		t.Setenv("SHELL", tc.shell)
		if got := detectShell(); got != tc.want {
			t.Errorf("detectShell() with SHELL=%q = %q, want %q", tc.shell, got, tc.want)
		}
	}
}

func TestShellInitNeverTriggersSetup(t *testing.T) {
	// A profile evaluates `sp shell-init` on every new shell, with stdin still
	// a terminal. If the wizard could open there, its prompts would be captured
	// into the script the shell is about to evaluate.
	root := NewRootCommand("test")
	for _, name := range []string{"shell-init", "setup"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("root.Find(%q): %v", name, err)
		}
		if wantsSetup(cmd) {
			t.Errorf("wantsSetup(%q) = true, want false", name)
		}
	}

	cmd, _, err := root.Find([]string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	if !wantsSetup(cmd) {
		t.Error("wantsSetup(list) = false, want true: an unconfigured list has nothing to list")
	}
}
