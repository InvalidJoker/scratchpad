// Package config loads and persists Scratchpad's user configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds every user-tunable setting. Directories are absolute paths with
// "~" already expanded.
type Config struct {
	ScratchDir string `toml:"scratch_dir"`
	// ProjectsDir is where `sp keep` promotes a project to.
	ProjectsDir string `toml:"projects_dir"`
	// ArchiveDir is where `sp archive` stores compressed projects.
	ArchiveDir string `toml:"archive_dir"`
	// TrashDir is the recovery area `sp trash` moves projects into.
	TrashDir string `toml:"trash_dir"`

	// DefaultExpiration is how long a new project lives before it is offered
	// up for cleanup. Zero means projects never expire.
	DefaultExpiration Duration `toml:"default_expiration"`
	// StaleAfter is how long without activity before a project reads as stale.
	StaleAfter Duration `toml:"stale_after"`

	InitGit bool `toml:"init_git"`
	// Editor opens projects; falls back to $VISUAL, then $EDITOR.
	Editor string `toml:"editor"`

	path string
	// exists records whether the config was read from disk, which is how
	// first run is detected.
	exists bool
}

// Duration is a time.Duration that round-trips through TOML as a string such
// as "30d" or "12h".
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d Duration) String() string {
	td := time.Duration(d)
	if td == 0 {
		return "0"
	}
	if td%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", td/(24*time.Hour))
	}
	return td.String()
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func (d *Duration) UnmarshalText(text []byte) error {
	td, err := ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(td)
	return nil
}

// ParseDuration extends time.ParseDuration with day and week units, so "30d"
// and "2w" work the way people expect them to in a config file or a flag.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" || s == "never" {
		return 0, nil
	}
	mult := time.Duration(0)
	switch {
	case strings.HasSuffix(s, "d"):
		mult = 24 * time.Hour
	case strings.HasSuffix(s, "w"):
		mult = 7 * 24 * time.Hour
	}
	if mult > 0 {
		var n float64
		if _, err := fmt.Sscanf(strings.TrimRight(s, "dw"), "%g", &n); err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n * float64(mult)), nil
	}
	td, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return td, nil
}

// Default returns the configuration used when no config file exists yet.
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ScratchDir:        filepath.Join(home, "Downloads", "Scratchpad"),
		ProjectsDir:       filepath.Join(home, "Projects"),
		ArchiveDir:        filepath.Join(home, "Archives", "Scratchpad"),
		TrashDir:          filepath.Join(home, "Downloads", "Scratchpad", ".trash"),
		DefaultExpiration: Duration(30 * 24 * time.Hour),
		StaleAfter:        Duration(14 * 24 * time.Hour),
		InitGit:           true,
	}
}

// DefaultPath is the config file location, honouring XDG_CONFIG_HOME.
func DefaultPath() (string, error) {
	if p := os.Getenv("SCRATCHPAD_CONFIG"); p != "" {
		return expand(p)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scratchpad", "config.toml"), nil
}

// Load reads the config at path, falling back to DefaultPath when path is
// empty. A missing file is not an error: defaults are returned instead.
func Load(path string) (*Config, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}

	cfg := Default()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, cfg.normalize()
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.exists = true
	return cfg, cfg.normalize()
}

func (c *Config) Path() string { return c.path }

// Exists reports whether a config file was found on disk. A false value means
// this is a first run and the values are defaults.
func (c *Config) Exists() bool { return c.exists }

// SetPath points the config at a different file, so a wizard can write where
// the user asked rather than where the config happened to be looked for.
func (c *Config) SetPath(path string) { c.path = path }

// Expand resolves ~ and environment variables in a user-supplied path.
func Expand(p string) (string, error) { return expand(p) }

// Save writes the config back to disk, creating parent directories.
func (c *Config) Save() error {
	if c.path == "" {
		p, err := DefaultPath()
		if err != nil {
			return err
		}
		c.path = p
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(c.path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), c.path); err != nil {
		return err
	}
	c.exists = true
	return nil
}

func (c *Config) EditorCommand() string {
	for _, v := range []string{c.Editor, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// normalize expands ~ and makes every directory absolute so the rest of the
// program never has to think about it.
func (c *Config) normalize() error {
	dirs := []*string{&c.ScratchDir, &c.ProjectsDir, &c.ArchiveDir, &c.TrashDir}
	for _, d := range dirs {
		v, err := expand(*d)
		if err != nil {
			return err
		}
		*d = v
	}
	if c.TrashDir == "" {
		c.TrashDir = filepath.Join(c.ScratchDir, ".trash")
	}
	return nil
}

func expand(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	return filepath.Abs(os.ExpandEnv(p))
}
