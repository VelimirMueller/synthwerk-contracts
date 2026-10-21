---
title: Contract checks
slug: contract-checks
status: beta
epic: E1
owner: contracts
paths: ["rules/**", "tools/**", "scripts/**"]
api: []
events: []
mcp: []
updated: 2026-10-21
---

# Contract checks

> **In 30 seconds**
> - Spectral lints every spec. oasdiff and `eventcompat` fail a PR that breaks a consumer.
> - A selftest proves the gate: a renamed required field must fail it.

## What it does

- `rules/spectral.yaml`: names, security and size rules for every spec in `openapi/`.
- `scripts/check-breaking.sh <base>`: oasdiff on every published spec, `eventcompat` on every event schema.
- `tools/eventcompat`: compares two versions of the event schemas. Rules are in the table below.
- `scripts/selftest-breaking.sh`: applies `tests/fixtures/rename-required-field.patch` and expects the gate to fail.
- `scripts/selftest-spectral.sh`: lints `tests/fixtures/spectral/violations.yaml` and expects every custom rule to fire.

## How to use

1. Run `just lint` and `just test` before you push.
2. Run `just breaking origin/main` to check your branch against `main`.
3. Breaking on purpose? Add `/v<N+1>/` or `.v<N+1>`. Do not edit the published version.

## Rules and limits

| Change | API (oasdiff) | Event schema (eventcompat) |
|---|---|---|
| Add optional field or endpoint | passes | passes |
| Add required field | fails (request) | fails |
| Remove or rename field | fails | fails |
| Make a required field optional | fails (response) | fails |
| Change type, format or const | fails | fails (also `$ref`, `prefixItems`) |
| Add enum value | warns (`rules/oasdiff-levels.txt`) | warns |
| Remove enum value | fails | fails |
| Drop the whole enum | fails | warns (consumers must tolerate any value) |
| Remove spec or schema file | fails | fails |

| Spectral rule | Checks |
|---|---|
| `synthwerk-operation-id` | `<service>.<verb><Noun>` |
| `synthwerk-path` | `/v<N>/<plural-noun>`, kebab-case, `:verb`; `/healthz`, `/readyz` allowed |
| `synthwerk-security` | non-empty `security` on each operation, except `/healthz`, `/readyz` |
| `synthwerk-string-max-length` | `maxLength` on every string without `enum` or `const` |
| `synthwerk-array-max-items` | `maxItems` on every array |
| `synthwerk-property-snake-case` | schema properties in `snake_case` |

## Events

| Type | When |
|---|---|
| None. | |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| None. | | |

## Tests

- `tests/unit/eventcompat/eventcompat_test.go` — one case per rule, for example `Compare - required property renamed - breaking`
- `scripts/selftest-breaking.sh` — `check-breaking - required field renamed - fails on API and event`
- `scripts/selftest-spectral.sh` — `spectral - violations fixture - every custom rule fires`

## Changes

| Date | Change |
|---|---|
| 2026-10-21 | First version. |
