---
title: Go service kit
slug: go-service-kit
status: beta
epic: E1
owner: contracts
paths: ["go/kit/**", "go/examples/**"]
api: ["GET /healthz", "GET /readyz"]
events: ["dev.synthwerk.pulse.target.status_changed.v1"]
mcp: []
updated: 2026-10-23
---

# Go service kit

> **In 30 seconds**
> - Shared Go packages for identity, llm and pulse: health probes, JSON logs, OpenTelemetry, CloudEvents, the transactional outbox and JWT auth.
> - Module `github.com/VelimirMueller/synthwerk-contracts/go`, folder `go/kit/`. An example service in `go/examples/service/` wires every package.
> - Integration tests prove the outbox against a real Postgres and NATS JetStream in CI.

## What it does

| Package | Gives |
|---|---|
| `kit/health` | `GET /healthz` and `GET /readyz` in the shape of `openapi/_shared/health.yaml`. Uses the generated types of `go/health`. |
| `kit/log` | `log/slog` JSON logger with the shared fields, trace ids and a filter for forbidden keys. HTTP middleware for the request line and `X-Request-ID`. |
| `kit/otel` | Tracer, meter and logger providers. OTLP over gRPC only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. W3C `traceparent` propagation. |
| `kit/cloudevents` | Builds and checks CloudEvents 1.0.2 events: UUIDv7 id, `source`, `orgid`, `traceparent`, NATS subject from the type. |
| `kit/outbox` | Outbox table, `Enqueue` in the caller's transaction, and the relay to JetStream with `Nats-Msg-Id` = event id. |
| `kit/env` | Reads `SYNTHWERK_<SERVICE>_*` and `SYNTHWERK_PLATFORM_ENV` (D-40). |
| `kit/auth` | Verifies Zitadel access tokens against the issuer JWKS (D-20, D-23). Maps claims to a `Principal` (subject, org, roles, scopes). HTTP middleware and `RequireRole`, `RequireOrg`, `RequireScope`, `RequireMFA`. |
| `kit/auth/authtest` | Fake issuer for tests: serves discovery and a JWKS over `httptest`, mints signed tokens. |

- `/readyz` answers 503 `down` until `MarkStarted`. Then it runs every check in parallel, each with its own timeout (default 300 ms, all within 900 ms).
- A failed critical check gives `down` (503). A failed optional check gives `degraded` (200). `detail` is constant text, never the error.
- The relay locks rows with `FOR UPDATE SKIP LOCKED`, publishes, and marks them in one database transaction. Several relays can run at once.
- A crash after the publish and before the commit leaves the row unpublished. The next run publishes it again with the same `Nats-Msg-Id`. JetStream drops the copy, so a consumer gets the event once.
- `auth.Verifier` allows RS256 and ES256 only. It checks `iss`, `aud` (any of a list), `exp` (required), `nbf` and `iat` with 30 s leeway (max 60 s).
- The JWKS cache refreshes after 15 min, and at once on an unknown `kid`, at most once per 10 s. When a refresh fails, the last good keys stay in use.
- `ZitadelMapper` keeps a role only when the roles claim grants it in the token's org. `operator` counts only when granted in `PlatformOrgID`. A token without `sub` or org id is refused.
- ID tokens never pass as access tokens. The header `typ` must be on an allow-list. A token with `nonce` or `at_hash` is refused.
- Claim names and the token shape are verified against Zitadel v4.20.0 (spike SW-M2-0.5, 2026-10-23). They live in `go/kit/auth/zitadel.go` only.
- Zitadel `aud` is `[client id, project id]`. Set `Audiences` to the project id.
- The org id claim exists only with the scope `urn:zitadel:iam:user:resourceowner`. Without it the token is refused (401).
- Zitadel access tokens have `client_id`, no `azp` and no `scope` claim. `Principal.ClientID` reads `client_id`, else `azp`. `Principal.Scopes` stays empty until an Action sets `synthwerk:scope`.
- `RequireMFA` passes when `synthwerk:mfa` is the boolean `true`, or when `amr` contains `mfa`. Zitadel enforces Force MFA only in its login UI, so admin and platform routes check the token themselves.
- Zitadel v4.20.0 access tokens carry no `amr` claim. synthwerk-identity adds `synthwerk:mfa: true` with a pre-access-token Action when the user has a second factor (TOTP, U2F, OTP) or a passkey (D-47). Zitadel gives the Action no session data, so the claim says what the user has enrolled, not what this sign-in used. Force MFA makes the login UI ask for that factor at every sign-in.
- The Zitadel project role key for the platform role must be `operator` (D-44), not `platform_admin`.
- `WithOrg`/`OrgID` set the `orgid` extension (D-44). `WithTenant`/`TenantID` are deprecated but keep working: they still set and check `tenantid` for consumers that read it.
- Without an OTLP endpoint the service installs no exporter and logs one `WARN` line (`otel.exporter.disabled`). Spans still get ids for the logs.

## How to use

1. Copy `go/examples/service/` into the new service repo. Rename `Service` (env prefix and `source`).
2. Add the module: `go get github.com/VelimirMueller/synthwerk-contracts/go@<version>`.
3. Run `outbox.Schema` in the service migrations, or call `outbox.Migrate` at startup.
4. In each write: begin a transaction, write the data row, call `outbox.Enqueue(ctx, tx, event)`, commit.
5. Start one `outbox.Relay` per process with `relay.Run(ctx)`.
6. Add one `health.Check` per dependency. Call `MarkStarted` when the service can serve.
7. Wrap the mux: `otelhttp.NewHandler(log.HTTP(logger, mux), service)`.
8. Auth: `auth.NewVerifier(auth.Config{Issuer, Audiences})`. Mount the API mux under `auth.Middleware(v, api)`; keep `/healthz` and `/readyz` outside. Add `v.Ready` as the `jwks` readiness check.
9. Tests: `authtest.NewIssuer(t)`, `auth.NewVerifier(iss.Config())`, `iss.Mint(t, authtest.Claims(sub, org, roles...))`.

