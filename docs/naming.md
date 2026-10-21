# Naming rules

> **In 30 seconds**
> - Every event type is `dev.synthwerk.<domain>.<entity>.<verb>.v<N>`. The verb is in the past tense.
> - The NATS subject, the stream, the schema file and the schema `$id` all derive from the type.
> - A breaking change never edits a published name. It adds `.v<N+1>` or `/v<N+1>/`.
> - `tests/contract` and `rules/spectral.yaml` check these rules in CI.

## Events (CloudEvents 1.0.2)

| Item | Rule | Example |
|---|---|---|
| `type` | `dev.synthwerk.<domain>.<entity>.<verb>.v<N>` | `dev.synthwerk.llm.toolcall.executed.v1` |
| `<domain>` | Service that produces the fact. Lowercase letters and digits. | `llm` |
| `<entity>` | Thing the fact is about. Singular, `snake_case`. | `toolcall`, `label_set` |
| `<verb>` | What happened. Past tense, `snake_case`. | `executed`, `status_changed` |
| `v<N>` | Major version of the data schema. Starts at `v1`. | `v1` |
| NATS subject (catalog `nats_subject`) | `evt.` + the type without `dev.synthwerk.` | `evt.llm.toolcall.executed.v1` |
| Stream | `EVT_<DOMAIN>` for facts, `JOBS_<DOMAIN>` for work queues | `EVT_LLM` |
| `source` | `/synthwerk/<service>/<env>`, env is `local`, `dev`, `stg` or `prd` | `/synthwerk/llm/prd` |
| `subject` | Id of the main entity | conversation id |
| `id` | UUIDv7. Also the `Nats-Msg-Id` for dedupe. | `0199f3a4-1d2c-7e3f-…` |
| Extensions | `tenantid`, `traceparent`, `actor`. Lowercase, no other names. | `actor: user:2837…` |
| Data fields | `snake_case` | `conversation_id`, `duration_ms` |
| Schema file | `events/<domain>/<entity>.<verb>.v<N>.schema.json` | `events/llm/toolcall.executed.v1.schema.json` |
| Schema `$id` | `https://schemas.synthwerk.dev/events/<domain>/<entity>.<verb>.v<N>.json` | |
| Example file | `events/examples/<type>.json` | `events/examples/dev.synthwerk.llm.toolcall.executed.v1.json` |

- An event is a fact: something that happened. A question is a sync HTTP call, not an event.
- Use the past tense: `role_changed`, not `change_role`. Use `requested` for a request that is a fact.
- `actor` is `user:<sub>`, `service:<name>` or `system`. Never an email or a name.
- Data never holds an email, an IP, a prompt or message content.
- The domain `schemas.synthwerk.dev` is a placeholder (plan Q-C1). Tools resolve `$id` locally, never over the network.

## When to bump `v<N>`

- A change that `eventcompat` reports as breaking needs a new type `.v<N+1>`.
- The producer then publishes v<N> and v<N+1> until every consumer has moved (`events/catalog.yaml`).
- An optional field or a new enum value keeps the version. Consumers must ignore what they do not know.

## HTTP APIs (OpenAPI 3.1)

| Item | Rule | Example |
|---|---|---|
| Path | `/v<N>/<plural-noun>` in kebab-case, custom verbs as `:verb` | `/v1/entitlements:check` |
| Probe paths | `/healthz`, `/readyz`, no version, no token | |
| `operationId` | `<service>.<verb><Noun>` | `identity.listMembers` |
| Schema properties | `snake_case` | `latency_ms` |
| MCP tool | `<service>_<verb>_<noun>`, read-only | `pulse_get_health` |

- A breaking API change needs a new path `/v<N+1>/…`. `oasdiff` fails the PR otherwise.
