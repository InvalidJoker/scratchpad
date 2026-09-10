package ui

import (
	"github.com/charmbracelet/huh"
	lipglossv1 "github.com/charmbracelet/lipgloss"
)

// FormTheme styles huh forms to match the rest of Scratchpad.
//
// It builds on lipgloss v1 because that is what huh renders with. The palette
// is duplicated here rather than shared with styles.go: the two lipgloss
// majors have incompatible colour types, and one small duplicated palette is
// cheaper than a conversion layer.
func FormTheme() *huh.Theme {
	t := huh.ThemeBase()

	var (
		accent = lipglossv1.AdaptiveColor{Light: "#5A3FD6", Dark: "#A78BFA"}
		muted  = lipglossv1.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
		ok     = lipglossv1.AdaptiveColor{Light: "#047857", Dark: "#34D399"}
		danger = lipglossv1.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	)

	t.Focused.Base = t.Focused.Base.BorderForeground(accent)
	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(accent).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(muted)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(accent)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(accent).Foreground(lipglossv1.Color("#FFFFFF"))
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(danger)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(danger)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(ok)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipglossv1.HiddenBorder())
	t.Blurred.Title = t.Blurred.Title.Foreground(muted).Bold(false)
	t.Blurred.NoteTitle = t.Blurred.NoteTitle.Foreground(muted).Bold(false)

	return t
}
