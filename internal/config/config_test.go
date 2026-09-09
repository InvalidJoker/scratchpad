package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/config"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"1.5d", 36 * time.Hour},
		{"never", 0},
		{"0", 0},
		{"", 0},
	}
	for _, tc := range tests {
		got, err := config.ParseDuration(tc.in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	for _, in := range []string{"tomorrow", "5x", "d"} {
		if _, err := config.ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) = nil error, want failure", in)
		}
	}
}

func TestDurationRoundTrip(t *testing.T) {
	d := config.Duration(30 * 24 * time.Hour)
	if got := d.String(); got != "30d" {
		t.Errorf("String = %q, want %q", got, "30d")
	}

	var back config.Duration
	if err := back.UnmarshalText([]byte(d.String())); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if back != d {
		t.Errorf("round trip = %v, want %v", back.Duration(), d.Duration())
	}
}

func TestLoadMissingFileYieldsDefaults(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ScratchDir == "" || cfg.TrashDir == "" {
		t.Errorf("defaults not populated: %+v", cfg)
	}
	if cfg.DefaultExpiration.Duration() != 30*24*time.Hour {
		t.Errorf("DefaultExpiration = %v, want 30d", cfg.DefaultExpiration.Duration())
	}
}

func TestLoadExpandsTildeAndSaveRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.ScratchDir = "~/scratch-test"
	cfg.StaleAfter = config.Duration(7 * 24 * time.Hour)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if filepath.IsAbs(reloaded.ScratchDir) == false {
		t.Errorf("ScratchDir = %q, want an absolute expanded path", reloaded.ScratchDir)
	}
	if reloaded.StaleAfter.Duration() != 7*24*time.Hour {
		t.Errorf("StaleAfter = %v, want 7d", reloaded.StaleAfter.Duration())
	}
}
