---
title: Health API
slug: health-api
status: beta
epic: E1
owner: contracts
paths: ["openapi/_shared/health.yaml", "codegen/oapi-codegen.health.yaml"]
api: ["GET /healthz", "GET /readyz"]
events: []
mcp: []
updated: 2026-10-21
---

# Health API

> **In 30 seconds**
> - Every Synthwerk service serves `GET /healthz` and `GET /readyz`. Pulse reads both.
> - The spec is `openapi/_shared/health.yaml`. CI generates a Go client and a TS client from it.

## What it does

- `/healthz` tells that the process runs. It does no dependency checks.
- `/readyz` tells that the service can do its job. It lists one result per dependency.
- `ok` and `degraded` answer 200. `down` answers 503.
- Go: package `github.com/VelimirMueller/synthwerk-contracts/go/health` (oapi-codegen types and client).
- TS: `createHealthClient` in `@synthwerk/contracts` (openapi-typescript types, openapi-fetch client).

## How to use

1. Serve both paths in the service. Return the `Liveness` and `Readiness` shapes.
2. Call them from Go with `health.NewClientWithResponses(baseURL)`.
3. Call them from TS with `createHealthClient({ baseUrl })`.

## Rules and limits

| Rule | Value |
|---|---|
| `/healthz` answer time | < 10 ms |
| `/readyz` answer time | < 1 s. Pulse times out at 3 s. |
| Checks per answer | max 32 |
| `detail` | max 256 characters. No secrets, no personal data. |
| Token | none. Both paths are on the Spectral allow-list. |

## Events

| Type | When |
|---|---|
| None. | |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 503 | none (body is `Readiness`) | A critical check is `down`. |

## Tests

- `tests/contract/health_test.go` — `GetLiveness - service runs - status ok`
- `tests/contract/health_test.go` — `GetReadiness - one check degraded - 200 with status degraded`
- `tests/contract/health_test.go` — `GetReadiness - critical check down - 503 with status down`

## Changes

| Date | Change |
|---|---|
| 2026-10-21 | First version. |
