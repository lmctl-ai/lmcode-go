# Standard library catalog v2

## Canonical schema and registration

`std/lmcode-std.schema.json` is copied byte-for-byte from lmcode-cpp and pinned
by the matching v2 `std.lock`. `bash scripts/sync-std.sh --verify` checks the
lock version, schema path, and SHA-256. The default `lmcode.NewRegistry()` now
registers all **171 signatures**; `NewRegistryWithoutStd()` remains empty, and
`RegisterStd` remains idempotent. The optional testing catalog is still a host
choice and does not count as std.

V2 adds `IntMap` and `FloatMap`; nine Int/Float/Str Stack, Queue, and priority
queue types; and immutable `Json` values created by `json_parse`. The root
package owns these binders so default registration does not create a
core-to-`stdlib` import cycle. The `stdlib` package re-exports the types and
explicit group binders for compatibility and opt-out registries.

## Container contracts

- IntMap/FloatMap keys are byte-exact Go strings. Overwrite retains the first
  insertion position; remove then reinsert appends the key. `key_at` follows
  insertion order and out-of-range access traps with the canonical method
  message.
- Stack `at(0)` is the next value to pop (the most recently pushed); Queue
  `at(0)` is the next FIFO value. `at` does not consume. `foreach` uses the
  normal snapshot-length/live-`at` VM protocol.
- Priority queues are min-first and stable by insertion sequence. `-0` and
  `+0` priorities compare equal; ties retain insertion order. Items may contain
  any float payload, but priorities must be finite. A rejected priority or
  exhausted int64 insertion sequence leaves the queue unchanged. `clear`
  resets the sequence.
- Every new constructor calls the same `Collections.object` allocator as the
  original lists/maps. Default std uses one 65,536-object arena per non-nested
  run; JSON child handles borrow nodes and do not allocate additional Json
  objects. `Json.keys()` allocates a StrList in that same arena.

## JSON contract

The Go SDK uses a strict parser instead of `encoding/json`: it must retain
source token spelling and insertion order, reject duplicate decoded keys, and
report byte offsets consistently with the C++ reference. It validates UTF-8
before token parsing, rejects a leading BOM, unescaped controls, malformed
escapes and unpaired surrogates, and accepts escaped NUL. JSON has no comments
or trailing commas. Container nesting is limited to 256, with the root
container at depth 1.

Integer tokens must fit int64. Decimal/exponent tokens parse as binary64;
nonfinite overflow is rejected while underflow and signed zero are preserved.
The Json tree is immutable. `kind`, `len`, `at`, `get`, `has`, `keys`,
`as_string`, `as_int`, `as_float`, `as_bool`, and `to_json` mirror the C++ API.
`to_json` emits compact syntax while preserving raw string/number tokens and
object key order, including for borrowed child nodes.

The parser's RUN_HOST messages and positions are covered by the shared vectors;
examples include `Json.get: missing key`, `Json.at: expected array`,
`json_parse: duplicate object key at byte N`, and
`json_parse: nonfinite number at byte N`. `ParseJSON` and the immutable `JSON`
type are also exposed for host callbacks.

## Go implementation

The default registry passes its single `Collections` arena to both
`RegisterContainers` and `RegisterJSON`. Maps use ordered entry slices plus a
byte-key index; queue storage uses a head cursor with periodic compaction; PQs
use Go's heap interface and a unique int64 sequence for stable ties. Map,
container, JSON-root, and `Json.keys()` allocations all charge the shared
65,536-object budget.

The optional `stdlib` package aliases the exported Go types and forwards the
two group binders. `JSON` exposes immutable typed accessors and `ToJSON()` to
host callbacks; DSL child handles point to the same tree nodes and do not
consume arena slots. No third-party Go modules or CGO are used.

## Verification and compiler status

The synchronized `conformance/stdlib-vectors` directory has 229 cases; the
core bytecode directory has 51. `go test ./...` checks both lock digests,
compares all registry signatures with the canonical schema, and runs both
vector suites. The test runner additionally compares the registry with
`lmscript --dsl-signatures` when a compiler is available.

Published npm lmscript `0.2.426` still returns 72 default signatures for an
empty host, though its explicit-schema command sees all 171 declarations. A
clean scratch build of lmctl-mono `origin/main` at `0ce49614a3236b88495652c7c892bba82bb3551d`
returns 171 for the empty-host probe. Setting `LMSCRIPT_BIN` to that scratch
compiler makes `TestCompilerDefaultCatalogParity` require exact 171-signature
parity; without it, the test skips only that default-bundle probe while still
checking the explicit v2 schema surface.

The vector pin is lmcode-cpp commit
`0ea7cb9eb5806ee7f62cd17bb01b8580ec373adf`. Do not edit the vendored expected
outputs here; sync a reviewed canonical commit with `scripts/sync-vectors.sh`.
