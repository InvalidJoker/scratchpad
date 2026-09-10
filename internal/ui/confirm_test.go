package ui_test

import (
	"io"
	"strings"
	"testing"

	"github.com/InvalidJoker/scratchpad/internal/ui"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		input string
		def   bool
		want  bool
	}{
		{"y\n", false, true},
		{"Y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"no\n", true, false},
		{"\n", true, true},
		{"\n", false, false},
		{"  \n", true, true},
		{"garbage\n", true, false},
		{"", false, false},
		{"", true, true},
	}

	for _, tc := range tests {
		got, err := ui.Confirm(strings.NewReader(tc.input), io.Discard, "Delete?", tc.def)
		if err != nil {
			t.Errorf("Confirm(%q): %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Confirm(%q, default=%v) = %v, want %v", tc.input, tc.def, got, tc.want)
		}
	}
}

func TestConfirmShowsDefaultInHint(t *testing.T) {
	var out strings.Builder
	if _, err := ui.Confirm(strings.NewReader("\n"), &out, "Delete?", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Y/n") {
		t.Errorf("prompt = %q, want it to show Y/n for a default-yes question", out.String())
	}
}
