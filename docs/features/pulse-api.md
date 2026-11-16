---
title: Pulse API
slug: pulse-api
status: beta
epic: E8
owner: contracts
paths: ["openapi/pulse/**", "codegen/oapi-codegen.pulse.yaml"]
api: ["GET /v1/health", "GET /v1/health/stream", "GET /v1/metrics", "GET /v1/orgs/{orgId}/metrics", "GET /v1/alerts", "GET /v1/public-stats"]
events: []
mcp: []
updated: 2026-11-16
---

# Pulse API

> **In 30 seconds**
> - `synthwerk-pulse` gives the live status, the platform and org numbers, the AI cost per day and the active alerts (synthwerk#57).
> - `GET /v1/public-stats` gives the public numbers of the site without a token (synthwerk#47).
> - The spec is `openapi/pulse/openapi.yaml`. CI generates a Go client and a TS client from it.
> - The caller is the studio BFF with the access token of the session. The browser never calls pulse.

## What it does

- `GET /v1/health` returns every target with its status. `GET /v1/health/stream` sends the same data as Server-Sent Events: a `snapshot` first, then one `status` event per change.
- `GET /v1/metrics` returns the platform numbers: services at the edge, host, database, bus, AI numbers and AI cost per day.
- `GET /v1/orgs/{orgId}/metrics` returns the AI numbers and the AI cost per day of one org only.
- `ai_cost` lists the last 7 UTC days, oldest first. Days without answers have zeros.
- `GET /v1/alerts` returns the active alerts. The rules are `target_down`, `service_errors` and `ai_failed`.
- An alert can have a trace link: `trace_id` and `trace_url`. The link opens one failed request in the trace UI.
- A part that pulse cannot read is `null`. The other parts stay.
- Go: package `github.com/VelimirMueller/synthwerk-contracts/go/pulse`. TS: `createPulseClient`, `Alert` and `AiCost` in `@synthwerk/contracts`.

### Public numbers (synthwerk#47)

- `GET /v1/public-stats` needs no token. It holds aggregates only: no org id and no user data.
- `uptime_ratio` is the share of target-minutes that were not `down` in the last 30 UTC days.
- `p95_ms` is the p95 of the edge requests in the last 24 h, without `pulse`.
- `answered_chats` is the number of completed llm answers in the last 30 UTC days.
- A number that pulse cannot read is `null`.
- The answer has `Cache-Control: public, max-age=60`. Pulse computes it at most once per 60 s.
- The studio site reads it through its own server route. The browser never calls pulse.

## Who may call

| Route | operator | owner, admin | member | no token |
|---|---|---|---|---|
| `GET /v1/health`, `GET /v1/health/stream`, `GET /v1/alerts` | 200 | 200 | 403 | 401 |
| `GET /v1/metrics` | 200 | 403 | 403 | 401 |
| `GET /v1/orgs/{orgId}/metrics`, own org | 200 | 200 | 403 | 401 |
| `GET /v1/orgs/{orgId}/metrics`, other org | 200 | 403 | 403 | 401 |

- `operator` counts only when the grant is in the platform org of pulse.
- `trace_id` and `trace_url` are for operators only. Other callers get `null`. A trace can hold data of every org.

## How to use

1. Call pulse from Go with `pulse.NewClientWithResponses(baseURL, pulse.WithRequestEditorFn(...))`. The editor sets `Authorization: Bearer <access token>`.
2. Call it from TS with `createPulseClient({ baseUrl, headers: { authorization: ... } })`. Server side only.
3. Read the stream with `parseAs: "stream"`. Apply the `snapshot`, then each `status` event.
4. Show `trace_url` as a link only when it is an `http` or `https` URL.

## Rules and limits

| Rule | Value |
|---|---|
| `orgId` | `^[A-Za-z0-9_-]{1,64}$`, the Zitadel org id |
| `trace_id` | `^[0-9a-f]{32}$` or `null` |
| `ai_cost.days` | Exactly 7 UTC days |
| `ai_cost.currency` | ISO 4217 code of the pulse price table, default `USD` |
| Alerts | Max 100. Critical first, then the oldest `since`. |
| Open streams | Max 50 per pulse process. Then 503 with `Retry-After: 5`. |
| Cache | `Cache-Control: no-store` on every `/v1` answer |

## Events

| Type | When |
|---|---|
| None. Alerts are not events yet. | |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 401 | `about:blank`, code `unauthenticated` | No Bearer token, or the token is not valid. |
| 403 | `about:blank`, code `forbidden` | The role does not allow the route or the org. |
| 503 | `about:blank`, code `auth_unavailable` | Pulse cannot load the token keys. |
| 503 | None (plain text) | `GET /v1/health/stream`: the max number of streams is open. |

## Tests

- `tests/contract/pulse_test.go` — `Snapshot - spec example - decodes strictly, checked_at null before the first check`
- `tests/contract/pulse_test.go` — `StatusStreamEvent - snapshot and status examples - decode by the event name`
- `tests/contract/pulse_test.go` — `PlatformMetrics - spec example - decodes strictly with 7 cost days`
- `tests/contract/pulse_test.go` — `Alerts - spec example - critical first, trace link only with a trace id`
- `tests/contract/pulse_test.go` — `Problem - every response example - decodes strictly with about:blank`
- `tests/contract/pulse_test.go` — `GetOrgMetrics - client - sends the bearer to the org path`
- `tests/contract/pulse_test.go` — `ListAlerts - 403 - typed problem body`
- `tests/contract/pulse_test.go` — `PublicStats - spec example - decodes strictly, aggregates only`
- `tests/contract/pulse_test.go` — `GetPublicStats - client - calls the path without a token`

## Open points

- An alert event (raised, resolved) for notifications.

## Changes

| Date | Change |
|---|---|
| 2026-11-16 | First version: status, stream, platform and org numbers with AI cost per day, active alerts with trace links (synthwerk#57). Spec version 0.1.0. |
| 2026-11-16 | Add `GET /v1/public-stats` without a token: uptime, p95 and answered chats for the public site (synthwerk#47). Spec version 0.2.0. |
