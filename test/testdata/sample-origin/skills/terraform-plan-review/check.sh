#!/usr/bin/env bash
# A sample executable helper shipped inside the skill. Its 0o755 mode is part of
# the git tree-SHA, so it makes the mode-preservation assertions meaningful
# (without an executable fixture file the exec-bit guarantee is never exercised).
set -euo pipefail
echo "terraform-plan-review: ok"
