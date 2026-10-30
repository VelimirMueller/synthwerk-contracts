---
title: LLM chat API
slug: llm-chat-api
status: planned
epic: E3
owner: contracts
paths: ["openapi/llm/**", "codegen/oapi-codegen.llm.yaml", "events/llm/message.completed.v1.schema.json", "events/llm/ai_settings.changed.v1.schema.json"]
api: ["POST /v1/conversations", "GET /v1/conversations/{id}", "POST /v1/conversations/{id}/messages", "GET /v1/me/export", "GET /v1/ai-settings", "PUT /v1/ai-settings"]
events: ["dev.synthwerk.llm.message.completed.v1", "dev.synthwerk.llm.ai_settings.changed.v1"]
mcp: []
updated: 2026-10-30
---

# LLM chat API

> **In 30 seconds**
> - `synthwerk-llm` serves conversations. A sent message comes back as a Server-Sent Events stream.
> - The stream has four events: `delta`, `usage`, `done`, `error` (D-44).
> - An owner or admin sets the AI settings of the org with `/v1/ai-settings` (synthwerk#39). They stay inside the service aliases and the plan.
> - llm answers CORS for create, read and send: the chat widget runs on customer sites (infra#21).
> - The spec is `openapi/llm/openapi.yaml`. CI generates a Go client and a TS client from it.

## What it does

- `POST /v1/conversations` creates an empty conversation for the caller.
- `GET /v1/conversations/{id}` returns the conversation with its messages, oldest first.
- `POST /v1/conversations/{id}/messages` stores the user message and streams the answer.
- `GET /v1/me/export` returns every conversation, message and own quota row of the caller, in every org (`LlmExport`, synthwerk#41). Identity embeds it in its own export.
- `GET /v1/ai-settings` and `PUT /v1/ai-settings` read and replace the AI settings of the token's org (synthwerk#39). See [AI settings](#ai-settings).
- The org and the owner come from the verified token. They are never body fields.
- Create, read and send answer CORS in llm (infra#21). The preflight (`OPTIONS`) needs no token. Every answer sends `Vary: Origin`.
- An allowed origin is in llm's static list or is listed by an app of an active org in identity. Only it gets `Access-Control-Allow-Origin` and `Access-Control-Expose-Headers: Retry-After`.
- After each answer the service emits `dev.synthwerk.llm.message.completed.v1`. It holds metadata only.
- Go: package `github.com/VelimirMueller/synthwerk-contracts/go/llm`. TS: `createLlmClient` and `StreamEvent` in `@synthwerk/contracts`.

## Stream protocol

The send route answers `200` with `Content-Type: text/event-stream`. Each frame has three fields:

```text
id: <message_id>:<sequence>
event: <delta | usage | done | error>
data: <one line of JSON>
```

| Event | `data` | When |
|---|---|---|
| `delta` | `{text}` | One per provider chunk. Append `text` to the answer. |
| `usage` | `{input_tokens, output_tokens, model}` | Once, after the last `delta`. `model` is the alias. |
| `done` | `{message_id, finish_reason}` | Last event of a stream that did not fail. `finish_reason` is `stop` or `length`. |
| `error` | RFC 9457 problem | The model failed after the first byte. The stream ends after it. |

- Order: zero or more `delta`, then `usage`, then `done`. Or `error` at any point, then the end.
- A refusal before the first byte is a normal HTTP status with `application/problem+json`.
- After the first byte the status stays `200`. A failure is an `error` event.
- Cancel: the client closes the connection. The server stops the model call and keeps the partial answer.
- Heartbeat: a comment line `: ping` every 15 s. Clients ignore comment lines.
- Headers: `Cache-Control: no-store`, `X-Accel-Buffering: no`.
- M2 does not replay. `Last-Event-ID` is ignored, but clients can store the `id`.
- OpenAPI 3.1 cannot type a stream. The spec types one frame as `StreamEvent`, a `oneOf` on `event`.

## AI settings

One settings record per org. llm applies it to every create and send of the org.

| Setting | Values | Default (`version` 0) |
|---|---|---|
| `model` | An alias of `allowed_models`. The model of a new conversation without `model`. | `default` |
| `allowed_models` | 1 to 32 aliases, a subset of `bounds.models` (the operator's `MODEL_ALIASES`). Holds `model`. | every service alias |
| `system_prompt` | Plain text, 4000 characters or less. Line breaks and tabs allowed, other control characters refused. Empty = the service prompt. | empty |
| `max_input_chars` | 1 to `bounds.max_input_chars` (service limit, at most 8000). A longer message gets 413. | service limit |
| `user_tokens_per_day` | 0 to `bounds.user_tokens_per_day` (the plan's per-user limit, identity entitlements). The smaller of cap and plan applies. 0 = plan only. | 0 |
| `temperature` | 0 to 2. Absent = the model default. | absent |

- Access: token role `owner` or `admin`, then an active stored membership. llm checks it with identity `GET /v1/me/entitlements` and the caller's token; the same call gives the plan limit for `bounds`.
- `member`, `operator` and widget tokens get 403. Identity down or not configured: 503 `settings_unavailable`.
- `PUT` replaces every setting. `version` must be the one the caller read, else 409 `version_conflict`.
- A change emits `dev.synthwerk.llm.ai_settings.changed.v1` with the full new state and the list of changed settings. No system prompt text, only its length. Identity shows it in the audit list.
- An org can never name a provider model: only aliases, and only those of the service.

## How to use

1. Call the routes from Go with `llm.NewClientWithResponses(baseURL)`. For the send route, use `llm.NewClient(baseURL)` and `LlmSendMessage`: it returns the raw `*http.Response`. `LlmSendMessageWithResponse` reads the whole stream before it returns.
2. Call them from TS with `createLlmClient({ baseUrl })`. Read the send route with `parseAs: "stream"`.
3. Parse each frame into `StreamEvent`. Switch on `event`.
4. Branch on the problem `code`, never on `title`. Accept unknown codes.

## Rules and limits

| Rule | Value |
|---|---|
| Message input | max 8000 characters (413 above) |
| Open streams per owner | default 2 (429 above) |
| Token quota per user and per org | per UTC day, set by the service (429 `quota_exhausted` above) |
| Heartbeat | every 15 s |
| Text format | plain text only, never HTML |
| Token | Bearer JWT on every route |

## Quota error

A send over quota answers `429` before any model call (synthwerk#33):

```json
{
  "type": "https://schemas.synthwerk.dev/problems/quota_exhausted",
  "title": "Quota exhausted",
  "status": 429,
  "code": "quota_exhausted",
  "quota": { "scope": "user", "unit": "tokens", "limit": 50000, "resets_at": "2026-10-24T00:00:00Z" }
}
```

- `quota.scope`: `user` (the caller's own quota in this org) or `org` (the shared quota of the org).
- `quota.limit`: tokens per window for that scope. `quota.resets_at`: start of the next window.
- `Retry-After`: seconds until `resets_at`.
- `quota` holds no usage of other users. It is optional: other problems never carry it.

## Events

| Type | When |
|---|---|
| `dev.synthwerk.llm.message.completed.v1` | An answer ends: `completed`, `failed` or `canceled`. No prompt, no answer text. |
| `dev.synthwerk.llm.ai_settings.changed.v1` | An owner or admin changed the AI settings. Full new state, the changed setting names, the prompt length only. `orgid` = identity org id. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 400 | `invalid_request` | Body or parameter not valid. |
| 401 | `unauthenticated` | Token missing or not valid. `WWW-Authenticate: Bearer`. |
| 403 | `forbidden` | Scope or origin does not allow the call. |
| 404 | `not_found` | Unknown id or another owner's id. Same body. |
| 409 | `version_conflict` | The AI settings changed since the caller read them. Nothing changed. |
| 413 | `input_too_large` | Message above the input limit. |
| 429 | `too_many_streams` | Owner has the max number of open streams. |
| 429 | `quota_exhausted` | User or org quota of the window used up. No model call. Body member `quota`; `Retry-After` = seconds to the reset. |
| 503 | `model_unavailable` | Provider down or busy. Also an `error` event mid-stream. |
| 503 | `settings_unavailable` | llm cannot check the membership and the plan for the AI settings now. |

## Tests

- `tests/contract/llm_test.go` — `StreamEvent - each spec example - decodes to its event type`
- `tests/contract/llm_test.go` — `SendMessage - no token - 401 problem with code unauthenticated`
- `tests/contract/llm_test.go` — `Problem - each 429 example - decodes strictly to a Problem with its code`
- `tests/contract/llm_test.go` — `Problem - quota_exhausted - names scope, unit, limit and reset`
- `tests/contract/llm_test.go` — `Problem - quota member - only in the problem schema, optional`
- `tests/contract/identity_account_test.go` — `LLM ExportMe - spec example - conversations, messages with tokens, own quota rows`; `LLM ExportMe - widget token - 403 forbidden`
- `tests/contract/llm_settings_test.go` — `AiSettings - each 200 example - decodes strictly and stays inside its bounds`; `UpdateAiSettings - spec request example - client sends PUT /v1/ai-settings with the body`; `UpdateAiSettings - stale version - 409 version_conflict`; `GetAiSettings - identity down - 503 settings_unavailable`; `GetAiSettings - member token - 403 forbidden`
- `tests/contract/llm_cors_test.go` — `CORS answer createConversation 201 - Vary documented, preflight described` (also getConversation, sendMessage)
- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`; `AuditEvent.type - every identity event and every event identity consumes - matches the spec pattern`

## Changes

| Date | Change |
|---|---|
| 2026-10-23 | First version: conversations, stream events, problem codes, `message.completed` event. |
| 2026-10-23 | `message.completed` carries the org id in the `orgid` extension. `tenantid` is deprecated (D-44). |
| 2026-10-29 | `quota_exhausted` is sent (synthwerk#33). New optional problem member `quota` (`QuotaExceeded`). Spec version 0.2.0. |
| 2026-10-30 | Add `GET /v1/me/export` and `LlmExport` (synthwerk#41). llm consumes `dev.synthwerk.identity.user.deleted.v1`. Spec version 0.3.0. |
| 2026-10-30 | Add `GET`/`PUT /v1/ai-settings`, `AiSettings`, problem codes `version_conflict` and `settings_unavailable`, and `llm.ai_settings.changed.v1` (synthwerk#39). Spec version 0.4.0. |
| 2026-10-30 | Describe the CORS answers of create, read and send; `Vary` on their success answers (infra#21). Spec version 0.5.0. |
