# Scratchpad — Implementation Plan

> **Scratchpad makes experimentation cheap.**
> You should never think "should I even create this project?" — you should think
> `sp new stupid-idea`, and decide later whether it was worth keeping.

Scratchpad is not a better Downloads folder. It is a **lifecycle manager for
things you're not sure deserve to become real projects yet**.

```
Scratch  →  Explore  →  Decide  →  Keep / Archive / Trash
```

---

## On-disk layout

```
~/Downloads/Scratchpad/          scratch_dir    temporary projects live here
├── weather-app/
│   ├── .scratchpad/
│   │   └── metadata.json        the single source of truth for one project
│   └── ...
└── .trash/                      trash_dir      recoverable deletions
    └── old-website/

~/Projects/                      projects_dir   `sp keep` promotes to here
~/Archives/Scratchpad/           archive_dir    `sp archive` compresses to here
```

Config lives at `$XDG_CONFIG_HOME/scratchpad/config.toml` (override with
`$SCRATCHPAD_CONFIG` or `--config`).

---

## Package map

| Package | Responsibility |
| --- | --- |
| `internal/config` | `config.toml`, path expansion, `30d`/`2w`/`never` durations |
| `internal/project` | the `Project` model and the lifecycle rules derived from it |
| `internal/store` | all persistence: create, load, save, list, move between locations |
| `internal/activity` | *(planned)* activity signals: fs mtime, git, cached size |
| `internal/gitx` | the slice of git we need: repo detection, init, status, risk |
| `internal/scaffold` | starter files for a new project |
| `internal/ui` | styles + human formatting (`2 hours ago`, `in 14 days`) |
| `internal/tui` | *(planned)* the bubbletea dashboard |
| `internal/cli` | cobra command tree |
| `cmd/scratchpad` | `main`, wired through fang |

---

## Milestone 0 — Foundation ✅

- [x] `internal/config` — TOML config, `~` expansion, extended durations
- [x] `internal/project` — model, `State` vs derived `Status`, name validation
- [x] `internal/store` — atomic metadata writes, create/load/list/get, locations
- [x] `internal/gitx` — `Available`, `IsRepo`, `Init`
- [x] `internal/scaffold` — README + `git init`, never overwrites
- [x] `internal/ui` — adaptive palette, relative time, `Tildify`
- [x] `internal/cli` + fang wiring, custom error handler
- [x] `sp new` — `-d`, `-t/--tag`, `--ttl`, `-k/--keep`, `--no-git`, `--no-readme`, `-p/--path`
- [x] Tests for config, project rules, store

---

## Milestone 1 — Core lifecycle (V1) ✅

The goal of V1: the full **scratch → decide** loop works from the CLI, and
nothing can silently destroy work.

V1 derives staleness from Scratchpad's own metadata — `created` and
`last_opened`. That is a known limitation: edit a project in your editor without
going through `sp` and it will drift toward Stale anyway. Fixing that is 2.1,
and nothing in this milestone is blocked on it, because every command reads
activity through `project.LastActivity()` and will pick up better signals for
free once they exist.

### 1.1 — `sp list` ✅

The command you will run most.

- [x] `store.Query` with a `Filter` (status, tag, text, idle time) across locations
- [x] Table output: `NAME  LAST USED  AGE  STATUS`, aligned by display width
- [x] Flags: `--all`, `--active`, `--stale`, `--expired`, `--kept`, `--trashed`,
      `--tag`, `--query`, `--older-than 30d`, `--sort used|name|age`, `--reverse`
- [x] `--json` with a stable wire shape, `-q/--quiet` for piping to xargs
- [x] Colour stripped automatically when stdout is not a TTY (colorprofile)
- [x] Empty state that teaches the next command (`sp new <name>`)
- [ ] `SIZE` column and `--sort size` — deferred to 2.1, since it needs the
      cached scan; walking every project tree on the hot path is exactly what
      the performance rule below forbids

**Done when:** `sp list` on an empty scratch dir is helpful rather than blank,
and on 50 projects it is still instant.

### 1.2 — `sp open` ✅

- [x] Resolve the project, `Touch()` it (bumps `last_opened`, `open_count`)
- [x] Launch `config.editor` → `$VISUAL` → `$EDITOR`, with a clear message when none is set
- [x] `-p/--print-path` for shell integration; `--reveal` for the file manager; `--no-editor`
- [x] Name matching in `store.Resolve`: exact, then prefix, then substring.
      Ambiguity is an error listing the candidates — guessing is not worth the
      risk when the next command may delete something

**Done when:** `open_count` becomes a real signal for "possible keeper".

### 1.3 — `sp keep` — the promote path ✅

The feature the whole product hangs on. Moving out of scratch must be one word.

- [x] `store.Keep` — cross-device-safe move (rename, fall back to copy),
      preserving symlinks and modification times
