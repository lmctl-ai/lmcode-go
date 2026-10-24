# lmcode-go notes

Go runtime and SDK for lmscript `_dsl_` bytecode. The package provides a dependency-free loader, structural verifier, stack VM, typed host registry, and shared conformance runner.
Repository: https://github.com/lmctl-ai/lmcode-go. Root package: `github.com/lmctl-ai/lmcode-go`. Toolchain: Go 1.26+.

## Document map

| File | Contents |
| --- | --- |
| [01-purpose-and-place.md](01-purpose-and-place.md) | Purpose, scope, and relationship with Go host applications |
| [02-decisions.md](02-decisions.md) | Architectural decisions, zero third-party dependencies, byte strings |
| [03-build-run-test.md](03-build-run-test.md) | Building, running unit tests, 51 core vectors, 61 catalog vectors |
| [04-gotchas.md](04-gotchas.md) | Schema byte hash binding, LF line endings, version 5 invariants |
| [05-open-items.md](05-open-items.md) | Planned improvements and sibling SDK alignment |

## Adding an example or host function

### Adding an example
1. Create `examples/<name>/` with a Go embedding host `main.go`, `schema.json`, and `.lms` script.
2. Compile bytecode with `lmscript build.lms <name>.dslb`.
3. Add verification script `test.sh` in the example directory.

### Adding a host function/method in Go
1. Use `reg := lmcode.NewRegistry()`.
2. Register methods: `reg.Method(type, name, paramTypes, returnType, callback)`.
3. Register functions: `reg.Function(name, paramTypes, returnType, callback)`.
4. Register context objects: `reg.Context(name, type)`.

## Releases
Releases are built and published automatically on every push to `main` when CI workflows succeed.
Cross-compiled for Linux (x64/arm64), macOS (arm64/x64), and Windows (x64).
