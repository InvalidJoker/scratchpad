// Package store is the persistence layer for scratch projects: it owns the
// scratch, trash and archive directories and every read/write of a project's
// metadata file.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/project"
)

// Sentinel errors callers are expected to branch on.
var (
	// ErrNotFound means no project with that name exists in the searched
	// locations.
	ErrNotFound = errors.New("project not found")
	// ErrExists means a directory already occupies the target path.
	ErrExists = errors.New("project already exists")
	// ErrNotManaged means a directory exists but has no Scratchpad metadata.
	ErrNotManaged = errors.New("directory is not a scratchpad project")
)

// Location is one of the directories the store manages.
type Location string

const (
	// Scratch is the working directory for temporary projects.
	Scratch Location = "scratch"
	// Trash is the recovery area.
	Trash Location = "trash"
	// Kept is the permanent projects directory.
	Kept Location = "kept"
	// Archive holds compressed projects.
	Archive Location = "archive"
)

// Store reads and writes projects across the configured directories.
type Store struct {
	cfg *config.Config
	// now is the clock, swappable in tests.
	now func() time.Time
}

func New(cfg *config.Config) *Store {
	return &Store{cfg: cfg, now: time.Now}
}

func (s *Store) Config() *config.Config { return s.cfg }

func (s *Store) Now() time.Time { return s.now() }

// SetClock overrides the store's clock. Intended for tests.
func (s *Store) SetClock(fn func() time.Time) { s.now = fn }

func (s *Store) Dir(loc Location) string {
	switch loc {
	case Trash:
		return s.cfg.TrashDir
	case Kept:
		return s.cfg.ProjectsDir
	case Archive:
		return s.cfg.ArchiveDir
	default:
		return s.cfg.ScratchDir
	}
}

// PathFor returns where a project of the given name would live in a location.
func (s *Store) PathFor(loc Location, name string) string {
	return filepath.Join(s.Dir(loc), name)
}

type CreateOptions struct {
	Name        string
	Description string
	Tags        []string
	// TTL overrides the configured default expiration. Use TTLSet to
	// distinguish "no override" from "never expires".
	TTL    time.Duration
	TTLSet bool
	// Permanent creates the project already promoted: no expiry, kept state.
	Permanent bool
}

// Create makes a new project directory with metadata and returns it. The
// project lands in the scratch directory, or in the permanent projects
// directory when opts.Permanent is set. It fails if anything already exists
// at the target path.
func (s *Store) Create(opts CreateOptions) (*project.Project, error) {
	if err := project.ValidateName(opts.Name); err != nil {
		return nil, err
	}

	loc := Scratch
	if opts.Permanent {
		loc = Kept
	}
	dir := s.PathFor(loc, opts.Name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrExists, dir)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	now := s.now()
	p := &project.Project{
		Name:        opts.Name,
		Description: opts.Description,
		Tags:        opts.Tags,
		State:       project.StateActive,
		Created:     now,
		LastOpened:  now,
	}
	if opts.Permanent {
		p.State = project.StateKept
	} else {
		ttl := s.cfg.DefaultExpiration.Duration()
		if opts.TTLSet {
			ttl = opts.TTL
		}
		if ttl > 0 {
			p.ExpiresAt = now.Add(ttl)
		}
	}
	p.SetDir(dir)

	if err := os.MkdirAll(filepath.Join(dir, project.MetaDir), 0o755); err != nil {
		return nil, fmt.Errorf("create project directory: %w", err)
	}
	if err := s.Save(p); err != nil {
		// Roll back the half-created directory so a failed create leaves no
		// unmanaged litter behind.
		os.RemoveAll(dir)
		return nil, err
	}
	return p, nil
}

// Save writes a project's metadata atomically.
func (s *Store) Save(p *project.Project) error {
	if p.Dir() == "" {
		return fmt.Errorf("project %q has no directory", p.Name)
	}
	metaDir := filepath.Join(p.Dir(), project.MetaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(metaDir, ".metadata-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p.MetaPath())
}

func (s *Store) LoadDir(dir string) (*project.Project, error) {
	metaPath := filepath.Join(dir, project.MetaDir, project.MetaFile)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotManaged, dir)
		}
		return nil, err
	}

	var p project.Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", metaPath, err)
	}
	p.SetDir(dir)
	// The directory name is authoritative: a project renamed on disk should
	// still resolve by its new name.
	if base := filepath.Base(dir); base != p.Name {
		p.Name = base
	}
	if p.State == "" {
		p.State = project.StateActive
	}
	return &p, nil
}

// Get finds a project by name, searching the given locations in order. With no
// locations it searches scratch, then trash.
func (s *Store) Get(name string, locs ...Location) (*project.Project, error) {
	if len(locs) == 0 {
		locs = []Location{Scratch, Trash}
	}
	for _, loc := range locs {
		p, err := s.LoadDir(s.PathFor(loc, name))
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ErrNotManaged) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
}

// List returns every managed project in a location, sorted by most recent
// activity first. A missing directory yields an empty list, not an error.
func (s *Store) List(loc Location) ([]*project.Project, error) {
	dir := s.Dir(loc)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var projects []*project.Project
	for _, e := range entries {
		if !isProjectDir(dir, e) {
			continue
		}
		p, err := s.LoadDir(filepath.Join(dir, e.Name()))
		if err != nil {
			// Unmanaged directories are none of our business; anything else
			// is a real problem worth surfacing.
			if errors.Is(err, ErrNotManaged) {
				continue
			}
			return nil, err
		}
		projects = append(projects, p)
	}

	sort.Slice(projects, func(i, j int) bool {
		a, b := projects[i].LastActivity(), projects[j].LastActivity()
		if a.Equal(b) {
			return projects[i].Name < projects[j].Name
		}
		return a.After(b)
	})
	return projects, nil
}

// isProjectDir reports whether an entry could hold a project, following
// symlinks so a linked-in project directory still counts.
func isProjectDir(parent string, e fs.DirEntry) bool {
	name := e.Name()
	if name == "" || name[0] == '.' {
		return false
	}
	if e.IsDir() {
		return true
	}
	if e.Type()&fs.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(parent, name))
	return err == nil && info.IsDir()
}

func (s *Store) EnsureDirs() error {
	for _, dir := range []string{s.cfg.ScratchDir, s.cfg.TrashDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}