## Rules and limits

| Rule | Value |
|---|---|
| `/readyz` check budget | 900 ms in total, 300 ms per check by default |
| Relay poll interval | 500 ms (`Config.Interval`) |
| Relay batch | 100 events per transaction (`Config.BatchSize`) |
| Publish timeout | 5 s per event (`Config.PublishTimeout`) |
| JetStream duplicate window | Must be longer than a relay restart. Default 2 min. The infra module owns the stream config. |
| Order | Kept per relay (`created_at`, `id`). Not kept across relays. |
| OTLP protocol | `grpc` only. Another `OTEL_EXPORTER_OTLP_PROTOCOL` fails at startup. |
| Log format | `json` (default). `console` only in env `local`. |
| `source` env | `local`, `dev`, `stg`, `prd`. Env `ci` publishes as `local` (the envelope has no `ci`). |
| Published rows | Stay in the table. Clean-up is not in the kit yet. |
| JWT algorithms | RS256, ES256. `none` and HS* are refused. RSA keys below 2048 bits are skipped. |
| JWT header `typ` | `at+jwt`, `application/at+jwt`, `JWT` (Zitadel v4.20.0) by default (`Config.AcceptedTypes`). No `typ` is refused unless the list has `""`. |
| ID tokens | A token with `nonce` or `at_hash` is refused, whatever its `typ`. |
| JWT leeway | 30 s default, 60 s max |
| JWKS cache | 15 min max age, refresh on unknown `kid` at most once per 10 s, 5 s fetch timeout, 1 MiB body |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.pulse.target.status_changed.v1` | The example service stores a new status for a target. Demo only: pulse owns this type. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 503 | none (body is `Readiness`) | Startup is not complete, or a critical check is `down`. |
| 400 | none (plain text) | Example service: bad target name or body. |
| 401 | `about:blank`, code `unauthenticated` | `kit/auth`: no Bearer token, or the token is refused. Header `WWW-Authenticate: Bearer`. |
| 403 | `about:blank`, code `forbidden` | `kit/auth`: role, org or scope does not match. |
| 403 | `about:blank`, code `mfa_required` | `kit/auth`: `RequireMFA` and the sign-in had no MFA. |
| 503 | `about:blank`, code `auth_unavailable` | `kit/auth`: the JWKS cannot be loaded and no key is known. |

## Tests

- `tests/integration/outbox/outbox_test.go` — `relay - transaction rolled back - no event published`
- `tests/integration/outbox/outbox_test.go` — `relay - crash after publish, then restart - consumer gets the event once`
- `tests/integration/outbox/outbox_test.go` — `relay - no stream for the subject - row kept, published after the stream exists`
- `tests/integration/outbox/outbox_test.go` — `relay - two relays at once - every event once`
- `tests/integration/example/example_test.go` — `set status - status changed - one valid event on EVT_PULSE`
- `tests/contract/kit_test.go` — `readyz - critical check fails - 503 matches schema`
- `tests/contract/kit_test.go` — `NATSSubject - every catalog type - equals catalog nats_subject`
- `tests/contract/kit_test.go` — `New - every catalog example - envelope validates`
- `tests/unit/kit/auth_test.go` — `Verify - expired token - rejects`
- `tests/unit/kit/auth_test.go` — `Verify - alg none - rejects`
- `tests/unit/kit/auth_test.go` — `Verify - unknown kid - refreshes JWKS once, then rejects`
- `tests/unit/kit/auth_test.go` — `Verify - key rotated after the first fetch - refreshes once, then accepts`
- `tests/unit/kit/auth_test.go` — `RequireRole - member on an admin route - 403`
- `tests/unit/kit/auth_test.go` — `RequireMFA - amr pwd only - 403`
- `tests/unit/kit/auth_test.go` — `RequireMFA - synthwerk mfa true - 204`, `RequireMFA - synthwerk mfa string true - 403`
- `tests/unit/kit/auth_test.go` — `Map - Zitadel v4.20.0 access token - org roles and client id`
- `tests/unit/kit/health_test.go` — `readyz - check hangs - answers within the budget with timeout`
- `tests/unit/kit/log_test.go` — `log line - forbidden keys anywhere - keys dropped`
- `tests/unit/kit/otel_test.go` — `Setup - endpoint not set - no exporter, one warning`

## Changes

| Date | Change |
|---|---|
| 2026-10-21 | First version: health, log, otel, cloudevents, outbox, env, example service. |
| 2026-10-23 | `kit/auth` and `kit/auth/authtest`: JWT verify with a JWKS cache, Zitadel claims mapper, HTTP middleware (SW-M2-0.1). |
| 2026-10-28 | `kit/auth`: `ClaimMFA` (`synthwerk:mfa`), `Principal.MFAEnrolled`; `RequireMFA` accepts it (#28, D-47). |
| 2026-10-23 | `kit/auth`: Zitadel v4.20.0 token shape (`client_id`, `aud`, org scope), `RequireMFA` (SW-M2-0.5). |
| 2026-10-23 | D-44: `kit/cloudevents` adds `WithOrg`/`OrgID`; `WithTenant`/`TenantID` deprecated, still working. |