- [x] Clear `expires_at`, set `state: kept`, record `original_path`
- [x] `--to <dir>` to override the destination parent for one project
- [x] Refuse to clobber an existing directory; `OccupiedError` names the path
- [x] Print the new path so the user can `cd` to it

**Done when:** `sp keep awesome-app` moves it to `~/Projects/awesome-app` and
the project stops appearing in scratch listings.

### 1.4 — `sp trash` — with git safety ✅

Never destroy work without saying what is about to be lost.

- [x] `gitx.Read(dir)` — branch, commit count, dirty files, unpushed commits,
      last commit time; ignores Scratchpad's own `.scratchpad/`
- [x] Pre-delete safety report: uncommitted changes and unpushed commits
- [x] Size via `store.DirSize`, a one-off walk at action time. Fine here: it is
      not the hot path, and "you are about to free 128 MB" is the point
- [x] Confirmation prompt, skippable with `-y/--yes`, defaulting to **no**
      whenever work is at risk. Written by hand rather than with `huh`: huh
      needs lipgloss v1 + `x/ansi` v0.9.3, and MVS resolves `x/ansi` to v0.11.0
      for lipgloss v2, which does not compile
- [x] Move to `trash_dir`, set `state: trashed`, stamp `trashed_at` and
      `original_path`
- [x] Name collisions in trash get a timestamp suffix
- [x] `--force` for the genuine "delete it right now" case, refusing any
      directory without a `.scratchpad/` so a bad path cannot become `rm -rf`

**Done when:** trashing a project with uncommitted changes requires an explicit
confirmation that names the files at risk.

### 1.5 — `sp restore` ✅

- [x] Restore to `original_path`, or to scratch if that is occupied
- [x] Reset `state` to `active` and grant a fresh expiry window
- [x] `sp restore --list` (and bare `sp restore`) to browse the trash

**Done when:** `sp restore old-website` undoes a mistaken trash completely.

### 1.6 — `sp info` ✅

- [x] Full detail: description, note, tags, git block, age, activity, expiry,
      open count (size joins once 2.1 lands)
- [x] `--json`

### 1.7 — `sp clean` — the review loop ✅

Cleaning must be a **review**, never a blind `rm -rf`.

- [x] Find expired + stale projects, with git status and size attached
- [x] Interactive multi-select review (huh), with safe candidates pre-ticked
- [x] Confirmation showing how much space comes back
- [x] `--dry-run`, `--older-than`, `-y/--yes`, `--force`
- [x] Never auto-select a project with uncommitted changes. Local-only commits
      do **not** block auto-selection: `clean` moves projects to a recoverable
      trash, so the gate is unsaved work, not unpushed work
- [x] Report reclaimed disk space at the end
- [x] Report and stop when there is no terminal to review in

**Done when:** running `sp clean` on a messy directory feels safe.

### 1.8 — `sp config` ✅

- [x] `sp config` prints the resolved config and where it came from
- [x] `sp config set <key> <value>`, `get`, `path`, `edit`
- [x] Values are validated and expanded on the way in; a bad value is rejected
      whole rather than half-applied
- [x] Shell completion of setting names
- [x] `sp setup` writes a starter config (see the setup wizard below)

### 1.9 — Shell integration ✅

The repo already had `completions/` and `scripts/` waiting for this.

- [x] `sp completion bash|zsh|fish` (cobra), generated at release time rather
      than committed
- [x] Dynamic completion of project names for `open`/`keep`/`trash`/`info`,
      scoped per command so `restore` only offers trashed projects
- [x] `sp shell-init` emitting `spo` and `spn`, since a child process cannot
      change the parent shell's directory
- [x] `scripts/build.sh` stamping `version` via `-ldflags`, plus a `Makefile`

### 1.10 — Setup wizard ✅

Not in the original plan; added because a first run should not begin with
reading documentation.

- [x] `sp setup` (alias `init`): a huh form covering directories, lifespan,
      staleness, editor and git
- [x] Runs automatically on first use, then continues with whatever the user
      actually typed
- [x] Non-interactive runs stay silent on defaults, so scripts and CI are never
      blocked on a prompt
- [x] Editor list is built from what is actually installed
- [x] Trash follows the scratch directory unless explicitly customised
- [x] Cancelling writes nothing

### 1.11 — CI ✅

- [x] Build, vet and `go test -race` on Linux, macOS and Windows
- [x] gofmt check, `go mod tidy` diff check, golangci-lint
- [x] Cross-compile matrix for linux/darwin/windows on amd64 and arm64
- [x] Release automation: pushing to main tags and publishes a GitHub release
      once CI is green, versioned by date (`v2026.09.12`, `-1` for the second
      release of a day)
- [x] `goreleaser check` in CI so the config cannot rot between releases
- [x] `scripts/install.sh` — checksum-verified download, completions for every
      shell found, and the `shell-init` block added to the user's profile
- [ ] Homebrew tap — see Packaging below

