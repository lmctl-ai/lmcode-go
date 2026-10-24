# Build, run, and test

## Test command

Run all tests (unit tests, 51 core vectors and 29 catalog vectors):

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

`go test ./stdlib` checks the string catalog and the 16 math signatures against their schema fragments, then compares registered signatures with `lmscript --dsl-signatures` when the compiler is on PATH. Set `LMSCRIPT_BIN` to choose a compiler explicitly. Catalog tests cover invalid UTF-8, NUL, byte slicing, ASCII operations, signed numeric limits and rounding, output allocation caps, glob work limits, integer wrapping and exact roots, and IEEE float edge cases.

The shared catalog vectors use `bytes(hex)` to supply raw bytes and `Trace.hex(string)` to record lowercase hexadecimal. Both fixture adapters are test-host functions, not SDK catalog entries. Native vector tests verify lock digests without starting bash; the sync script remains available for manual updates. CI runs both standalone vector directories on Linux x64/arm64, macOS x64/arm64, and Windows x64.

Static analysis and a dependency-free build:

```sh
go vet ./...
CGO_ENABLED=0 go build ./...
```
