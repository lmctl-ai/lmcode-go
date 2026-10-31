# lmcode-go

The Go runtime for lmscript `_lmcode_` bytecode: loader, verifier, stack VM, and typed host registry in package `lmcode`.
Go 1.26+, standard library only, zero third-party dependencies (`go.mod` has no `require`). Builds cleanly with `CGO_ENABLED=0` into static binaries.

## Status

Experimental. The current runtime reads bytecode version 6 only; older and newer versions are refused. Use a matching
`lmscript` compiler from the published `@lmctl-ai/lmctl` npm package (or toolchain).

## What it is

lmscript compiles a `_lmcode_` block (a small strongly typed configuration language whose names come from a JSON schema) into a
portable bytecode file (`.dslb`). A host program written in Go imports `github.com/lmctl-ai/lmcode-go`, implements the schema's
objects and functions in an `lmcode.Registry`, loads the file with the exact schema bytes, and runs the chunk once per event with an
`lmcode.Context` of live host objects. The host owns the process, the objects, and their lifetime; the chunk only calls what
the registry offers. lmscript never executes the lmcode program; this SDK never reads the schema's content, only its bytes (for the
SHA-256 binding).

## Gallery

| **Bytecode Conformance Suite** | **Unit Tests & Invariants** |
| --- | --- |
| <img src="docs/images/conformance.png" width="440" alt="Bytecode Conformance Suite"> | <img src="docs/images/unit-tests.png" width="440" alt="Unit Tests & Invariants"> |
| All 70 core bytecode vectors pass across opcodes, types, error paths, and control flow.<br>`go run ./cmd/conformance conformance/vectors` | Unit tests covering VM type invariants, line tables, and registry semantics.<br>`go test -v .` |

| **Greeter Example Host** | **Same Bytecode, Four Runtimes** |
| --- | --- |
| <img src="docs/images/greeter-example.png" width="440" alt="Greeter Example Host"> | <img src="docs/images/same-bytecode-four-runtimes.png" width="440" alt="Same Bytecode, Four Runtimes"> |
| Minimal embedding host compiling and executing lmcode logic in pure Go.<br>`lmscript examples/greeter/build.lms && ./bin/greeter greeter.dslb` | Cross-language parity: C++, Rust, Go, and C# run the exact same bytecode.<br>`PASS Historical v5 parity; v6 ports are rolling out` |

## Embedding it in twenty lines

```go
package main

import (
	"fmt"
	"os"

	"github.com/lmctl-ai/lmcode-go"
)

type Greeter struct{ last string }

func main() {
	reg := lmcode.NewRegistry()
	_ = reg.Context("g", "Greeter")
	_ = reg.Method("Greeter", "say", []string{"string"}, "void", func(self any, a []lmcode.Value) (lmcode.Value, error) {
		self.(*Greeter).last = a[0].Str
		return lmcode.None(), nil
	})
	_ = reg.Function("now", nil, "string", func(a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String("2026-10-24"), nil
	})
	if len(os.Args) > 1 && os.Args[1] == "--lmcode-signatures" {
		for _, s := range reg.Signatures() { fmt.Println(s) }
		return
	}

	bc, _ := lmcode.ReadFile(os.Args[1])
	schema, _ := lmcode.ReadFile(os.Args[2])
	program, err := lmcode.Load(bc, schema, reg)
	lmcode.ExitIfError("load", err)

	greeter := &Greeter{}
	ctx := lmcode.NewContext()
	ctx.Set("g", greeter)
	err = program.Run(ctx, reg)
	lmcode.ExitIfError("run", err)
	fmt.Println(greeter.last)
}
```

`Registry` is declared with the schema's types (parameter types and return type, as strings); a name may have several overloads with distinct parameter tuples, while registering the same parameter tuple twice is refused. `Load()` resolves canonical import signatures (including parameter and return types), and refuses a file whose imports the registry does not provide, before anything runs. `reg.Signatures()` lists the sorted canonical strings (`context g: Greeter`, `fn now() -> string`, `method Greeter.say(string) -> void`), including enabled std. Compare compiler declarations with `CheckSchemaSignatures` so both host-only and std-merged schemas are accepted, while host declaration drift is rejected. That is a consistency check, not a defense: by design the VM performs no comprehensive call-time argument or return checks; it still checks method receiver kinds and other defensive VM invariants. Bytecode is trusted like the host's own code. `Value` carries `INT` (int64), `FLOAT` (binary64), `STR`, `BOOL`, and `OBJ` (host object pointer/interface plus schema type name); a void method returns `lmcode.None()`. Host code returns an error or calls `lmcode.NewHostError(msg)` to report a run-time failure.

**Result line and exit codes.** CLI hosts print exactly one JSON line on stdout through `lmcode.ReportOK(json)`, `lmcode.ReportError("load"|"run", err)` (returns 3) or `lmcode.ReportAppFailure(message)` (returns 4) and exit with the returned code; lmscript's own compile failures are exit 2. `Error.Code` is `LOAD_BAD_BYTECODE`, `LOAD_SCHEMA_MISMATCH`, `LOAD_UNBOUND`, `RUN_ERROR` (a VM error), `RUN_HOST` (returned by a host closure), or `APP_FAIL`. When a non-empty line table maps the faulting opcode, `Error` carries numeric `Line`/`Column` and prefixes the message with `dsl line N:C: `; the standard JSON result includes those fields; a zero column means the line is known and only `line` is emitted. Empty line tables preserve the original message and omit the fields.

