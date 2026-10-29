#!/usr/bin/env bash
set -euo pipefail
"${LMSCRIPT_BIN:-lmscript}" --no-preflight examples/rule-entries/rules.lms bin/rule-entries.dslb
"${LMSCRIPT_BIN:-lmscript}" --no-preflight examples/greeter/build.lms bin/greeter.dslb
