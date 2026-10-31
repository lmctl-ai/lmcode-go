#!/usr/bin/env bash
set -eu
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDK="${LMCODE_CPP:-$HERE/../lmcode-cpp}"
DST="$HERE/std/lmcode-std.schema.json"
LOCK="$HERE/std.lock"
mkdir -p "$HERE/std"
if [ "${1:-}" = "--verify" ]; then
	[ -f "$DST" ] && [ -f "$LOCK" ] || { echo "std schema or std.lock missing" >&2; exit 1; }
	schema="$(sed -n 's/^schema=//p' "$LOCK")"
	version="$(sed -n 's/^version=//p' "$LOCK")"
	want="$(sed -n 's/^sha256=//p' "$LOCK")"
	have="$(sha256sum "$DST" | cut -d' ' -f1)"
	[ "$schema" = "std/lmcode-std.schema.json" ] && [ "$version" = "3" ] || { echo "std.lock is not the v3 canonical lock" >&2; exit 1; }
	[ "$want" = "$have" ] || { echo "std schema digest differs from std.lock" >&2; exit 1; }
	python3 "$HERE/scripts/gen-std-docs.py" --check
	echo "std v$version schema matches std.lock"
	exit 0
fi
	git -C "$SDK" rev-parse --git-dir >/dev/null 2>&1 || { echo "lmcode-cpp checkout not found at $SDK" >&2; exit 1; }
	commit="$(git -C "$SDK" rev-parse HEAD)"
	git -C "$SDK" show "$commit:std/lmcode-std.schema.json" > "$DST"
	git -C "$SDK" show "$commit:std.lock" > "$LOCK"
	git -C "$SDK" show "$commit:std/lmcode-std.docs.json" > "$HERE/std/lmcode-std.docs.json"
	git -C "$SDK" show "$commit:stdlib/result.schema-fragment.json" > "$HERE/stdlib/result.schema-fragment.json"
	python3 "$HERE/scripts/gen-std-docs.py"
	echo "synced canonical std lock and schema from lmcode-cpp $commit"
