# Tests and releases

Root test.lms drives Go unit tests, both conformance catalogs and compiled greeter/annotated Score examples. All binaries use CGO_ENABLED=0. Install Go 1.26 and current npm @lmctl-ai/lmctl. CI runs on main pushes, PRs and manual dispatch across five platforms; successful main builds publish greeter, conformance and rule-entries executables with licenses/checksums.

Release tags use v0.6.N because the Go module import path has no /v6 suffix; bytecode format v6 is independent of module major version.
