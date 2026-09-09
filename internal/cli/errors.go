package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/x/term"
)

// ErrorHandler renders errors in fang's house style, minus its title-casing
// of the first word. That transform mangles the identifiers our messages lead
// with: `--ttl` becomes `--Ttl` and "weather-app" becomes "Weather-App".
func ErrorHandler(w io.Writer, styles fang.Styles, err error) {
	if f, ok := w.(term.File); ok && !term.IsTerminal(f.Fd()) {
		fmt.Fprintln(w, err.Error())
		return
	}

	text := styles.ErrorText.UnsetTransform()
	fmt.Fprintln(w, styles.ErrorHeader.String())
	fmt.Fprintln(w, text.Render(punctuate(err.Error())))
	fmt.Fprintln(w)

	if isUsageError(err) {
		fmt.Fprintln(w, lipgloss.JoinHorizontal(
			lipgloss.Left,
			text.UnsetWidth().Render("Try"),
			styles.Program.Flag.Render(" --help "),
			text.UnsetWidth().UnsetMargins().Render("for usage."),
		))
		fmt.Fprintln(w)
	}
}

// punctuate ends the message with a full stop unless it already closes with
// punctuation of its own.
func punctuate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if r := rune(s[len(s)-1]); unicode.IsPunct(r) && r != '"' && r != '\'' && r != ')' {
		return s
	}
	return s + "."
}

// usagePrefixes are the cobra errors that mean the user got the invocation
// wrong, rather than the command failing. Cobra does not type these, so
// matching on the message is the only option available.
var usagePrefixes = []string{
	"flag needs an argument:",
	"unknown flag:",
	"unknown shorthand flag:",
	"unknown command",
	"invalid argument",
	"accepts ",
	"requires at least",
	"unknown help topic",
}

func isUsageError(err error) bool {
	s := err.Error()
	for _, p := range usagePrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
