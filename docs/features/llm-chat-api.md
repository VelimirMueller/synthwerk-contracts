---
title: LLM chat API
slug: llm-chat-api
status: planned
epic: E3
owner: contracts
paths: ["openapi/llm/**", "codegen/oapi-codegen.llm.yaml", "events/llm/message.completed.v1.schema.json"]
api: ["POST /v1/conversations", "GET /v1/conversations/{id}", "POST /v1/conversations/{id}/messages"]
events: ["dev.synthwerk.llm.message.completed.v1"]
mcp: []
updated: 2026-10-23
---

# LLM chat API

> **In 30 seconds**
> - `synthwerk-llm` serves conversations. A sent message comes back as a Server-Sent Events stream.
> - The stream has four events: `delta`, `usage`, `done`, `error` (D-44).
> - The spec is `openapi/llm/openapi.yaml`. CI generates a Go client and a TS client from it.

## What it does

- `POST /v1/conversations` creates an empty conversation for the caller.
- `GET /v1/conversations/{id}` returns the conversation with its messages, oldest first.
- `POST /v1/conversations/{id}/messages` stores the user message and streams the answer.
- The org and the owner come from the verified token. They are never body fields.
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
| Heartbeat | every 15 s |
| Text format | plain text only, never HTML |
| Token | Bearer JWT on every route |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.llm.message.completed.v1` | An answer ends: `completed`, `failed` or `canceled`. No prompt, no answer text. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 400 | `invalid_request` | Body or parameter not valid. |
| 401 | `unauthenticated` | Token missing or not valid. `WWW-Authenticate: Bearer`. |
| 403 | `forbidden` | Scope or origin does not allow the call. |
| 404 | `not_found` | Unknown id or another owner's id. Same body. |
| 413 | `input_too_large` | Message above the input limit. |
| 429 | `too_many_streams` | Owner has the max number of open streams. |
| 429 | `quota_exhausted` | Reserved (D-46). M2 never sends it. |
| 503 | `model_unavailable` | Provider down or busy. Also an `error` event mid-stream. |

## Tests

- `tests/contract/llm_test.go` — `StreamEvent - each spec example - decodes to its event type`
- `tests/contract/llm_test.go` — `SendMessage - no token - 401 problem with code unauthenticated`
- `tests/contract/llm_test.go` — `Problem - quota_exhausted - reserved in the 429 response`
- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`

## Changes

| Date | Change |
|---|---|
| 2026-10-23 | First version: conversations, stream events, problem codes, `message.completed` event. |
| 2026-10-23 | `message.completed` carries the org id in the `orgid` extension. `tenantid` is deprecated (D-44). |
