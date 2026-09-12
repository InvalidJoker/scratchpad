#!/usr/bin/env sh
# Install Scratchpad (sp) from a GitHub release, with completions and the
# shell-init wrappers wired up.
#
#   curl -fsSL https://raw.githubusercontent.com/InvalidJoker/scratchpad/main/scripts/install.sh | sh
#
# Options (also readable as environment variables):
#   --version <tag>     SP_VERSION           release to install (default: latest)
#   --dir <path>        SP_INSTALL_DIR       where the binary goes
#   --no-completions    SP_NO_COMPLETIONS=1  skip shell completions
#   --no-shell-init     SP_NO_SHELL_INIT=1   skip the spo/spn profile block
set -eu

REPO="InvalidJoker/scratchpad"
VERSION="${SP_VERSION:-}"
INSTALL_DIR="${SP_INSTALL_DIR:-}"
NO_COMPLETIONS="${SP_NO_COMPLETIONS:-}"
NO_SHELL_INIT="${SP_NO_SHELL_INIT:-}"

BLOCK_START="# >>> scratchpad shell integration >>>"
BLOCK_END="# <<< scratchpad shell integration <<<"

TMP=""
cleanup() { [ -n "$TMP" ] && rm -rf "$TMP"; }
trap cleanup EXIT INT TERM

say() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
  cat <<'USAGE'
Install Scratchpad (sp) from a GitHub release.

  --version <tag>     SP_VERSION           release to install (default: latest)
  --dir <path>        SP_INSTALL_DIR       where the binary goes
  --no-completions    SP_NO_COMPLETIONS=1  skip shell completions
  --no-shell-init     SP_NO_SHELL_INIT=1   skip the spo/spn profile block
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ $# -ge 2 ] || die "--version needs a tag"; VERSION="$2"; shift 2 ;;
    --dir) [ $# -ge 2 ] || die "--dir needs a path"; INSTALL_DIR="$2"; shift 2 ;;
    --no-completions) NO_COMPLETIONS=1; shift ;;
    --no-shell-init) NO_SHELL_INIT=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option $1 (try --help)" ;;
  esac
done

# --- what are we installing on ------------------------------------------------

detect_os() {
  case "$(uname -s)" in
    Linux) echo linux ;;
    Darwin) echo darwin ;;
    MINGW*|MSYS*|CYGWIN*) die "Windows is not supported by this script: download the .zip from https://github.com/$REPO/releases" ;;
    *) die "unsupported operating system $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) die "unsupported architecture $(uname -m)" ;;
  esac
}

# fetch writes a URL to stdout. Failures are fatal, so a truncated download can
# never be mistaken for a release artefact.
fetch() {
  if have curl; then
    curl -fsSL "$1"
  elif have wget; then
    wget -qO- "$1"
  else
    die "neither curl nor wget is available"
  fi
}

# latest_version reads the tag from the /releases/latest redirect rather than
# the API, which is rate-limited for unauthenticated callers.
latest_version() {
  url="https://github.com/$REPO/releases/latest"
  tag=""
  if have curl; then
    tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$url" 2>/dev/null | sed 's#.*/tag/##')
  elif have wget; then
    tag=$(wget -qS --max-redirect 5 -O /dev/null "$url" 2>&1 |
      sed -n 's#^[[:space:]]*Location:.*/tag/\([^[:space:]]*\).*#\1#p' | tail -1)
  fi
  case "$tag" in
    ""|*/*) die "could not determine the latest release: pass --version <tag>" ;;
  esac
  printf '%s\n' "$tag"
}

# default_install_dir prefers a system location the user can already write to,
# and otherwise stays inside $HOME rather than asking for sudo.
default_install_dir() {
  for dir in /usr/local/bin "$HOME/.local/bin"; do
    if [ -w "$dir" ] 2>/dev/null; then
      printf '%s\n' "$dir"
      return
    fi
  done
  printf '%s\n' "$HOME/.local/bin"
}

verify_checksum() {
  archive="$1" name="$2" sums="$3"
  line=$(grep " \{1,2\}\*\{0,1\}${name}\$" "$sums" 2>/dev/null || true)
  if [ -z "$line" ]; then
    warn "no checksum published for $name, skipping verification"
    return
  fi
  expected=$(printf '%s\n' "$line" | awk '{print $1}')
  if have sha256sum; then
    actual=$(sha256sum "$archive" | awk '{print $1}')
  elif have shasum; then
    actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  else
    warn "no sha256 tool available, skipping verification"
    return
  fi
  [ "$expected" = "$actual" ] || die "checksum mismatch for $name"
  say "  checksum ok"
}

# --- download and install the binary -----------------------------------------

OS=$(detect_os)
ARCH=$(detect_arch)
[ -n "$VERSION" ] || VERSION=$(latest_version)
[ -n "$INSTALL_DIR" ] || INSTALL_DIR=$(default_install_dir)

# Release archives are named with the bare version; tags carry the v.
NUMBER=${VERSION#v}
ARCHIVE="sp_${NUMBER}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"

TMP=$(mktemp -d)
say "Scratchpad $VERSION ($OS/$ARCH)"
say "  downloading $ARCHIVE"
fetch "$BASE/$ARCHIVE" > "$TMP/$ARCHIVE" || die "download failed: $BASE/$ARCHIVE"
fetch "$BASE/checksums.txt" > "$TMP/checksums.txt" 2>/dev/null || true
verify_checksum "$TMP/$ARCHIVE" "$ARCHIVE" "$TMP/checksums.txt"

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" || die "could not extract $ARCHIVE"
[ -f "$TMP/sp" ] || die "archive did not contain an sp binary"

mkdir -p "$INSTALL_DIR" || die "could not create $INSTALL_DIR"
[ -w "$INSTALL_DIR" ] || die "$INSTALL_DIR is not writable: rerun with --dir <path>"
chmod +x "$TMP/sp"
# Replace via rename so a running sp is never half-overwritten.
mv -f "$TMP/sp" "$INSTALL_DIR/sp" || die "could not install into $INSTALL_DIR"
BIN="$INSTALL_DIR/sp"
say "  installed $BIN"

# --- completions --------------------------------------------------------------

# completion prints the script for a shell, preferring the copy shipped in the
# archive and falling back to the binary we just installed.
completion() {
  if [ -f "$TMP/completions/sp.$1" ]; then
    cat "$TMP/completions/sp.$1"
  else
    "$BIN" completion "$1"
  fi
}

install_file() {
  dest="$1"
  mkdir -p "$(dirname "$dest")" || return 1
  cat > "$dest" || return 1
  say "  completions: $dest"
}

# zsh only loads completions from fpath, so reuse a directory that is already
# there when one is writable, and fall back to the XDG location otherwise.
ZSH_COMP_DIR=""
ZSH_NEEDS_FPATH=""
pick_zsh_completion_dir() {
  if have zsh; then
    for dir in $(zsh -c 'print -rl -- $fpath' 2>/dev/null); do
      case "$dir" in
        */site-functions|*/zfunc)
          if [ -d "$dir" ] && [ -w "$dir" ]; then
            ZSH_COMP_DIR="$dir"
            return
          fi
          ;;
      esac
    done
  fi
  ZSH_COMP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions"
  ZSH_NEEDS_FPATH=1
}

