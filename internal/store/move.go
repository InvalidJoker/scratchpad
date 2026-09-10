package store

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/project"
)

// ErrOccupied means the destination path already exists, so moving there would
// clobber something.
var ErrOccupied = errors.New("destination already exists")

// Keep promotes a project into the permanent projects directory. dest
// overrides the configured parent directory when it is non-empty.
func (s *Store) Keep(p *project.Project, dest string) error {
	if p.State == project.StateKept {
		return fmt.Errorf("%s is already kept", p.Name)
	}
	parent := dest
	if parent == "" {
		parent = s.Dir(Kept)
	}
	target := filepath.Join(parent, p.Name)

	origin := p.Dir()
	if err := s.relocate(p, target); err != nil {
		return err
	}
	p.State = project.StateKept
	p.ExpiresAt = time.Time{}
	p.TrashedAt = time.Time{}
	p.OriginalPath = origin
	return s.saveOrUnwind(p, origin, target)
}

// Trash moves a project into the recovery area. Deletion is never immediate:
// see Destroy for the path that actually removes bytes.
func (s *Store) Trash(p *project.Project) error {
	if p.State == project.StateTrashed {
		return fmt.Errorf("%s is already in the trash", p.Name)
	}
	target, err := s.freeTrashPath(p.Name)
	if err != nil {
		return err
	}

	origin := p.Dir()
	if err := s.relocate(p, target); err != nil {
		return err
	}
	p.State = project.StateTrashed
	p.TrashedAt = s.now()
	p.OriginalPath = origin
	return s.saveOrUnwind(p, origin, target)
}

// Restore returns a trashed project to where it came from, or to the scratch
// directory when its original location is gone or taken.
func (s *Store) Restore(p *project.Project) error {
	if p.State != project.StateTrashed {
		return fmt.Errorf("%s is not in the trash", p.Name)
	}

	target := p.OriginalPath
	if target == "" || exists(target) {
		target = s.PathFor(Scratch, originalName(p))
	}
	if exists(target) {
		return fmt.Errorf("%w: %s", ErrOccupied, target)
	}

	origin := p.Dir()
	if err := s.relocate(p, target); err != nil {
		return err
	}
	p.State = project.StateActive
	p.TrashedAt = time.Time{}
	p.OriginalPath = ""
	// A restored project gets a fresh window rather than arriving expired.
	if ttl := s.cfg.DefaultExpiration.Duration(); ttl > 0 {
		p.ExpiresAt = s.now().Add(ttl)
	}
	p.RecordActivity(s.now())
	return s.saveOrUnwind(p, origin, target)
}

// Destroy permanently removes a project. There is no recovery after this.
func (s *Store) Destroy(p *project.Project) error {
	if p.Dir() == "" {
		return fmt.Errorf("project %q has no directory", p.Name)
	}
	// Refuse to remove anything that is not recognisably a project, so a
	// corrupt path can never turn into `rm -rf` on something else.
	if !exists(filepath.Join(p.Dir(), project.MetaDir)) {
		return fmt.Errorf("%w: %s", ErrNotManaged, p.Dir())
	}
	return os.RemoveAll(p.Dir())
}

// relocate moves the project directory and updates the in-memory path. It does
// not touch metadata; callers do that and then save.
func (s *Store) relocate(p *project.Project, target string) error {
	if p.Dir() == "" {
		return fmt.Errorf("project %q has no directory", p.Name)
	}
	if exists(target) {
		return fmt.Errorf("%w: %s", ErrOccupied, target)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := movePath(p.Dir(), target); err != nil {
		return err
	}
	p.SetDir(target)
	return nil
}

// saveOrUnwind writes the metadata for a project that has just been moved. If
// the write fails the move is undone, so a failure leaves the project where it
// started rather than in the new location with stale metadata.
func (s *Store) saveOrUnwind(p *project.Project, origin, target string) error {
	if err := s.Save(p); err != nil {
		if undo := movePath(target, origin); undo == nil {
			p.SetDir(origin)
		}
		return err
	}
	return nil
}

// freeTrashPath finds an unused path in the trash, disambiguating a repeat
// deletion with a timestamp rather than refusing it.
func (s *Store) freeTrashPath(name string) (string, error) {
	base := s.PathFor(Trash, name)
	if !exists(base) {
		return base, nil
	}
	stamped := fmt.Sprintf("%s-%s", base, s.now().Format("20060102-150405"))
	if !exists(stamped) {
		return stamped, nil
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", stamped, i)
		if !exists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrOccupied, base)
}

// originalName recovers the pre-trash name, which differs from the directory
// name when a collision forced a timestamp suffix.
func originalName(p *project.Project) string {
	if p.OriginalPath != "" {
		return filepath.Base(p.OriginalPath)
	}
	return p.Name
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// movePath renames src to dst, falling back to copy-and-delete when the two
// live on different filesystems (scratch on an external drive, projects on the
// internal one).
func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("move %s: %w", filepath.Base(src), err)
	}
	if err := copyTree(src, dst); err != nil {
		os.RemoveAll(dst)
		return fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("remove %s after copy: %w", filepath.Base(src), err)
	}
	return nil
}

func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, errCrossDevice)
	}
	return false
}

// copyTree copies a directory recursively, preserving permissions, modification
// times and symlinks. Modification times matter: they are the raw material for
// staleness detection.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			// Directory times are restored on the way out, since writing
			// children updates them again.
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case !info.Mode().IsRegular():
			// Sockets, devices and pipes have no meaningful copy; skipping is
			// better than failing the whole move.
			return nil
		default:
			if err := copyFile(path, target, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chtimes(target, time.Time{}, info.ModTime())
		}
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
