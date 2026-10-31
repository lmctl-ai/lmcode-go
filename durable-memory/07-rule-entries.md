# Rule entries, lmcode bytecode (DSLB v6) (2026-10-28)

R49 is authoritative (mono 0e621804 `requirements/dsl-bytecode.md` and `design/dsl-language.md`).
Version 6 only. Entry metadata: Name, Annotation, Iface, Params, Returns. Entries()
returns copies in source order. Call(index, ctx, registry, args, optionalStats)
shares execute with Run, including bound registry, run scope, statistics and
located errors; no prior Run is required. Return is the single untyped HALT value,
or None for void. Arguments occupy locals from zero; kinds are host-owned.

Loader refuses parameter count/type mismatch, hidden parameter slots, wrong HALT
depth even without loops, malformed signatures, out-of-range local/constant indexes
and jumps outside the entry region. Errors name the entry and use LOAD_BAD_BYTECODE.
Call arity is RUN_HOST `call entry 'NAME' expects N argument(s); found M`.
Ten refusal controls were shown failing against v5 before the implementation.
API tests also check direct calls, untyped values, metadata copies, RunStats,
entry error locations, repeated calls, and void results.

Core70 and std267 vectors match C++ d608ba2 byte-for-byte and are pinned in the
existing lock files. The compiler generator's known empty-line-table mutation
truncates the v6 entry count; the C++ snapshot retains that tail. The compiler
checkout was never edited. The old extra runtime-type-error vector is absent
from the authoritative70; VM type-error unit coverage remains intact.
Every conformance call runs the body once before invoking the entry; this is a
runner convention, not an API requirement. The runner uses encoding/json with
UseNumber to preserve int64 host arguments and compares full result/trace lengths.

Gates: LMSCRIPT_BIN=<v6 compiler> go test ./...; go vet ./...;
CGO_ENABLED=0 go build ./...; go run ./cmd/conformance conformance/vectors;
same runner on conformance/stdlib-vectors; examples/greeter/test.sh with the v6
compiler bin first on PATH. This repo has no matrix script; C++'s sibling matrix
builds checkouts owned by other teams and requires coordinated execution.
