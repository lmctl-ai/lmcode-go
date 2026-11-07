#!/usr/bin/env bash
# Pin the shared REST conformance suite (the opt-in `rest` std module); --verify checks the copy against rest-suite.lock.
#   scripts/sync-rest.sh            copy $LMCODE_SPEC_DIR (default ../lmcode)/tests/rest-suite to conformance/rest-suite and write the lock
#   scripts/sync-rest.sh --verify   check the copy against the lock
# Portable digest: sha256 over rows "<sha256(file)>  <relative path>\n", paths sorted bytewise.
set -eu
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC="${LMCODE_SPEC_DIR:-$HERE/../lmcode}"
FIND="find"
command -v /usr/bin/find >/dev/null 2>&1 && FIND="/usr/bin/find"
SORT="sort"
command -v /usr/bin/sort >/dev/null 2>&1 && SORT="/usr/bin/sort"
DST="$HERE/conformance/rest-suite"
LOCK="$HERE/rest-suite.lock"
digest() { (cd "$1" && $FIND . -type f | sed 's|^\./||' | LC_ALL=C $SORT | xargs sha256sum | sed -e 's/ \*/  /' | tr -d '\r' | sha256sum | cut -c1-64); }
if [ "${1:-}" = "--verify" ]; then
  [ -f "$LOCK" ] || { echo "rest-suite.lock missing" >&2; exit 1; }
  want="$(sed -n 's/^sha256=//p' "$LOCK" | tr -d '\r')"; have="$(digest "$DST")"
  [ "$want" = "$have" ] || { echo "conformance/rest-suite digest $have differs from rest-suite.lock $want: run scripts/sync-rest.sh (or restore the pinned copy)" >&2; exit 1; }
  echo "conformance/rest-suite matches rest-suite.lock ($(sed -n 's/^commit=//p' "$LOCK" | tr -d '\r'))"
else
  [ -d "$SPEC/tests/rest-suite" ] || { echo "lmcode checkout not found at $SPEC (set LMCODE_SPEC_DIR)" >&2; exit 1; }
  rm -rf "$DST"; mkdir -p "$DST"; cp -R "$SPEC/tests/rest-suite/." "$DST/"
  printf 'repo=https://github.com/lmctl-ai/lmcode\ncommit=%s\nsha256=%s\n' "$(git -C "$SPEC" rev-parse HEAD)" "$(digest "$DST")" >"$LOCK"
  echo "synced rest-suite from lmcode $(git -C "$SPEC" rev-parse --short HEAD)"
fi
