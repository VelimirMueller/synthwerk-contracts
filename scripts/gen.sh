#!/usr/bin/env bash
# Generates the Go and TS clients from the specs. CI runs it and fails on drift.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
shopt -s nullglob
configs=(codegen/oapi-codegen.*.yaml)
[[ ${#configs[@]} -gt 0 ]] || { echo "gen: no codegen/oapi-codegen.*.yaml" >&2; exit 1; }
for config in "${configs[@]}"; do
  name=$(basename "$config" .yaml)
  name=${name#oapi-codegen.}
  spec=openapi/_shared/$name.yaml
  [[ -f "$spec" ]] || spec=openapi/$name/openapi.yaml
  go tool oapi-codegen -config "$config" "$spec"
done
pnpm run --silent gen:ts
