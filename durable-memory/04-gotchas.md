# Gotchas

- Bytecode is bound to exact schema bytes; changing whitespace or line endings in a schema changes its SHA-256 hash. Line endings in `.gitattributes` are pinned to LF across all conformance files (`*.json`, `*.dsl`, `*.expect`).
- Vector lock verification on Windows: MSYS2/Git-for-Windows `sha256sum` outputs `*` in binary mode and emits `\r\n`. `scripts/sync-vectors.sh` normalizes ` *` to `  ` and strips `\r` before the second hash stage so digest calculation is strictly platform-independent.
- Bytecode version 5 only: no backward compatibility with v3/v4; older and newer formats are refused at load time with `LOAD_BAD_BYTECODE`.
- Go integer division: Go's runtime panics on `MinInt64 / -1` and `MinInt64 % -1`. The VM explicitly intercepts these cases to match two's-complement wraparound semantics.
- String handling: never range over VM strings as runes. VM strings are raw byte sequences.
- Resource limits: the VM imposes no memory or execution time limits (no stack cap, no locals cap, no string cap). Hosts enforce domain and resource policy in their process environment.
