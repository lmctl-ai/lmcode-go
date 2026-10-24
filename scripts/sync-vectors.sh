#!/usr/bin/env bash
# Pin core and optional string catalog fixtures from lmcode-cpp; --verify checks both digests.
set -eu
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDK="${LMCODE_CPP:-$HERE/../lmcode-cpp}"
FIND="find"
command -v /usr/bin/find >/dev/null 2>&1 && FIND="/usr/bin/find"
SORT="sort"
command -v /usr/bin/sort >/dev/null 2>&1 && SORT="/usr/bin/sort"
digest() { (cd "$1" && $FIND . -type f | LC_ALL=C $SORT | xargs sha256sum | sed -e 's/ \*/  /' | tr -d '\r' | sha256sum | cut -c1-64); }
for suite in vectors stdlib-vectors; do
  DST="$HERE/conformance/$suite"
  LOCK="$HERE/$suite.lock"
  if [ "${1:-}" = "--verify" ]; then
    [ -f "$LOCK" ] || { echo "$suite.lock missing" >&2; exit 1; }
    want="$(sed -n 's/^sha256=//p' "$LOCK")"; have="$(digest "$DST")"
    [ "$want" = "$have" ] || { echo "conformance/$suite digest $have differs from $suite.lock $want: run scripts/sync-vectors.sh (or restore the pinned copy)" >&2; exit 1; }
    echo "conformance/$suite matches $suite.lock ($(sed -n 's/^commit=//p' "$LOCK"))"
  else
    [ -d "$SDK/conformance/$suite" ] || { echo "lmcode-cpp checkout not found at $SDK (set LMCODE_CPP)" >&2; exit 1; }
    rm -rf "$DST"; mkdir -p "$DST"; cp "$SDK/conformance/$suite/"* "$DST/"
    printf 'repo=https://github.com/lmctl-ai/lmcode-cpp\ncommit=%s\nsha256=%s\n' "$(git -C "$SDK" rev-parse HEAD)" "$(digest "$DST")" >"$LOCK"
    echo "synced $suite from lmcode-cpp $(git -C "$SDK" rev-parse --short HEAD)"
  fi
done
