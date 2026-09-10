package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/InvalidJoker/scratchpad/internal/config"
	"github.com/InvalidJoker/scratchpad/internal/project"
	"github.com/InvalidJoker/scratchpad/internal/store"
	"github.com/InvalidJoker/scratchpad/internal/ui"
	"github.com/spf13/cobra"
)

type listOptions struct {
	all       bool
	active    bool
	stale     bool
	expired   bool
	kept      bool
	trashed   bool
	tag       string
	query     string
	olderThan string
	sortKey   string
	reverse   bool
	asJSON    bool
	quiet     bool
}

func newListCommand(app *App) *cobra.Command {
	var opts listOptions

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "l"},
		Short:   "List your scratch projects",
		Long: "List scratch projects, newest activity first.\n\n" +
			"By default this shows the scratch directory only. Use --all to\n" +
			"include kept and trashed projects.",
		Args: cobra.NoArgs,
		Example: "  sp list\n" +
			"  sp list --stale --older-than 30d\n" +
			"  sp list --tag experiment --sort name\n" +
			"  sp list -q | xargs -n1 sp info",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runList(opts)
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&opts.all, "all", "a", false, "include kept and trashed projects")
	f.BoolVar(&opts.active, "active", false, "only active projects")
	f.BoolVar(&opts.stale, "stale", false, "only projects past the staleness window")
	f.BoolVar(&opts.expired, "expired", false, "only projects past their expiry date")
	f.BoolVar(&opts.kept, "kept", false, "only permanent projects")
	f.BoolVar(&opts.trashed, "trashed", false, "only projects in the trash")
	f.StringVarP(&opts.tag, "tag", "t", "", "only projects with this tag")
	f.StringVar(&opts.query, "query", "", "match name, description, note or tags")
	f.StringVar(&opts.olderThan, "older-than", "", "only projects untouched for longer than this, e.g. 30d")
	f.StringVar(&opts.sortKey, "sort", "used", "sort by used, name or age")
	f.BoolVar(&opts.reverse, "reverse", false, "reverse the sort order")
	f.BoolVar(&opts.asJSON, "json", false, "output JSON")
	f.BoolVarP(&opts.quiet, "quiet", "q", false, "print names only, one per line")

	return cmd
}

func (a *App) runList(opts listOptions) error {
	filter, err := opts.filter()
	if err != nil {
		return err
	}

	projects, err := a.store.Query(filter)
	if err != nil {
		return err
	}

	switch {
	case opts.asJSON:
		return a.printJSON(projects)
	case opts.quiet:
		for _, p := range projects {
			a.println(p.Name)
		}
		return nil
	}

	if len(projects) == 0 {
		a.printEmptyState(opts)
		return nil
	}
	a.printTable(projects)
	return nil
}

// filter translates the flags into a store.Filter, resolving which locations
// need to be scanned from the statuses the user asked for.
func (o listOptions) filter() (store.Filter, error) {
	f := store.Filter{Reverse: o.reverse}

	switch store.SortKey(o.sortKey) {
	case store.SortActivity, store.SortName, store.SortAge:
		f.Sort = store.SortKey(o.sortKey)
	default:
		return f, fmt.Errorf("unknown sort %q: use used, name or age", o.sortKey)
	}

	if o.olderThan != "" {
		d, err := config.ParseDuration(o.olderThan)
		if err != nil {
			return f, err
		}
		f.IdleFor = d
	}

	f.Tag = o.tag
	f.Query = o.query

	locations := map[store.Location]bool{}
	addStatus := func(s project.Status, loc store.Location) {
		f.Statuses = append(f.Statuses, s)
		locations[loc] = true
	}
	if o.active {
		addStatus(project.StatusActive, store.Scratch)
	}
	if o.stale {
		addStatus(project.StatusStale, store.Scratch)
	}
	if o.expired {
		addStatus(project.StatusExpired, store.Scratch)
	}
	if o.kept {
		addStatus(project.StatusKept, store.Kept)
	}
	if o.trashed {
		addStatus(project.StatusTrashed, store.Trash)
	}

	switch {
	case len(locations) > 0:
		for loc := range locations {
			f.Locations = append(f.Locations, loc)
		}
	case o.all:
		f.Locations = []store.Location{store.Scratch, store.Kept, store.Trash}
	default:
		f.Locations = []store.Location{store.Scratch}
	}
	return f, nil
}

func (a *App) printTable(projects []*project.Project) {
	now := a.store.Now()

	table := ui.NewTable("name", "last used", "age", "status")
	for _, p := range projects {
		table.Row(
			ui.Bold.Render(p.Name),
			ui.RelativeTime(p.LastActivity(), now),
			ui.Duration(p.Age(now)),
			ui.StatusBadge(a.store.StatusOf(p)),
		)
	}

	a.println("", table.Render(), "")
	a.println(ui.Muted.Render(a.summary(projects)))
}

// summary renders the trailing "7 projects · 3 active · 2 stale" line.
func (a *App) summary(projects []*project.Project) string {
	counts := a.store.Counts(projects)
	parts := []string{plural(len(projects), "project")}
	// Fixed order so the summary does not shuffle between runs.
	for _, s := range []project.Status{
		project.StatusActive, project.StatusStale, project.StatusExpired,
		project.StatusKept, project.StatusArchived, project.StatusTrashed,
	} {
		if n := counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	return strings.Join(parts, " · ")
}

func (a *App) printEmptyState(opts listOptions) {
	if opts.filtered() {
		a.println("", ui.Muted.Render("No projects match that filter."), "")
		return
	}
	a.println("",
		ui.Muted.Render("No scratch projects yet."),
		"",
		"Start one with "+ui.Accent.Render("sp new <name>")+".",
		"")
}

// filtered reports whether the user narrowed the listing, which changes what
// an empty result means.
func (o listOptions) filtered() bool {
	return o.active || o.stale || o.expired || o.kept || o.trashed ||
		o.tag != "" || o.query != "" || o.olderThan != ""
}

// jsonProject is the stable shape of `--json` output. It exists so the wire
// format does not drift every time the internal model changes.
type jsonProject struct {
	Name        string     `json:"name"`
	Path        string     `json:"path"`
	Description string     `json:"description,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	State       string     `json:"state"`
	Status      string     `json:"status"`
	Created     time.Time  `json:"created"`
	LastUsed    time.Time  `json:"last_used"`
	ExpiresAt   *time.Time `json:"expires_at"`
	AgeDays     int        `json:"age_days"`
	IdleDays    int        `json:"idle_days"`
	OpenCount   int        `json:"open_count"`
}

func (a *App) printJSON(projects []*project.Project) error {
	now := a.store.Now()
	out := make([]jsonProject, 0, len(projects))
	for _, p := range projects {
		item := jsonProject{
			Name:        p.Name,
			Path:        p.Dir(),
			Description: p.Description,
			Tags:        p.Tags,
			State:       string(p.State),
			Status:      string(a.store.StatusOf(p)),
			Created:     p.Created,
			LastUsed:    p.LastActivity(),
			AgeDays:     int(p.Age(now).Hours() / 24),
			IdleDays:    int(p.Idle(now).Hours() / 24),
			OpenCount:   p.OpenCount,
		}
		if !p.ExpiresAt.IsZero() {
			expires := p.ExpiresAt
			item.ExpiresAt = &expires
		}
		out = append(out, item)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	a.println(string(data))
	return nil
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
