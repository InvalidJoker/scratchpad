package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/store"
)

func resolveFixture(t *testing.T) *store.Store {
	t.Helper()
	s := testStore(t, time.Now())
	for _, name := range []string{"weather-app", "api-server", "api-client", "notes"} {
		if _, err := s.Create(store.CreateOptions{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestResolveExactBeatsPrefix(t *testing.T) {
	s := testStore(t, time.Now())
	for _, name := range []string{"api", "api-server"} {
		if _, err := s.Create(store.CreateOptions{Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := s.Resolve("api")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name != "api" {
		t.Errorf("Name = %q, want the exact match %q", p.Name, "api")
	}
}

func TestResolveByPrefixAndSubstring(t *testing.T) {
	s := resolveFixture(t)

	tests := []struct{ query, want string }{
		{"weath", "weather-app"},
		{"WEATHER-APP", "weather-app"},
		{"server", "api-server"},
		{"notes", "notes"},
	}
	for _, tc := range tests {
		p, err := s.Resolve(tc.query)
		if err != nil {
			t.Errorf("Resolve(%q): %v", tc.query, err)
			continue
		}
		if p.Name != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.query, p.Name, tc.want)
		}
	}
}

func TestResolveAmbiguousRefusesToGuess(t *testing.T) {
	s := resolveFixture(t)

	_, err := s.Resolve("api")
	if !errors.Is(err, store.ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}

	var ambiguous *store.AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want an *AmbiguousError", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("candidates = %v, want both api projects", ambiguous.Candidates)
	}
	if ambiguous.Candidates[0] != "api-client" {
		t.Errorf("candidates = %v, want them sorted for a stable message", ambiguous.Candidates)
	}
}

func TestResolveNotFound(t *testing.T) {
	s := resolveFixture(t)
	if _, err := s.Resolve("nothing-like-this"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveHonoursLocationScope(t *testing.T) {
	s := testStore(t, time.Now())
	p, err := s.Create(store.CreateOptions{Name: "gone"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(p); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Resolve("gone", store.Scratch); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want a trashed project to be invisible when scoped to scratch", err)
	}
	if _, err := s.Resolve("gone", store.Trash); err != nil {
		t.Errorf("Resolve in trash: %v", err)
	}
}

func TestSuggestFiltersByPrefix(t *testing.T) {
	s := resolveFixture(t)

	got := s.Suggest("api", store.Scratch)
	if len(got) != 2 || got[0] != "api-client" || got[1] != "api-server" {
		t.Errorf("Suggest(api) = %v, want both api projects sorted", got)
	}
	if all := s.Suggest("", store.Scratch); len(all) != 4 {
		t.Errorf("Suggest() = %v, want all four", all)
	}
}
