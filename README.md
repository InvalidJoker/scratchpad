# Scratchpad (`sp`)

A CLI lifecycle manager for experimental projects. It makes starting a
project cheap and deciding its fate explicit:

```
Scratch  →  Explore  →  Decide  →  Keep / Archive / Trash
```

You should never think "should I even create this?" — you should think
`sp new stupid-idea`, and decide later whether it was worth keeping.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/InvalidJoker/scratchpad/main/scripts/install.sh | sh
```

On Windows (PowerShell 5.1+):

```powershell
irm https://raw.githubusercontent.com/InvalidJoker/scratchpad/main/scripts/install.ps1 | iex
```

Both scripts download a checksummed release archive, install the `sp`
binary, wire up shell completions, and add the `spo`/`spn` wrappers (see
[Shell integration](#shell-integration)).

Or, with Go installed:

```sh
go install github.com/InvalidJoker/scratchpad/cmd/scratchpad@latest
```

The first time you run `sp`, a setup wizard walks you through configuring
directories, default lifespan, staleness, editor and git. Non-interactive
runs (scripts, CI) silently use defaults instead of prompting.

## Usage

```sh
sp new weather-app                    # create a scratch project
sp new api-test --ttl 7d --tag exp    # custom expiry and a tag
sp list                               # see what's active, stale, expired
sp open weather-app                   # open it in your editor
sp keep weather-app                   # promote it to a real project
sp trash old-idea                     # move it to the recoverable trash
sp restore old-idea                   # undo that
sp clean                              # interactive review of stale/expired projects
```

| Command | What it does |
| --- | --- |
| `sp new <name>` | Create a scratch project (README + git init by default) |
| `sp list` | List projects, filterable by status/tag/age, `--json` for scripting |
| `sp open <name>` | Launch the project in `$EDITOR`/`$VISUAL`/configured editor |
| `sp info <name>` | Full detail: git status, activity, expiry, tags, notes |
| `sp keep <name>` | Promote out of scratch into `projects_dir` permanently |
| `sp trash <name>` | Move to trash, with a safety report on uncommitted/unpushed work |
| `sp restore [name]` | Restore a trashed project, or browse the trash with `--list` |
| `sp clean` | Interactive review of stale/expired projects, with space reclaimed |
| `sp config` | Inspect or edit the resolved configuration |
| `sp setup` | Re-run the setup wizard |

Run `sp help <command>` for full flags and examples.

## How it decides "stale" and "expired"

Every project gets a default expiry (`sp config set default_expiration 30d`,
or `never`); override it per project with `sp new --ttl 7d`. Once a project
has had no activity for longer than `stale_after`, it's **stale**; once past
its expiry, it's **expired**. Neither state deletes anything on its own —
`sp clean` is always the one that acts, and only after you review it.

## Safety

Scratchpad deletes people's work, so every destructive path:

- Moves to trash rather than deleting, unless you pass `--force`.
- Checks git for uncommitted changes or commits that exist on no remote,
  and requires explicit confirmation when it finds either.
- Refuses to act without a TTY unless pre-approved with `-y/--yes`.
- Never `rm -rf`s a directory that isn't a Scratchpad project.

## Configuration

Config lives at `$XDG_CONFIG_HOME/scratchpad/config.toml` (override with
`$SCRATCHPAD_CONFIG` or `--config`):

```toml
scratch_dir        = "~/Downloads/Scratchpad"
projects_dir        = "~/Projects"
archive_dir         = "~/Archives/Scratchpad"
trash_dir           = "~/Downloads/Scratchpad/.trash"
default_expiration  = "30d"
stale_after         = "14d"
init_git            = true
editor              = ""
```

## Development

```sh
make build      # build ./sp with the version stamped in
make test       # go test -race ./...
make lint       # vet + gofmt check + golangci-lint
make fmt        # gofmt -w
make snapshot   # goreleaser release --snapshot --clean (local artefacts)
```

## License

Scratchpad is licensed under the GNU General Public License v3.0. See [LICENSE](LICENSE) for details.