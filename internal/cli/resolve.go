package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

// resolve finds a project by name and turns store errors into messages that
// tell the user what to do next.
func (a *App) resolve(name string, locs ...store.Location) (*project.Project, error) {
	p, err := a.store.Resolve(name, locs...)
	if err == nil {
		return p, nil
	}

	var ambiguous *store.AmbiguousError
	if errors.As(err, &ambiguous) {
		return nil, fmt.Errorf("%q matches %d projects: %s",
			name, len(ambiguous.Candidates), strings.Join(ambiguous.Candidates, ", "))
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("no project named %q (try `sp list`)", name)
	}
	return nil, err
}

// completeProjects powers shell completion of project name arguments.
func (a *App) completeProjects(locs ...store.Location) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 || a.store == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return a.store.Suggest(toComplete, locs...), cobra.ShellCompDirectiveNoFileComp
	}
}

// confirm asks the user to approve a destructive action. When stdin is not a
// terminal there is nobody to ask, so the action is refused unless it was
// pre-approved with --yes.
func (a *App) confirm(question string, def bool) (bool, error) {
	if !a.interactive() {
		return false, errors.New("cannot ask for confirmation without a terminal: re-run with --yes")
	}
	return ui.Confirm(a.in, a.out, question, def)
}
