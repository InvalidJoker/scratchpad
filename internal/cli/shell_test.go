package cli

import (
	"strings"
	"testing"
)

func TestShellInitScript(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
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
		// The wrappers exist to cd, which is the one thing the binary cannot
		// do for its parent shell.
		if !strings.Contains(script, "cd ") {
			t.Errorf("%s script never calls cd", shell)
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
