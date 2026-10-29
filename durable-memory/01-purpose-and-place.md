# Purpose and place

lmcode-go is the Go runtime and SDK for lmscript `_lmcode_` bytecode. The native Go application remains the owner of process state, memory, and domain objects; the DSL supplies typed configuration and sequencing inside that program.

The schema is the host boundary. It names context objects, methods, functions, conversions, operators, and enums. The runtime does not infer domain behavior from the schema.

The SDK is designed to be directly imported (`import "github.com/lmctl-ai/lmcode-go"`) and compiled with `CGO_ENABLED=0` into static binaries.
