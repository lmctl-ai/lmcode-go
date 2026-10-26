# Build, run, and test

## Test command

Run all tests (unit tests, 51 core vectors and 229 stdlib v2 vectors):

```sh
go test -v ./...
```

## Standalone conformance runner

Run the standalone conformance tool against a vector directory:

```sh
go run ./cmd/conformance ./conformance/vectors
go run ./cmd/conformance ./conformance/stdlib-vectors
```

## Vector synchronization and verification

The sync script copies both `conformance/vectors/` and `conformance/stdlib-vectors/` from C++. Their source commit and SHA-256 digests are pinned independently in `vectors.lock` and `stdlib-vectors.lock`. Verify both copies:

```sh
bash scripts/sync-vectors.sh --verify
```

Resync from sibling repository `../lmcode-cpp`:

```sh
bash scripts/sync-vectors.sh
```

## Example host build

Build the greeter example host as a static binary (`CGO_ENABLED=0`):

```sh
CGO_ENABLED=0 go build -o bin/greeter ./examples/greeter
./bin/greeter examples/greeter/greeter.dslb examples/greeter/schema.json
```

Or run the example test:

```sh
bash examples/greeter/test.sh
```

## Catalog and fixture validation

`go test ./...` checks the canonical 171-signature registry, exact `std.lock` bytes, schema-fragment compatibility binders, all 51 core vectors, and all 229 v2 stdlib vectors. It compares the full schema signature list with `lmscript --dsl-signatures`. Published lmscript 0.2.426 still bundles the legacy 72 default signatures; the empty-host 171-signature default-bundle probe skips that compiler and must be run with `LMSCRIPT_BIN` set to a scratch build of lmctl-mono main. Catalog vectors pin malformed UTF-8, duplicate decoded JSON keys, byte offsets, signed numeric limits, order and priority ties, output allocation caps, glob work limits, and JSON token preservation.

The shared catalog vectors use `bytes(hex)` to supply raw bytes and `Trace.hex(string)` to record lowercase hexadecimal. Both fixture adapters are test-host functions, not SDK catalog entries. Native vector tests verify lock digests without starting bash; the sync script remains available for manual updates. CI runs both standalone vector directories on Linux x64/arm64, macOS x64/arm64, and Windows x64.

The root standard surface is pinned separately: `bash scripts/sync-std.sh --verify` checks `std/lmcode-std.schema.json` against the byte-identical v2 `std.lock`. Native tests compare all 171 enabled signatures with the pinned schema and, when `LMSCRIPT_BIN` points to a v2 compiler, compare the compiler's default empty-host catalog with the registry. The published compiler `0.2.426` still advertises the legacy 72 built-ins, so v2 default-bundle parity uses a scratch build of lmctl-mono main.

Static analysis and a dependency-free build:

```sh
go vet ./...
CGO_ENABLED=0 go build ./...
```

The synchronized catalog suite contains 229 vectors, including containers and JSON;
the core suite remains 51. File fixtures own a fresh temporary directory per
case and remove it afterward. `test_long_path` creates a Unicode filename past
260 characters. `runtime_error_file` checks the operation/path prefix and a
nonempty OS reason without depending on localized wording. Compiler parity
also checks the five-function file fragment and the 171-signature standard.

The catalog suite includes 41 seeded property vectors (seeds 7, 42, 20261024),
with independently calculated byte/numeric/container expectations. CI runs these
through the ordinary catalog conformance step. `LMCODE_TRACE=1` emits actual
trace/error records for byte-exact cross-SDK comparisons; the source generator
and larger 16-seed/64-round mode live in lmcode-cpp `scripts/gen-std-corpus.py`
and `scripts/diff-stdlib.py`. The synchronized suite has 229 catalog cases and
51 core cases.
