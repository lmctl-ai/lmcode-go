# Public decisions

- Zero third-party dependencies: uses standard library only (`crypto/sha256`, `math`, `fmt`, etc.). `go.mod` has no `require` block.
- Root library package is `lmcode`, imported as `github.com/lmctl-ai/lmcode-go`.
- Strings are raw UTF-8 byte sequences: kept as Go `string`, never ranged over as runes, never re-encoded or validated. Embedded NUL bytes are preserved.
- 64-bit two's-complement arithmetic with wraparound: native wrapping, with explicit guards for `math.MinInt64 / -1` (wraps to `math.MinInt64`) and `math.MinInt64 % -1` (evaluates to 0) which would otherwise panic in Go runtime. Division and remainder by zero produce `RUN_ERROR`.
- Float formatting: `FormatG15` replicates C's `%.15g` exact behavior (`inf`, `-inf`, `nan`, `-0`) deterministically across platforms.
- Host objects: methods accept `self any` and arguments `args []Value`. The VM defensively checks the schema type name of `self` at call time.
- Host error reporting: callbacks return `(Value, error)`. Non-nil errors preserve existing `*Error` codes or wrap as `RUN_HOST`. Line and column reporting locates errors at the faulting bytecode instruction.
- Single test command: `go test ./...` runs all 51 conformance vectors, vectors.lock integrity checks, and native unit tests.
- Standard library catalog: package `stdlib` (`github.com/lmctl-ai/lmcode-go/stdlib`) provides optional host functions starting with `str_len(string) -> int` (byte length, NULs included, raw UTF-8 bytes) and `RegisterString` binder.
