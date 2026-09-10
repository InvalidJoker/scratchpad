package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/config"
)

func TestConfigSetAndGet(t *testing.T) {
	cfg := config.Default()

	tests := []struct {
		key, value, want string
	}{
		{"default_expiration", "14d", "14d"},
		{"stale_after", "2w", "14d"},
		{"default_expiration", "never", "0"},
		{"init_git", "false", "false"},
		{"init_git", "true", "true"},
		{"editor", "nvim", "nvim"},
	}
	for _, tc := range tests {
		if err := setConfigValue(cfg, tc.key, tc.value); err != nil {
			t.Errorf("set %s=%s: %v", tc.key, tc.value, err)
			continue
		}
		got, err := configValue(cfg, tc.key)
		if err != nil {
			t.Errorf("get %s: %v", tc.key, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestConfigSetExpandsPaths(t *testing.T) {
	cfg := config.Default()
	if err := setConfigValue(cfg, "scratch_dir", "~/scratch-test"); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(cfg.ScratchDir, "~") {
		t.Errorf("scratch_dir = %q, want ~ expanded", cfg.ScratchDir)
	}
}

func TestConfigRejectsBadValues(t *testing.T) {
	cfg := config.Default()

	if err := setConfigValue(cfg, "nonsense", "x"); err == nil {
		t.Error("setting an unknown key succeeded, want an error")
	}
	if err := setConfigValue(cfg, "init_git", "maybe"); err == nil {
		t.Error("init_git = maybe succeeded, want an error")
	}
	if err := setConfigValue(cfg, "default_expiration", "tomorrow"); err == nil {
		t.Error("an unparseable duration succeeded, want an error")
	}

	// A rejected value must not have been half-applied.
	if cfg.DefaultExpiration.Duration() != 30*24*time.Hour {
		t.Errorf("DefaultExpiration = %v, want the default left intact", cfg.DefaultExpiration.Duration())
	}
}

func TestUnknownKeyErrorListsValidKeys(t *testing.T) {
	err := unknownKey("bogus")
	for _, key := range []string{"scratch_dir", "init_git", "stale_after"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error message does not mention %q: %v", key, err)
		}
	}
}