## Versioning rule

`lmcode.Version` is the bytecode format version this SDK reads. The SDK version equals that number (currently 6) and the loader refuses every other version: there is no backward compatibility. When the compiler's format changes, the compiler, this SDK, and every host move together. Version 6 retains structured `while` and `range` loops, lexical `break` and `continue`, integer and floating-point math, typed imports, and line tables.

## What load() guarantees

Wrong magic, another version (v3 or v4 included), a schema-hash mismatch, truncation, trailing bytes, unknown constant tags, undecodable instructions, out-of-range constant/local/import/context indexes, calls whose import kind or arity differ, ordinary jumps that miss an instruction start, go backwards outside a paired `LOOP_BACK` or cross a loop boundary, malformed or misnested range/while loops, unknown MATH function IDs, writes to their read-only hidden locals, a missing HALT, and context declarations or import signatures the registry does not provide (the message names the host's own signature for that name) are refused with a message that names the problem. `Run()` checks value kinds defensively and imposes no limits: no stack cap, no locals cap, no string cap; `max_stack` from the file is a reservation hint. Integer arithmetic is 64-bit two's complement and wraps; division and remainder by zero are run-time errors.

## Standard library catalog

`NewRegistry()` installs the canonical lmcode std v3 catalog (206 signatures) automatically; hosts do not need group-registration calls or handwritten std declarations. The `stdlib` package re-exports compatibility binders for opt-out registries. The exact v3 schema bytes are pinned in `std/lmcode-std.schema.json` and `std.lock`, then hashed by the loader.

The original string, math, collection, and file contracts remain. V2 added typed insertion-ordered `IntMap`/`FloatMap`; Int/Float/Str stacks and queues; stable min-first priority queues with insertion-order ties and signed-zero priority equality; and immutable `Json` values created by strict `json_parse`. V3 adds strict Base64/hex byte codecs and independent insertion-order `keys()`/`values()` snapshots on all three maps. JSON preserves number/string token spelling and object order in `to_json`, rejects malformed UTF-8/duplicate decoded keys/nonfinite numbers/depth over 256, and uses the shared 65,536-object run arena. Containers use the same arena and retain the documented bounds/empty errors. Strings remain raw bytes, including invalid UTF-8 and NUL; offsets are bytes and case/whitespace rules are ASCII only.

Compiler bundles can lag SDK additions. The tests verify the explicit canonical schema and registry signatures; the optional default-catalog probe reports when the installed compiler has not yet bundled the new declaration.

The detailed Go v3 semantics and verification record are in
[durable-memory/06-stdlib-catalog.md](durable-memory/06-stdlib-catalog.md).

The built-in math catalog, also described by the compatibility fragment `stdlib/math.schema-fragment.json` provides `math_abs`, `math_min`, `math_max`, `math_floor`, `math_ceil`, `math_round`, `math_pow`, and `math_sqrt`, each with `int` and `float` overloads. Integer arithmetic wraps at int64; negative integer exponents and square roots return `RUN_HOST`. Float operations preserve IEEE NaN, infinity, and signed-zero rules, with ties-away rounding.

The collections and testing catalogs are declared in `stdlib/collections.schema-fragment.json` and `stdlib/testing.schema-fragment.json`. Default std owns its arena and clears it at run entry and exit; shared-state nested/concurrent runs are rejected. Explicit `RegisterCollections` on an opt-out registry remains a manual compatibility path with an optional `*stdlib.Collections` arena. `RegisterTesting` accepts an output writer, with nil selecting stdout.

Collection results are ordinary exported objects, so a host callback can consume a returned handle directly: `list := value.Obj.(*stdlib.StrList); snapshot := list.Values()`. Map consumers use `value.Obj.(*stdlib.StrMap).Entries()` for insertion-ordered key/value snapshots.

The root `lmcode` package owns the canonical 206-signature standard surface. `NewRegistry()` enables it, `NewRegistryWithoutStd()` opts out, and `RegisterStd` is idempotent. `StdSignatures`, `HostSignatures`, and `MatchesSchemaSignatures` expose normalized declaration checks. Exact schema bytes are pinned in `std/lmcode-std.schema.json` and `std.lock`; `stdlib` remains a compatibility forwarding package.

## Build and test

Run the entire test suite (unit tests, 70 core vectors and 267 stdlib vectors):

```sh
go test ./...
```

Run the standalone conformance runner:

```sh
go run ./cmd/conformance ./conformance/vectors
go run ./cmd/conformance ./conformance/stdlib-vectors
bash scripts/sync-std.sh --verify
bash scripts/sync-vectors.sh --verify
```

Build the static example host:

```sh
CGO_ENABLED=0 go build -o bin/greeter ./examples/greeter
```

## Sibling runtimes

The same bytecode v6 and the same conformance vectors are implemented by:
- [lmcode-cpp](https://github.com/lmctl-ai/lmcode-cpp) (C++17 single-header reference)
- [lmcode-rust](https://github.com/lmctl-ai/lmcode-rust) (Rust crate reference)
- [lmcode-go](https://github.com/lmctl-ai/lmcode-go) (Go module)
- [lmcode-csharp](https://github.com/lmctl-ai/lmcode-csharp) (C#/.NET runtime)

## License

MIT License. Copyright (c) 2026 Lattice Systems LLC.

Built-in std has string, math, collections, file, containers, and JSON APIs. `file_read`,
`file_write` (create/truncate), and `file_append` preserve every content byte.
`file_exists` is true only for regular files; missing paths and directories are
false. `file_lines` splits LF bytes into an arena-owned StrList, preserving CR
and empty fields, including the final field (`"a\n"` becomes `["a", ""]`).
Relative paths use the process cwd without sandbox restrictions; Unix passes
path bytes unchanged, while Windows requires UTF-8 paths and supports long names.
Failures trap with `RUN_HOST` and `FUNCTION: PATH: OS_REASON`.
Testing is outside std: hosts declare its separate fragment and explicitly call
`stdlib.RegisterTesting(registry, writer)` for `print`/`assert_close`.

## Rule entries (R47/R48/R49, lmcode bytecode (DSLB v6))

`Program.Entries()` returns source-order metadata copies with `Name`, `Annotation`,
`Iface`, `Params` (type names) and `Returns`. An annotated rule is also independently
host-callable; ordinary source calls remain compiler-inlined. Interfaces are not
registry callbacks. Entries share the file pools, with their own code, locals,
stack metadata and source maps.

```go
result, err := program.Call(0, context, registry, []lmcode.Value{lmcode.Int(41)})
```

`Run` executes only the body. `Call` needs no prior `Run`; it fills parameter
locals from zero and returns the HALT value (`None()` for void). An optional
`*RunStats` argument has the same semantics as Run. Both use one execute path
and the same bound-registry lifetime behavior. Argument kinds are host-owned;
wrong arity is RUN_HOST `call entry 'NAME' expects N argument(s); found M`.
The loader checks parameter count/type/visibility, entry-local jumps, and HALT
depth unconditionally, even without loops. Version 6 is the only accepted format.

The pinned corpus is 70 core plus 267 standard vectors from lmcode-cpp d608ba2.
`go test ./...` covers those vectors and direct loader/API controls. See
[rule-entry notes](durable-memory/07-rule-entries.md). Samples pin independently reviewed module versions in go.mod/go.sum.

## Local gates and CI

Install Go 1.26, Node 24 and `npm install -g @lmctl-ai/lmctl`. From this repository run `lmscript test.lms` or `lmctl test.lms`; test.sh forwards to lmscript.

The release workflow is manual-only under the 2026-10-31 operator ruling; main pushes do not publish releases. A separate test workflow, maintained concurrently, can run on pushes and pull requests. The full local gate is required before landing changes; manual workflow dispatch and release remain operator-owned.

`examples/rule-entries/rules.lms` defines `@increment fn next(value: int) -> int implements Score`. The Go host selects the annotation and calls it with 41, asserting 42 before running any body. Tests also compile and execute the greeter, unit tests and both conformance catalogs. Releases contain greeter, conformance and rule-entries binaries.

## Text templates

The standard `template_render(string, Json) -> string` function renders arbitrary text with nested context, indexed lists, loops and boolean conditions. Inserted values remain literal. See the [portable contract and examples](std/template.md).

## Invocation results

The canonical standard includes 24 lmcode_return_* write imports. Bind a host-owned
ResultSink with Registry.SetResultSink before execution, retain it across body
and selected entry calls, and Reset it only before the next logical program.
Writes own their data; they do not alter entry HALT results. The registry has no
default/global sink. A missing sink is RUN_HOST, not a silently discarded write.

After successful dispatch AND application validation, explicitly call
WriteResultFrame(output, &sink). It emits a mandatory terminal null frame when
unset. On failure, emit no frame. Native libraries never print automatically;
existing diagnostic reporters and sample hosts need their own adoption changes.
ResultFrame formats binary64 with Go shortest-round-trip formatting, independently
of str(float); 0.1 + 0.2 is 0.30000000000000004. See
[ownership/numeric/framing details](durable-memory/08-result-sink.md) and the
[generated canonical reference](docs/std.md).

The full local gate also checks canonical return fixtures, declaration docs and
production-copy mutations. The return-conformance executable is a fixture host,
not a generic application dispatcher. No bytecode-version change or new Json
builder API is involved.

## Historical literal names

The documentation calls the language lmcode. `--lmcode`, `--lmcode-schema` and `--lmcode-signatures` are current host flag literals; `DSLB` is the bytecode magic and `DSLB v6` is the current format name; `.dslb` is the current bytecode filename extension. Other code-formatted API names, paths, manifest names and diagnostic text retain their current literal spellings.
