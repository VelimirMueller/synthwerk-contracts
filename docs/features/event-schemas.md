---
title: Event schemas
slug: event-schemas
status: beta
epic: E1
owner: contracts
paths: ["events/**"]
api: []
events: ["dev.synthwerk.pulse.target.status_changed.v1", "dev.synthwerk.llm.toolcall.executed.v1"]
mcp: []
updated: 2026-10-21
---

# Event schemas

> **In 30 seconds**
> - Every Synthwerk event is a CloudEvent 1.0.2. `events/envelope.schema.json` defines the envelope.
> - Each type has a JSON Schema 2020-12 for its `data`, one example and one catalog entry.
> - This repo defines events. It publishes none.

## What it does

- `events/envelope.schema.json`: the CloudEvents attributes, the Synthwerk rules and the extensions `tenantid`, `traceparent`, `actor`.
- `events/catalog.yaml`: per type the subject, stream, producer, consumers, status, schema and example.
- `events/<domain>/<entity>.<verb>.v<N>.schema.json`: the `data` schema of one type.
- `events/examples/<type>.json`: one full, valid event per type.

## How to use

1. Add the schema file and the example. Follow `docs/naming.md`.
2. Add the catalog entry with status `draft`.
3. Run `just test-contract`. It validates the example against the envelope and the data schema.

## Rules and limits

| Rule | Value |
|---|---|
| Lifecycle | `draft` → `active` → `deprecated` → `retired` |
| Unknown fields | rejected (`additionalProperties: false`) |
| Personal data | no email, IP, prompt or content in `data`. `actor` holds a subject id only. |
| Breaking change | new type `.v<N+1>`, see `docs/features/contract-checks.md` |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.pulse.target.status_changed.v1` | Pulse sees a new readiness status of a target. |
| `dev.synthwerk.llm.toolcall.executed.v1` | Audit fact (D-26): the AI called one read-only MCP tool. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| None. | | |

## Tests

- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`
- `tests/contract/events_test.go` — `Catalog - schema file not listed - fails`
- `tests/contract/events_test.go` — `envelope - type without version suffix - rejected` (and 4 more reject cases)

## Changes

| Date | Change |
|---|---|
| 2026-10-21 | First version: envelope, catalog, pulse status change, LLM tool-call audit fact. |
