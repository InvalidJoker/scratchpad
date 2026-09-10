package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/InvalidJoker/scratchpad/internal/project"
)

// StatusBadge renders a status as its icon plus a coloured label.
func StatusBadge(s project.Status) string {
	return s.Icon() + " " + statusStyle(s).Render(title(string(s)))
}

func statusStyle(s project.Status) lipgloss.Style {
	switch s {
	case project.StatusActive:
		return Success
	case project.StatusStale:
		return Warning
	case project.StatusExpired, project.StatusTrashed:
		return Danger
	case project.StatusKept:
		return Accent
	default:
		return Muted
	}
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
