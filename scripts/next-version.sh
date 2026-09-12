#!/usr/bin/env sh
# Print the tag for the next release: today's UTC date, with -N appended when
# today already has one. v2026.09.12, then v2026.09.12-1, v2026.09.12-2, ...
#
# Prints nothing (exit 0) when nothing worth releasing has landed — that is how
# the release workflow decides to skip a push without failing it.
set -eu

# UTC, because the runner's idea of "today" should not depend on where it runs.
BASE="v$(date -u +%Y.%m.%d)"

last=$(git describe --tags --abbrev=0 2>/dev/null || true)
if [ -n "$last" ]; then
  # Nothing new since the last tag, or HEAD is that tag.
  if [ "$(git rev-list -n 1 "$last")" = "$(git rev-parse HEAD)" ]; then
    exit 0
  fi

  # chore, docs, test and ci commits are filtered out of the changelog, so a
  # push carrying only those would publish a release with nothing in it.
  subjects=$(git log --format=%s "$last..HEAD")
  messages=$(git log --format=%B "$last..HEAD")
  if ! printf '%s\n' "$subjects" | grep -qE '^(feat|fix|perf|refactor|revert)(\([^)]*\))?!?:' &&
     ! printf '%s\n' "$subjects" | grep -qE '^[a-zA-Z]+(\([^)]*\))?!:' &&
     ! printf '%s\n' "$messages" | grep -qE '^BREAKING[ -]CHANGE'; then
    exit 0
  fi
fi

# The first release of the day is bare; the rest count up from 1. Suffixes are
# read back off the tags themselves, so a failed release does not reuse a name.
if ! git rev-parse -q --verify "refs/tags/$BASE" >/dev/null; then
  printf '%s\n' "$BASE"
  exit 0
fi

next=1
for tag in $(git tag --list "$BASE-*"); do
  n=${tag#"$BASE-"}
  case "$n" in
    ''|*[!0-9]*) continue ;;
  esac
  if [ "$n" -ge "$next" ]; then
    next=$((n + 1))
  fi
done

printf '%s-%s\n' "$BASE" "$next"
