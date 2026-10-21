#!/usr/bin/env bash
# Proves the Spectral ruleset: tests/fixtures/spectral/violations.yaml must trip every custom rule.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
out=$(pnpm exec spectral lint --ruleset rules/spectral.yaml --format text tests/fixtures/spectral/violations.yaml 2>&1 || true)
echo "$out"

fail=0
expect() {
  grep -q "$1" <<<"$out" || { echo "selftest: FAIL. No finding: $1"; fail=1; }
}
for rule in $(grep -oE '^  synthwerk-[a-z-]+' rules/spectral.yaml); do
  expect "$rule"
done
# Two security findings: the missing list and the empty list (security: []).
[[ $(grep -c "synthwerk-security" <<<"$out") -eq 2 ]] || { echo "selftest: FAIL. security: [] passed."; fail=1; }
[[ $fail -eq 0 ]] && echo "selftest: OK. Every custom Spectral rule fires."
exit $fail
