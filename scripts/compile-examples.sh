#!/usr/bin/env bash
# Maintainer fixture generation uses compiler-owned bytes, not a public export API.
set -euo pipefail
node scripts/compile-fixture.mjs examples/rule-entries/rules.lms bin/rule-entries.dslb
node scripts/compile-fixture.mjs examples/greeter/build.lms bin/greeter.dslb
