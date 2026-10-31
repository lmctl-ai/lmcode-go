# Invocation results (amendment R3)

The canonical std v3 catalog keeps 206 signatures and 24 return imports. DSLB
stays version 6. Canonical C++ R1 bytecodes and exact frames are pinned through
scripts/sync-vectors.sh; scripts/sync-std.sh mirrors schema and generated docs.
No compatibility reporter or sink-only framing API remains.

InvocationResultsFromCommand reads the private environment descriptor; explicit
InvocationResultsFromManifest and InvocationResultsFromMetadata constructors
support embedding/testing without changing process environment. Missing env uses
CLI occurrence order, including duplicates. Present empty env is invalid. Strict
JSON, exact keys, contiguous safe indices, absolute unique metadata files and a
complete registration permutation are required. Returned registrations are copies.
The manifest never supplies result contracts. Introspection must bypass this reader.

One ResultSink per slot spans body and entries. Select binds it through the registry;
reselection neither resets nor changes its contract. Complete validates and seals
an owned snapshot. Writes, reset, reselection and second completion then fail.
Do not copy sinks/collectors or concurrently operate on them; the host's registry
synchronization remains authoritative. New invocations create new collectors.

ResultContractFromSchema reads schema 2's optional result declaration. Supported
types are null, bool, int, float, string, Json, StrList/IntList/FloatList and
StrMap/IntMap/FloatMap. Nullable defaults false; type null forbids nullable.
No declaration allows any valid JSON. Initial/reset null is unfinished state;
explicit null and Complete validate it. Homogeneous elements cannot be null.
Known int and integer collections widen to their float counterparts; typed empty
collections retain their source distinction. Json handles defer shape checking.
Writes copy and validate before replacing state, including field writes.

All earlier numeric/copy rules remain: UTF-8, no duplicate JSON keys, safe integer
bounds, nonfinite/underflow refusal, pointer paths and owned container snapshots.
Go uses strconv.FormatFloat(value, 'g', -1, 64), independent of str(float).
0.1 + 0.2 stays 0.30000000000000004; negative zero normalizes to zero.

Fail retains the first observed error and builder part (single/global failures
omit part). New slots cannot start after failure, but selected work may drain.
Complete type errors latch their slot before returning. Global app validation
also discards all results. A missing completion is a failure, never silent null.

WriteFrame emits exactly one canonical success/failure envelope in add order,
returns 0/2/3/4 and flushes buffered writers. A partial write or flush failure
returns 4 with best-effort stderr and cannot append a second frame. FailureFrame
covers pre-collector errors; callers must return immediately with its status.
Go os.Stdout preserves LF on Windows. Native cross-platform gates are not implied
by Linux testing. The VM itself never prints or changes HALT results.

Gates: test.lms; go vet ./...; go test -race ./...; the return and invocation
mutation scripts. No network, paid model calls or public invocation-path option
is part of this API. Host/sample adoption is the later R7 lane.
