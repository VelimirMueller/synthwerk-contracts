#!/usr/bin/env bash
# Generates the Go and TS clients from the specs. CI runs it and fails on drift.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
for config in codegen/oapi-codegen.*.yaml; do
  name=$(basename "$config" .yaml)
  name=${name#oapi-codegen.}
  spec=openapi/_shared/$name.yaml
  [[ -f "$spec" ]] || spec=openapi/$name/openapi.yaml
  go tool oapi-codegen -config "$config" "$spec"
done
pnpm run --silent gen:ts