---

## Milestone 2 — Make it truthful and fast to live in (V2)

V1 trusts its own metadata. V2 starts by making staleness reflect what you
actually did, then makes the whole thing pleasant to live in.

### 2.1 — `internal/activity`: real staleness signals

Metadata timestamps alone lie — you edit files without going through `sp`.

- [ ] Cheap recursive newest-mtime scan, skipping `node_modules`, `target`,
      `.git`, `venv`, `dist` (they churn without meaning)
- [ ] `gitx.LastCommit(dir)` — last commit time
- [ ] Fold both into `project.RecordActivity`, persist the result
- [ ] `du`-style size calculation with the same skip list
- [ ] Cache derived signals in `.scratchpad/` with an mtime guard so `sp list`
      never re-walks a 2 GB project

**Done when:** a project you edited in your editor (never through `sp`) reads
as Active, not Stale.

### 2.2 — `sp` with no arguments: the TUI

- [ ] Bubbletea dashboard: counts, project list, status column
- [ ] Keys: `Enter` open · `N` new · `K` keep · `A` archive · `D` trash ·
      `R` rename · `S` search · `?` help · `Q` quit
- [ ] Detail pane with the git/size/activity block
- [ ] Confirmation modals reusing the same safety checks as the CLI
- [ ] Falls back to `sp list` when stdout is not a TTY

### 2.3 — Search, tags and notes

- [ ] `sp search <query>` over name, description, note and tags
- [ ] `sp tag <name> [+tag] [-tag]`
- [ ] `sp note <name>` opening `$EDITOR` on the note field
- [ ] `sp rename <old> <new>`

### 2.4 — `sp archive`

The middle option between keeping and deleting.

- [ ] `tar` + `zstd` to `archive_dir/<name>-<date>.tar.zst`
- [ ] Keep a sidecar `.json` of the metadata so archives stay listable
- [ ] `sp archive --list`, `sp unarchive <name>`
- [ ] Report the compression ratio — it is the reward for archiving

### 2.5 — Disk awareness

- [ ] `sp stats` — total size, per-project, biggest offenders, reclaimable
- [ ] Flag `node_modules`-style directories as separately reclaimable

---

## Milestone 3 — The parts that make it feel smart (V3)

### 3.1 — "What's safe to delete?"

The killer feature: open Scratchpad and immediately know what to do.

- [ ] A confidence score per project from age, opens, commits, dirtiness, size
- [ ] Greeting view splitting projects into **🧹 Cleanup** and **⭐ Possible keeper**
- [ ] One-key action on each suggestion

### 3.2 — Templates

- [ ] `sp new x --template node|go|rust|python|web`
- [ ] User templates in `$XDG_CONFIG_HOME/scratchpad/templates/`
- [ ] `sp template list|add|remove`

### 3.3 — Nice-to-haves

- [ ] GitHub/GitLab: detect a remote, warn less about pushed work
- [ ] Snapshots before destructive actions
- [ ] Dependency cleanup (`node_modules`, `target`) without deleting source
- [ ] Optional background "you haven't touched this in 60 days" notification
- [ ] Auto-generated project summaries

---

## Cross-cutting concerns

**Safety rules** — these hold everywhere, not per-command:
1. Nothing is deleted without a recovery path (trash first, `--force` to skip).
2. Uncommitted git changes always require explicit confirmation.
3. Destructive commands are `--dry-run` by default when stdout is not a TTY.
4. Writes are atomic; a failed operation leaves no half-state behind.

**Testing** — every store operation gets a table test against `t.TempDir()`
with an injected clock. CLI commands get golden-output tests with colour off.

**Performance** — `sp list` must stay instant. Scan concurrently, cache derived
signals, and never walk a project tree on the hot path.

**Packaging** — goreleaser builds checksummed archives for linux/darwin/windows
on amd64 and arm64, each carrying the completions for that release. Still open:
a Homebrew tap (`brews:` in `.goreleaser.yaml`, once the tap repo exists).

---

## Decisions already made

| Decision | Why |
| --- | --- |
| Metadata inside each project, no central index | A project survives being moved; nothing to desync |
| The directory name is authoritative on load | Rename on disk and it still resolves |
| `stale` is derived, never stored | Staleness is a fact about activity, not a state you enter |
| `--keep` writes straight to `projects_dir` | A "kept" project sitting in scratch is a lie |
| Atomic write + rollback on failed create | A crash must never leave unmanaged litter in scratch |
| Default expiry 30d, per-project `--ttl` | The spec showed both 14d and 30d; 30d is the configurable default |

## Open questions

- Should `sp keep` offer to `git init` + commit if the project has no repo?
- Should expiry ever *act* on its own, or only ever mark projects for `sp clean`?
  (Current answer: only mark. Nothing deletes itself.)
- Is `projects_dir` one directory, or should `sp keep` support named destinations?
