# Standard REST Endpoints & Stateless CLI Interfaces (`lmcode_std_v3` 226 Signatures)

## Summary

`lmcode-go` expands `lmcode_std_v3` from 206 to 226 canonical signatures by integrating `lmcode_std_rest_v1` into `RegisterStd` (`std_rest.go`, `rest/controller.go`, `rest/router.go`, `std/lmcode-std.schema.json`):
- **10 Standard Interfaces**: `RestGet`, `RestPost`, `RestPut`, `RestDelete`, `RestPatch`, `RestHead`, `RestOptions`, `RestHandler`, `CliCommand`, `CliOption`.
- **20 Standard Signatures**: `rest_config`, `rest_route`, `rest_serve`, `rest_stop`, `rest_cli`, and 15 `RestExchange` methods.
- **Route Templates & Stateless CLI**: Supports `:param` / `:rest...` placeholders alongside `{param}` / `{rest...}`, `rest_config`, and `rest_cli(argv)`.
