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
updated: 2026-10-21
---

# Go service kit

> **In 30 seconds**
> - Shared Go packages for identity, llm and pulse: health probes, JSON logs, OpenTelemetry, CloudEvents and the transactional outbox.
> - Module `github.com/VelimirMueller/synthwerk-contracts/go`, folder `go/kit/`. An example service in `go/examples/service/` wires every package.
> - Integration tests prove the outbox against a real Postgres and NATS JetStream in CI.

## What it does

| Package | Gives |
|---|---|
| `kit/health` | `GET /healthz` and `GET /readyz` in the shape of `openapi/_shared/health.yaml`. Uses the generated types of `go/health`. |
| `kit/log` | `log/slog` JSON logger with the shared fields, trace ids and a filter for forbidden keys. HTTP middleware for the request line and `X-Request-ID`. |
| `kit/otel` | Tracer, meter and logger providers. OTLP over gRPC only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. W3C `traceparent` propagation. |
| `kit/cloudevents` | Builds and checks CloudEvents 1.0.2 events: UUIDv7 id, `source`, `traceparent`, NATS subject from the type. |
| `kit/outbox` | Outbox table, `Enqueue` in the caller's transaction, and the relay to JetStream with `Nats-Msg-Id` = event id. |
| `kit/env` | Reads `SYNTHWERK_<SERVICE>_*` and `SYNTHWERK_PLATFORM_ENV` (D-40). |

- `/readyz` answers 503 `down` until `MarkStarted`. Then it runs every check in parallel, each with its own timeout (default 300 ms, all within 900 ms).
- A failed critical check gives `down` (503). A failed optional check gives `degraded` (200). `detail` is constant text, never the error.
- The relay locks rows with `FOR UPDATE SKIP LOCKED`, publishes, and marks them in one database transaction. Several relays can run at once.
- A crash after the publish and before the commit leaves the row unpublished. The next run publishes it again with the same `Nats-Msg-Id`. JetStream drops the copy, so a consumer gets the event once.
- Without an OTLP endpoint the service installs no exporter and logs one `WARN` line (`otel.exporter.disabled`). Spans still get ids for the logs.

## How to use

1. Copy `go/examples/service/` into the new service repo. Rename `Service` (env prefix and `source`).
2. Add the module: `go get github.com/VelimirMueller/synthwerk-contracts/go@<version>`.
3. Run `outbox.Schema` in the service migrations, or call `outbox.Migrate` at startup.
4. In each write: begin a transaction, write the data row, call `outbox.Enqueue(ctx, tx, event)`, commit.
5. Start one `outbox.Relay` per process with `relay.Run(ctx)`.
6. Add one `health.Check` per dependency. Call `MarkStarted` when the service can serve.
7. Wrap the mux: `otelhttp.NewHandler(log.HTTP(logger, mux), service)`.

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

## Events

| Type | When |
|---|---|
| `dev.synthwerk.pulse.target.status_changed.v1` | The example service stores a new status for a target. Demo only: pulse owns this type. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 503 | none (body is `Readiness`) | Startup is not complete, or a critical check is `down`. |
| 400 | none (plain text) | Example service: bad target name or body. |

## Tests

- `tests/integration/outbox/outbox_test.go` — `relay - transaction rolled back - no event published`
- `tests/integration/outbox/outbox_test.go` — `relay - crash after publish, then restart - consumer gets the event once`
- `tests/integration/outbox/outbox_test.go` — `relay - no stream for the subject - row kept, published after the stream exists`
- `tests/integration/outbox/outbox_test.go` — `relay - two relays at once - every event once`
- `tests/integration/example/example_test.go` — `set status - status changed - one valid event on EVT_PULSE`
- `tests/contract/kit_test.go` — `readyz - critical check fails - 503 matches schema`
- `tests/contract/kit_test.go` — `NATSSubject - every catalog type - equals catalog nats_subject`
- `tests/contract/kit_test.go` — `New - every catalog example - envelope validates`
- `tests/unit/kit/health_test.go` — `readyz - check hangs - answers within the budget with timeout`
- `tests/unit/kit/log_test.go` — `log line - forbidden keys anywhere - keys dropped`
- `tests/unit/kit/otel_test.go` — `Setup - endpoint not set - no exporter, one warning`

## Changes

| Date | Change |
|---|---|
| 2026-10-21 | First version: health, log, otel, cloudevents, outbox, env, example service. |
