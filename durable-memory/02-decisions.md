# Public decisions

- Zero third-party dependencies: uses standard library only (`crypto/sha256`, `math`, `fmt`, etc.). `go.mod` has no `require` block.
- Root library package is `lmcode`, imported as `github.com/lmctl-ai/lmcode-go`.
- Strings are arbitrary byte sequences (including invalid UTF-8): kept as Go `string`, never ranged over as runes, never re-encoded or validated. Embedded NUL bytes are preserved.
- 64-bit two's-complement arithmetic with wraparound: the VM explicitly guards `math.MinInt64 / -1` (wraps to `math.MinInt64`) and `math.MinInt64 % -1` (evaluates to 0), whose native Go operations have a panic case. Division and remainder by zero produce `RUN_ERROR`.
- Float formatting: `FormatG15` replicates C's `%.15g` exact behavior (`inf`, `-inf`, `nan`, `-0`) deterministically across platforms.
- Host objects: methods accept `self any` and arguments `args []Value`. The VM defensively checks the schema type name of `self` at call time.
- Host error reporting: callbacks return `(Value, error)`. Non-nil errors preserve existing `*Error` codes or wrap as `RUN_HOST`. Line and column reporting locates errors at the faulting bytecode instruction.
- Single test command: `go test ./...` runs 51 core conformance vectors, 18 optional string catalog vectors, both vector-lock integrity checks, and native unit tests. The conformance host registers the catalog and lowercase-hex byte fixture adapters as ordinary host callbacks. Native Go digest checks keep tests independent of shell discovery, including on Windows.
- Package `stdlib` provides all 16 functions in the C++ string catalog and the `RegisterString` binder. They are ordinary optional host functions; no VM opcodes or dependencies are added. `stdlib/string.schema-fragment.json` declares their exact signatures.
- Catalog offsets and lengths count bytes; substring slicing can split a UTF-8 codepoint. Case conversion changes only ASCII letters; trimming removes ASCII space and bytes 0x09 through 0x0d.
- Repeat and replacement preflight the 64 MiB result cap, including no-op replacement copies. Repeat rejects negative counts. Catalog resource policy failures are `RUN_HOST`, independent of VM resource policy.
- Decimal parsers accept complete signed tokens without whitespace or hexadecimal syntax. Integer overflow, nonfinite floats, and nonzero underflow to zero are invalid; conversion helpers return zero on failure. Representable subnormals and signed floating zero are preserved, without consulting process locale.
- Byte glob matching supports stars, question marks, negated classes, ranges and escapes, with constant auxiliary memory. Every matcher iteration, scanned class entry and skipped trailing star counts toward a 2^27 per-call work cap.
