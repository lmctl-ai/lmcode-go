#!/usr/bin/env bash
set -eu
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDK="${LMCODE_CPP:-$HERE/../lmcode-cpp}"
DST="$HERE/std/lmcode-std.schema.json"
LOCK="$HERE/std.lock"
mkdir -p "$HERE/std"
if [ "${1:-}" = "--verify" ]; then
	[ -f "$DST" ] && [ -f "$LOCK" ] || { echo "std schema or std.lock missing" >&2; exit 1; }
	want="$(sed -n 's/^sha256=//p' "$LOCK")"
	have="$(sha256sum "$DST" | cut -d' ' -f1)"
	[ "$want" = "$have" ] || { echo "std schema digest differs from std.lock" >&2; exit 1; }
	echo "std schema matches std.lock ($(sed -n 's/^commit=//p' "$LOCK"))"
	exit 0
fi
	[ -d "$SDK/.git" ] || { echo "lmcode-cpp checkout not found at $SDK" >&2; exit 1; }
	commit="$(git -C "$SDK" rev-parse HEAD)"
	git -C "$SDK" show "$commit:std/lmcode-std.schema.json" > "$DST"
	digest="$(sha256sum "$DST" | cut -d' ' -f1)"
	printf 'repo=https://github.com/lmctl-ai/lmcode-cpp\ncommit=%s\nsha256=%s\n' "$commit" "$digest" > "$LOCK"
	echo "synced std schema from lmcode-cpp $commit"
