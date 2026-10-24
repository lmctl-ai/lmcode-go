# Build, run, and test

## Test command

Run all tests (unit tests and all 50 conformance vectors):

```sh
go test -v ./...
```

## Standalone conformance runner

Run the standalone conformance tool against a vector directory:

```sh
go run ./cmd/conformance ./conformance/vectors
```

## Vector synchronization and verification

Verify that the local copy in `conformance/vectors` matches the pin in `vectors.lock`:

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
