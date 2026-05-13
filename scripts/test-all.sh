#!/usr/bin/env bash
# Run `go test ./...` in every module listed in go.work.
set -euo pipefail

cd "$(dirname "$0")/.."

# Extract module paths from go.work `use (...)` block.
mods=$(awk '
  /^use \(/ { inblock=1; next }
  inblock && /^\)/ { inblock=0; next }
  inblock { gsub(/^[[:space:]]+|[[:space:]]+$/, ""); if ($0 != "") print $0 }
' go.work)

if [[ -z "$mods" ]]; then
  echo "no modules listed in go.work — nothing to test"
  exit 0
fi

fail=0
while IFS= read -r mod; do
  echo "==> $mod"
  ( cd "$mod" && go test ./... ) || fail=1
done <<< "$mods"

exit "$fail"
