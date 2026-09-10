package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Confirm asks a yes/no question. def is the answer for a bare Enter.
//
// This is deliberately a plain prompt rather than a full-screen one: it has to
// work over ssh, inside a pipe-fed terminal and in a subshell, and it must not
// repaint the transcript of what the user is deciding about.
func Confirm(in io.Reader, out io.Writer, question string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Fprintf(out, "%s %s ", question, Muted.Render(hint))

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		// EOF with no input (a closed stdin) means take the default rather
		// than fail; callers gate on TTY before asking at all.
		fmt.Fprintln(out)
		return def, nil
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
