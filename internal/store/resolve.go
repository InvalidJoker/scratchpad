package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/project"
)

// ErrAmbiguous means a name matched more than one project.
var ErrAmbiguous = errors.New("ambiguous project name")

// AmbiguousError carries the candidates so the CLI can list them.
type AmbiguousError struct {
	Name       string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %s", e.Name, strings.Join(e.Candidates, ", "))
}

func (e *AmbiguousError) Is(target error) bool { return target == ErrAmbiguous }

// Resolve finds a project by exact name, then by prefix, then by substring.
// Anything short of a unique match is an error: guessing which project the
// user meant is not worth the risk when the next command may delete it.
func (s *Store) Resolve(name string, locs ...Location) (*project.Project, error) {
	if len(locs) == 0 {
		locs = []Location{Scratch, Kept, Trash}
	}

	var all []*project.Project
	for _, loc := range locs {
		found, err := s.List(loc)
		if err != nil {
			return nil, err
		}
		all = append(all, found...)
	}

	lower := strings.ToLower(name)
	var prefix, substring []*project.Project
	for _, p := range all {
		candidate := strings.ToLower(p.Name)
		switch {
		case candidate == lower:
			return p, nil
		case strings.HasPrefix(candidate, lower):
			prefix = append(prefix, p)
		case strings.Contains(candidate, lower):
			substring = append(substring, p)
		}
	}

	for _, matches := range [][]*project.Project{prefix, substring} {
		switch len(matches) {
		case 0:
			continue
		case 1:
			return matches[0], nil
		default:
			return nil, &AmbiguousError{Name: name, Candidates: namesOf(matches)}
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
}

func namesOf(ps []*project.Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	sort.Strings(out)
	return out
}

// Suggest returns project names for shell completion.
func (s *Store) Suggest(prefix string, locs ...Location) []string {
	if len(locs) == 0 {
		locs = []Location{Scratch, Kept}
	}
	var out []string
	for _, loc := range locs {
		found, err := s.List(loc)
		if err != nil {
			return nil
		}
		for _, p := range found {
			if prefix == "" || strings.HasPrefix(strings.ToLower(p.Name), strings.ToLower(prefix)) {
				out = append(out, p.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}
