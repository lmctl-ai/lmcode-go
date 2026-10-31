# Standard library catalog v3

## Canonical schema and registration

`std/lmcode-std.schema.json` is copied byte-for-byte from lmcode-cpp and pinned
by the matching v3 `std.lock`. `bash scripts/sync-std.sh --verify` checks the
lock version, schema path, and SHA-256. The default `lmcode.NewRegistry()` now
registers all **181 signatures**; `NewRegistryWithoutStd()` remains empty, and
`RegisterStd` remains idempotent. The optional testing catalog is still a host
choice and does not count as std.

V2 added `IntMap` and `FloatMap`; nine Int/Float/Str Stack, Queue, and priority
queue types; and immutable `Json` values created by `json_parse`. The root
package owns these binders so default registration does not create a
core-to-`stdlib` import cycle. The `stdlib` package re-exports the types and
explicit group binders for compatibility and opt-out registries.

## V3 additions

The v3 catalog adds strict byte-oriented `str_base64_encode/decode` and
`str_hex_encode/decode`. Hex encoding is lowercase; decoding accepts ASCII
mixed-case hex only. Base64 decoding requires canonical padded RFC 4648 input
and rejects whitespace, URL-safe characters, malformed padding, and nonzero pad
bits. These functions preserve arbitrary bytes, including NUL and invalid UTF-8.

`StrMap`, `IntMap`, and `FloatMap` now expose `keys()` and `values()` snapshots
in insertion order. Each call allocates an independent typed list from the
shared collection arena; later map or list changes do not alias. Float values
preserve their binary64 payloads.

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
host callbacks; lmcode child handles point to the same tree nodes and do not
consume arena slots. No third-party Go modules or CGO are used.

## Verification and compiler status

The synchronized `conformance/stdlib-vectors` directory has 267 cases; the
core bytecode directory has 51. `go test ./...` checks both lock digests,
compares all registry signatures with the canonical schema, and runs both
vector suites. The test runner additionally compares the registry with
`lmscript --lmcode-signatures` when a compiler is available.

Published npm lmscript `0.2.426` returns 72 default signatures for an empty
host. A scratch build of lmctl-mono `origin/main` at `fa04890c` returns 171
default signatures; its explicit schema command recognizes all 181 v3
declarations. The Go tests verify exact compiler parity for the canonical v3
schema and skip the default-catalog probe until lmctl-mono registers all 181
functions/methods by default.

The v3 vector pin is lmcode-cpp commit
`38bbd918b7fde81db15d1d7427955cc5465d6f35`. Do not edit the vendored expected
outputs here; sync a reviewed canonical commit with `scripts/sync-vectors.sh`.
