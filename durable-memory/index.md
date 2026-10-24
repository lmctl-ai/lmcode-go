# lmcode-go public facts

Go bytecode runtime for the lmscript `_dsl_` framework. The package provides a dependency-free loader, structural verifier, stack VM, typed host registry, and shared conformance runner.

Status: experimental public v5 integration. Module path `github.com/lmctl-ai/lmcode-go`, root package `lmcode`. Zero third-party dependencies (`go.mod` has no `require`). Go 1.26+ toolchain.

The runtime format is owned by the public `lmcode-cpp`, `lmcode-rust`, `lmcode-go`, and `lmcode-csharp` projects. Bytecode is schema-bound by SHA-256, big-endian, and intentionally has no backward compatibility promise (version 5 only).

The DSL has no bytecode collection values. Hosts expose collections as schema objects with `len()` and `at(i)`. The common language includes loops, compiler-inlined functions, `foreach`, `switch`, `while`, `break`, `continue`, and common numeric math. Source line information is carried into runtime errors by the v5 SDK.

## Public files

- `README.md`: setup, embedding, and examples.
- `vm.go`, `loader.go`, `registry.go`, `value.go`, `errors.go`: core SDK library.
- `cmd/conformance/main.go`: standalone conformance runner.
- `conformance/`: shared conformance runner and vector tests.
- `examples/greeter/`: 20-line embedding host example.
- `scripts/sync-vectors.sh`: synchronization and lock verification against `lmcode-cpp`.
- `durable-memory/`: public design facts and operational notes.
- `LICENSE`: MIT License.

Public repository: https://github.com/lmctl-ai/lmcode-go
