#!/usr/bin/env bash
# Fails when the contracts in HEAD break a consumer of the contracts in BASE.
#   API:    every published spec must still exist, and oasdiff must find no ERR change.
#           rules/oasdiff-levels.txt lowers a new response enum value to a warning (plan §5.4).
#   Events: tools/eventcompat (rules in docs/features/breaking-checks.md).
#
# Usage: scripts/check-breaking.sh <base> [<head-dir>]
#   <base>      a git ref (for example origin/main) or a dir that holds openapi/ and events/
#   <head-dir>  default: the repo root
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
base=$1
head=$(cd "${2:-$root}" && pwd)

if [[ ! -d "$base" ]]; then
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  git -C "$root" archive "$base" | tar -x -C "$tmp"
  base=$tmp
fi
base=$(cd "$base" && pwd)
cd "$root"

status=0
if [[ -d "$base/openapi" ]]; then
  while IFS= read -r -d '' spec; do
    rel=${spec#"$base"/}
    if [[ ! -f "$head/$rel" ]]; then
      echo "BREAKING $rel: spec removed"
      status=1
      continue
    fi
    echo "oasdiff: $rel"
    go tool oasdiff breaking --fail-on ERR --severity-levels rules/oasdiff-levels.txt "$spec" "$head/$rel" || status=1
  done < <(find "$base/openapi" -name '*.yaml' -print0 | sort -z)
fi

go run ./tools/eventcompat/cmd/eventcompat -base "$base/events" -head "$head/events" || status=1

if [[ $status -ne 0 ]]; then
  echo "check-breaking: FAIL. Add a new path /v<N+1>/... or a new event type .v<N+1> instead (docs/naming.md)."
fi
exit $status
