# REST server over stdio (`rest` std module)

Opt-in module of [spec/std-rest.md](https://github.com/lmctl-ai/lmcode/blob/main/spec/std-rest.md)
(`lmcode_std_rest_v1`, 18 signatures). Nothing is registered by `lmcode.NewRegistry()`; import the `rest`
package and call `rest.Register`. There is no network listener: requests are read from an input stream
(default stdin), responses are written to an output stream (default stdout).

```go
import (
	lmcode "github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/rest"
)

registry := lmcode.NewRegistry()
server, err := rest.Register(registry, rest.Options{
	MaxBodyBytes:   1 << 20,                                   // 413 above this Content-Length (default 1 MiB)
	OnHandlerError: func(entry, msg string) { log.Println(entry, msg) }, // default: one line on stderr
})
program, err := lmcode.Load(bytecode, hostSchemaBytes, registry) // host schema = lmcode-rest.schema.json (+ your own)
err = server.Run(program, lmcode.NewContext())                   // runs the body, then serves until EOF / rest_stop()
```

The program calls `rest_route("GET", "/users/{id}", "user")` for each endpoint (the annotation names an
`@user fn ... implements RestHandler` entry) and then `rest_serve()`, which validates and returns; serving starts when
the body ends. Handlers run one at a time. While serving, stdout belongs to the protocol: do not register `print` on it.

* `server.Dispatch(rest.Request{...})` is the transport-independent core used by the unit tests; `server.Stop()` ends serving like `rest_stop()`.
* `cmd/rest-serve` is a tiny host: `rest-serve PROGRAM.dslb HOST.schema.json [--max-body-bytes N]` (exit 0 after serving, 2 usage, 3 load/run error).
* Tests: `go test ./rest` (unit, plus the shared suite replayed through in-memory streams and through `cmd/rest-serve`'s real stdin/stdout).
  The shared suite lives in `conformance/rest-suite`, pinned by `rest-suite.lock`; `scripts/sync-rest.sh` re-copies it from
  `../lmcode` (or `$LMCODE_SPEC_DIR`), `scripts/sync-rest.sh --verify` checks the copy.
