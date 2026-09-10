// Package ui holds the shared presentation layer: colours, styles and the
// small formatting helpers every command prints through.
package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Palette. Adaptive colours keep the output readable on light and dark
// terminals alike.
var (
	colorAccent = compat.AdaptiveColor{Light: lipgloss.Color("#5A3FD6"), Dark: lipgloss.Color("#A78BFA")}
	colorMuted  = compat.AdaptiveColor{Light: lipgloss.Color("#6B7280"), Dark: lipgloss.Color("#9CA3AF")}
	colorOK     = compat.AdaptiveColor{Light: lipgloss.Color("#047857"), Dark: lipgloss.Color("#34D399")}
	colorWarn   = compat.AdaptiveColor{Light: lipgloss.Color("#B45309"), Dark: lipgloss.Color("#FBBF24")}
	colorErr    = compat.AdaptiveColor{Light: lipgloss.Color("#B91C1C"), Dark: lipgloss.Color("#F87171")}
)

var (
	Accent  = lipgloss.NewStyle().Foreground(colorAccent)
	Muted   = lipgloss.NewStyle().Foreground(colorMuted)
	Success = lipgloss.NewStyle().Foreground(colorOK)
	Warning = lipgloss.NewStyle().Foreground(colorWarn)
	Danger  = lipgloss.NewStyle().Foreground(colorErr)
	Bold    = lipgloss.NewStyle().Bold(true)
	Label   = lipgloss.NewStyle().Foreground(colorMuted).Width(12)
	Header  = lipgloss.NewStyle().Foreground(colorMuted).Bold(true)
)

// Check renders a success line, e.g. "✓ Created weather-app".
func Check(format string, args ...any) string {
	return Success.Render("✓") + " " + fmt.Sprintf(format, args...)
}

func Warn(format string, args ...any) string {
	return Warning.Render("!") + " " + fmt.Sprintf(format, args...)
}

// Field renders an aligned "label  value" pair.
func Field(label, value string) string {
	return Label.Render(label) + value
}

// RelativeTime renders a timestamp the way a human would say it: "2 hours
// ago", "yesterday", "3 weeks ago".
func RelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 48*time.Hour:
		return "yesterday"
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day") + " ago"
	case d < 365*24*time.Hour:
		return plural(int(d.Hours()/24/30), "month") + " ago"
	default:
		return plural(int(d.Hours()/24/365), "year") + " ago"
	}
}

// RelativeFuture renders a deadline as "in 14 days", or "expired" if passed.
func RelativeFuture(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := t.Sub(now)
	if d <= 0 {
		return "expired"
	}
	// Round rather than truncate: a deadline 29.99 days out reads as "in 30
	// days", which is what the user asked for.
	switch {
	case d < time.Hour:
		return "in " + plural(round(d.Minutes()), "minute")
	case d < 48*time.Hour:
		return "in " + plural(round(d.Hours()), "hour")
	default:
		return "in " + plural(round(d.Hours()/24), "day")
	}
}

// Duration renders a span compactly for table columns: "3d", "18d", "2h".
func Duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// round converts a fractional unit count to the nearest whole one, with a
// floor of 1 so a near-term deadline never reads as "in 0 days".
func round(v float64) int {
	if n := int(math.Round(v)); n > 0 {
		return n
	}
	return 1
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// Tildify shortens a path under the user's home directory to "~/...", which
// is how people actually read paths.
func Tildify(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~" + string(filepath.Separator) + rel
	}
	return path
}
