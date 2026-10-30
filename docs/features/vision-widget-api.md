---
title: Vision classification and upload API
slug: vision-widget-api
status: beta
epic: E5
owner: contracts
paths: ["openapi/vision/**", "codegen/oapi-codegen.vision.yaml"]
api: ["POST /v1/classifications", "POST /v1/images"]
events: []
mcp: []
updated: 2026-10-30
---

# Vision classification and upload API

> **In 30 seconds**
> - The vision widget classifies and stores pictures through these two routes (synthwerk#76).
> - The spec is `openapi/vision/openapi.yaml`, the same spec as the [label-set admin routes](vision-label-sets-api.md).
> - The caller is a browser with a widget token. The token is bound to one origin.

## What it does

- `POST /v1/classifications` classifies an image. Vision does not store it. The answer is `Classification`: `primary`, `topic`, the top-k `labels`, the `topics`, `uncertain`, `correction` (or null) and `timings_ms`.
- `POST /v1/images` stores an image for the token's org. The answer is `Image` with the generated `id`.
- Both take `multipart/form-data` with one field, `image`.
- The label set and the corrections are those of the token's org.
- Go: `vision.NewClientWithResponses`, then `VisionCreateClassificationWithBodyWithResponse` and `VisionCreateImageWithBodyWithResponse`. TS: `createVisionClient`, `Classification` and `VisionImage` in `@synthwerk/contracts`.

## Who may call

- The security scheme `widget`: a widget token from identity (`POST /v1/apps/{appId}/widget-tokens`). ES256, at most 15 min.
- The browser `Origin` header must equal the token claim `synthwerk:origin`. Else 403.
- The vision admin token does not work on these routes.

## How to use

1. Get a widget token from identity for the app and the page origin.
2. Send `Authorization: Bearer <widget token>` and a form with the field `image`. The browser sets `Origin`.
3. Branch on the status. The body is FastAPI `{detail}`, text for people.

## Rules and limits

| Rule | Value |
|---|---|
| Image types | JPEG, PNG, WebP |
| Upload size | `SYNTHWERK_VISION_MAX_UPLOAD_BYTES`, default 5 MiB |
| Upload quota | bytes and files per org, plus total bytes for all orgs. Full: 507. |
| Request id | `X-Request-ID` on every answer. Classification repeats it as `request_id`. |
| Errors | FastAPI `{detail}`, not RFC 9457 problems yet |

## Events

| Type | When |
|---|---|
| None. | |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 401 | None (`{detail}`) | No token, a token that is not valid, or not a widget token. |
| 403 | None (`{detail}`) | `Origin` missing, different from the token origin, or sent twice. |
| 413 | None (`{detail}`) | Image above the upload limit. |
| 415 | None (`{detail}`) | Not a JPEG, PNG or WebP image. |
| 422 | None (`{detail}` list) | No `image` field. |
| 503 | None (`{detail}`) | Model not loaded, token keys not available, or upload storage down. |
| 507 | None (`{detail}`) | Upload only: the quota of the org or the total quota is full. |

## Tests

- `tests/contract/vision_widget_test.go` — `Classification - spec example - decodes strictly, no correction`
- `tests/contract/vision_widget_test.go` — `Classification - with a correction - decodes the correction strictly`
- `tests/contract/vision_widget_test.go` — `Image - spec example - decodes strictly`
- `tests/contract/vision_widget_test.go` — `CreateClassification - client - posts the image field with the widget bearer`
- `tests/contract/vision_widget_test.go` — `CreateImage - client - posts the image field and reads the stored image`
- `tests/contract/vision_widget_test.go` — `CreateClassification - each error status - typed error body`
- `tests/contract/vision_widget_test.go` — `CreateImage - each error status - typed error body`

## Open points

- Problem details (RFC 9457) for vision errors.
- `GET /v1/images/{id}` and a retention rule for old uploads.

## Changes

| Date | Change |
|---|---|
| 2026-10-30 | First version: classification and upload with a widget token (synthwerk#76). Spec version 0.2.0. |
