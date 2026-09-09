# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

**Scratchpad** (`sp`) is a CLI lifecycle manager for experimental projects. It
makes starting a project cheap and deciding its fate explicit:

```
Scratch  →  Explore  →  Decide  →  Keep / Archive / Trash
```

`PLAN.md` holds the full feature roadmap and the decisions already made. Read it
before adding a feature — most of them are already specified there.

## Commands

```bash
go build ./...                       # build
go test ./...                        # test
go vet ./... && gofmt -l internal cmd # lint (gofmt must print nothing)
go build -o /tmp/sp ./cmd/scratchpad # binary for manual testing
```

Manual testing must never touch the real scratch directory. Point the binary at
a throwaway config:

```bash
T=$(mktemp -d)
cat > $T/config.toml <<EOF
scratch_dir = "$T/Scratchpad"
projects_dir = "$T/Projects"
trash_dir = "$T/Scratchpad/.trash"
EOF
SCRATCHPAD_CONFIG=$T/config.toml /tmp/sp new demo
```

## Architecture

Dependencies point one way: `cli` → `store` → `project` → `config`. Nothing
below `cli` knows about cobra, and nothing outside `ui` formats for a terminal.

| Package | Responsibility |
| --- | --- |
| `internal/config` | `config.toml`, `~` expansion, `30d`/`2w`/`never` durations |
| `internal/project` | the `Project` model and lifecycle rules |
| `internal/store` | all persistence and directory moves |
| `internal/gitx` | git subprocess wrapper |
| `internal/scaffold` | starter files for new projects |
| `internal/ui` | lipgloss styles and human-readable formatting |
| `internal/cli` | cobra command tree |
| `cmd/scratchpad` | `main`, wired through fang |

## Model invariants

These are load-bearing. Breaking one breaks the product, not just a test.

- **Metadata lives in the project**, at `<project>/.scratchpad/metadata.json`.
  There is no central index, so a project survives being moved by hand.
- **The directory name wins.** `store.LoadDir` overrides `Name` with the
  directory's basename, so renaming on disk still resolves.
- **`State` is persisted; `Status` is derived.** Persisted state is
  `active | kept | archived | trashed`. `stale` and `expired` are computed at
  read time by `project.Status(now, staleAfter)` — never store them.
- **Activity flows through `RecordActivity`.** Filesystem and git signals move
  the activity floor forward; they never move it backwards.
- **Writes are atomic.** Metadata goes to a temp file then `os.Rename`. A failed
  `Create` removes the directory it made.

## Safety rules

Scratchpad deletes people's work. Every destructive path must:

1. Move to trash rather than delete, unless `--force`.
2. Check `gitx` for uncommitted or unpushed work and require explicit
   confirmation when it finds any.
3. Default to `--dry-run` when stdout is not a TTY.

## Conventions

- **Comments explain why, not what.** Do not write a doc comment that restates
  the identifier — `// Dir returns the project's directory` earns nothing. Do
  write the comment that explains a rollback, a fallback, or a workaround.
- **Errors are lowercase, no trailing period**, and name the thing that failed:
  `fmt.Errorf("create project directory: %w", err)`. The CLI adds punctuation.
  Wrap with `%w`; callers branch on the sentinels in `store`.
- **Never title-case error text.** `internal/cli/errors.go` replaces fang's
  default handler for exactly this reason: it mangled `--ttl` into `--Ttl`.
- **New commands** go in `internal/cli/<command>.go` as
  `new<Name>Command(app *App) *cobra.Command`, registered in `root.go`. Keep the
  logic in a method on `App` so it stays testable.
- **All output goes through `internal/ui`** so colour and formatting stay
  consistent, and through `app.println` so it can be captured in tests.
- **Every store operation gets a test** against `t.TempDir()` with an injected
  clock via `store.SetClock`.

## Dependencies

cobra (commands), fang (CLI presentation), lipgloss v2 (styles), BurntSushi/toml
(config). fang pulls `charm.land/lipgloss/v2` — use that import path, not
`github.com/charmbracelet/lipgloss` v1. The two are not compatible.

Planned: bubbletea and huh for the TUI and prompts (Milestone 2).
