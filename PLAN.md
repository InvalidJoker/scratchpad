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
| `internal/activity` | *(planned)* activity signals: fs mtime, git, size |
| `internal/gitx` | the slice of git we need: repo detection, init, status |
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

## Milestone 1 — Core lifecycle (V1)

The goal of V1: the full **scratch → decide** loop works from the CLI, and
nothing can silently destroy work.

### 1.1 — `sp list`

The command you will run most.

- [ ] `store.List` across locations, with a `Filter` (state, tag, age, query)
- [ ] Table output: `NAME  LAST USED  AGE  SIZE  STATUS`
- [ ] Flags: `--all`, `--stale`, `--expired`, `--kept`, `--trashed`,
      `--tag <t>`, `--older-than 30d`, `--sort name|age|used|size`
- [ ] `--json` for scripting, plain output when stdout is not a TTY
- [ ] Empty state that teaches the next command (`sp new <name>`)

**Done when:** `sp list` on an empty scratch dir is helpful rather than blank,
and on 50 projects it is still instant.

### 1.2 — `internal/activity`: real staleness signals

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

### 1.3 — `sp open`

- [ ] Resolve the project, `Touch()` it (bumps `last_opened`, `open_count`)
- [ ] Launch `$SCRATCHPAD_EDITOR` → `config.editor` → `$VISUAL` → `$EDITOR`
- [ ] `--print-path` for shell integration; `--reveal` for the file manager
- [ ] Fuzzy name matching: `sp open weath` finds `weather-app`, and prompts
      when a prefix is ambiguous

**Done when:** `open_count` becomes a real signal for "possible keeper".

### 1.4 — `sp keep` — the promote path

The feature the whole product hangs on. Moving out of scratch must be one word.

- [ ] `store.Move(p, Kept)` — cross-device-safe move (rename, fall back to copy)
- [ ] Clear `expires_at`, set `state: kept`, record `original_path`
- [ ] `--to <dir>` to override the destination for one project
- [ ] Refuse to clobber an existing directory in `projects_dir`
- [ ] Print the new path so the user can `cd` to it

**Done when:** `sp keep awesome-app` moves it to `~/Projects/awesome-app` and
the project stops appearing in scratch listings.

### 1.5 — `sp trash` — with git safety

Never destroy work without saying what is about to be lost.

- [ ] `gitx.Status(dir)` — branch, commit count, dirty files, unpushed commits
- [ ] Pre-delete safety report: uncommitted changes, unpushed commits, size
- [ ] Confirmation prompt (`huh`), skippable with `-y/--yes`
- [ ] Move to `trash_dir`, set `state: trashed`, stamp `trashed_at` and
      `original_path`
- [ ] Name collisions in trash get a timestamp suffix
- [ ] `--force` for the genuine "delete it right now" case

**Done when:** trashing a project with uncommitted changes requires an explicit
confirmation that names the files at risk.

### 1.6 — `sp restore`

- [ ] Restore to `original_path`, or to scratch if that is occupied
- [ ] Reset `state` to `active` and grant a fresh expiry window
- [ ] `sp restore --list` to browse the trash

**Done when:** `sp restore old-website` undoes a mistaken trash completely.

### 1.7 — `sp info`

- [ ] Full detail: description, note, tags, git block, size, age, activity,
      expiry, open count
- [ ] `--json`

### 1.8 — `sp clean` — the review loop

Cleaning must be a **review**, never a blind `rm -rf`.

- [ ] Find expired + stale projects, sorted by how safe they are to delete
- [ ] Interactive review: `[Enter] review each · [a] all · [c] cancel`
- [ ] Per project: `[k] keep · [a] archive · [t] trash · [s] skip`
- [ ] `--dry-run` (default when not a TTY), `--older-than`, `--yes`
- [ ] Never auto-select a project with uncommitted git changes
- [ ] Report reclaimed disk space at the end

**Done when:** running `sp clean` on a messy directory feels safe.

### 1.9 — `sp config`

- [ ] `sp config` prints the resolved config and where it came from
- [ ] `sp config set <key> <value>`, `sp config path`, `sp config edit`
- [ ] `sp init` writes a starter config with comments

### 1.10 — Shell integration

The repo already has `completions/` and `scripts/` waiting for this.

- [ ] `sp completion bash|zsh|fish` (cobra) + a `make completions` target
- [ ] Dynamic completion of project names for `open`/`keep`/`trash`/`info`
- [ ] `sp shell-init` emitting a `spcd` function, since a child process cannot
      change the parent shell's directory
- [ ] `scripts/build.sh` stamping `version` via `-ldflags`

---

## Milestone 2 — Make it fast to live in (V2)

### 2.1 — `sp` with no arguments: the TUI

- [ ] Bubbletea dashboard: counts, project list, status column
- [ ] Keys: `Enter` open · `N` new · `K` keep · `A` archive · `D` trash ·
      `R` rename · `S` search · `?` help · `Q` quit
- [ ] Detail pane with the git/size/activity block
- [ ] Confirmation modals reusing the same safety checks as the CLI
- [ ] Falls back to `sp list` when stdout is not a TTY

### 2.2 — Search, tags and notes

- [ ] `sp search <query>` over name, description, note and tags
- [ ] `sp tag <name> [+tag] [-tag]`
- [ ] `sp note <name>` opening `$EDITOR` on the note field
- [ ] `sp rename <old> <new>`

### 2.3 — `sp archive`

The middle option between keeping and deleting.

- [ ] `tar` + `zstd` to `archive_dir/<name>-<date>.tar.zst`
- [ ] Keep a sidecar `.json` of the metadata so archives stay listable
- [ ] `sp archive --list`, `sp unarchive <name>`
- [ ] Report the compression ratio — it is the reward for archiving

### 2.4 — Disk awareness

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

**Packaging** — goreleaser, Homebrew tap, `go install`, checksummed binaries.

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
