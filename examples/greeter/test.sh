#!/usr/bin/env bash
set -eu
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
cd "$ROOT"

BIN="$ROOT/bin/greeter"
CGO_ENABLED=0 go build -o "$BIN" ./examples/greeter
if [ -f "$BIN.exe" ]; then
  BIN="$BIN.exe"
fi

# Introspection host signatures test. The default registry also contains the
# canonical std surface, so compare the host-only view to the host schema.
# lmscript 0.2.420 builds std into every schema; --without-std isolates the
# host-declared surface.
diff -u <("$BIN" --host-signatures | tr -d '\r') <(
  if command -v lmscript >/dev/null 2>&1; then
    lmscript --lmcode-signatures "$HERE/schema.json" --without-std | tr -d '\r'
  else
    printf "context g: Greeter\nfn now() -> string\nmethod Greeter.say(string) -> void\n"
  fi
)

# Recompile if lmscript is available
if command -v lmscript >/dev/null 2>&1; then
  (cd "$HERE" && lmscript build.lms "$HERE/greeter.dslb")
fi

# Run greeter
OUT="$("$BIN" "$HERE/greeter.dslb" "$HERE/schema.json" | tr -d '\r')"
echo "greeter output: $OUT"
[ "$OUT" = "Hello from Go DSL at 2026-10-24" ]
echo "ok - greeter example"
