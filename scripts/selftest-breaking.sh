#!/usr/bin/env bash
# Proves the breaking-change gate: apply tests/fixtures/rename-required-field.patch to a copy
# of the contracts and expect scripts/check-breaking.sh to fail on the API and on the event.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp -R "$root/openapi" "$root/events" "$tmp/"
patch --quiet -p1 -d "$tmp" < "$root/tests/fixtures/rename-required-field.patch"

if out=$("$root/scripts/check-breaking.sh" "$root" "$tmp" 2>&1); then
  echo "$out"
  echo "selftest: FAIL. The gate passed a renamed required field."
  exit 1
fi
echo "$out"

fail=0
grep -q "response-required-property-removed" <<<"$out" || { echo "selftest: FAIL. oasdiff did not report the API rename."; fail=1; }
grep -q "BREAKING llm/toolcall.executed.v1.schema.json conversation_id: required property removed" <<<"$out" \
  || { echo "selftest: FAIL. eventcompat did not report the event rename."; fail=1; }
[[ $fail -eq 0 ]] && echo "selftest: OK. The gate fails on a renamed required field."
exit $fail
