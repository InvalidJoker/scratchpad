package ui

import (
	"errors"

	"github.com/charmbracelet/huh"
)

// ErrCancelled means the user backed out of a prompt with Esc or Ctrl-C. It is
// not a failure: callers report "cancelled" and exit cleanly.
var ErrCancelled = errors.New("cancelled")

// Confirm asks a yes/no question. def is the highlighted answer.
func Confirm(question, affirmative, negative string, def bool) (bool, error) {
	answer := def
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(question).
				Affirmative(affirmative).
				Negative(negative).
				Value(&answer),
		),
	).WithTheme(FormTheme()).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, ErrCancelled
		}
		return false, err
	}
	return answer, nil
}