if [ -n "$NO_COMPLETIONS" ]; then
  say "  completions: skipped"
else
  if have bash; then
    completion bash | install_file \
      "${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion/completions/sp" || warn "bash completions failed"
  fi
  if have zsh; then
    pick_zsh_completion_dir
    completion zsh | install_file "$ZSH_COMP_DIR/_sp" || warn "zsh completions failed"
  fi
  if have fish; then
    completion fish | install_file \
      "${XDG_CONFIG_HOME:-$HOME/.config}/fish/completions/sp.fish" || warn "fish completions failed"
  fi
fi

# --- shell integration (spo / spn) -------------------------------------------

# rc_file names the profile for the user's login shell, since that is the only
# one we have any business editing.
rc_file() {
  case "$(basename "${SHELL:-sh}")" in
    zsh) printf '%s\n' "${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash)
      # macOS bash reads .bash_profile for login shells; prefer whichever exists.
      if [ -f "$HOME/.bashrc" ] || [ ! -f "$HOME/.bash_profile" ]; then
        printf '%s\n' "$HOME/.bashrc"
      else
        printf '%s\n' "$HOME/.bash_profile"
      fi
      ;;
    fish) printf '%s\n' "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish" ;;
    *) printf '\n' ;;
  esac
}

shell_block() {
  case "$(basename "${SHELL:-sh}")" in
    fish)
      printf '%s\n' "$BLOCK_START"
      printf '%s\n' 'sp shell-init fish | source'
      printf '%s\n' "$BLOCK_END"
      ;;
    zsh)
      printf '%s\n' "$BLOCK_START"
      if [ -n "$ZSH_NEEDS_FPATH" ]; then
        printf 'fpath=(%s $fpath)\n' "${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions"
        printf '%s\n' 'autoload -Uz compinit && compinit'
      fi
      printf '%s\n' 'eval "$(sp shell-init zsh)"'
      printf '%s\n' "$BLOCK_END"
      ;;
    *)
      printf '%s\n' "$BLOCK_START"
      printf '%s\n' 'eval "$(sp shell-init bash)"'
      printf '%s\n' "$BLOCK_END"
      ;;
  esac
}

RC=$(rc_file)
if [ -n "$NO_SHELL_INIT" ]; then
  say "  shell integration: skipped (add it with: eval \"\$(sp shell-init)\")"
elif [ -z "$RC" ]; then
  warn "unrecognised shell ${SHELL:-}, add this to your profile yourself:"
  say '  eval "$(sp shell-init)"'
elif [ -f "$RC" ] && grep -qF "$BLOCK_START" "$RC"; then
  say "  shell integration: already in $RC"
else
  mkdir -p "$(dirname "$RC")"
  { printf '\n'; shell_block; } >> "$RC" && say "  shell integration: added to $RC" ||
    warn "could not write $RC"
fi

# --- final word ---------------------------------------------------------------

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) warn "$INSTALL_DIR is not on your PATH; add it with: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

say ""
say "Done. Restart your shell, then run: sp setup"
