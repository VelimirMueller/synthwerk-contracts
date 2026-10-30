---
title: Vision label-set API
slug: vision-label-sets-api
status: beta
epic: E5
owner: contracts
paths: ["openapi/vision/**", "codegen/oapi-codegen.vision.yaml"]
api: ["GET /v1/orgs/{orgId}/labels", "POST /v1/orgs/{orgId}/labels", "DELETE /v1/orgs/{orgId}/labels/{labelId}", "POST /v1/orgs/{orgId}/feedback"]
events: []
mcp: []
updated: 2026-10-30
---

# Vision label-set API

> **In 30 seconds**
> - `synthwerk-vision` keeps one label set and one list of corrections per org (synthwerk#44).
> - The spec is `openapi/vision/openapi.yaml`. CI generates a Go client and a TS client from it.
> - The caller is a service with the vision admin token, today the studio BFF. Vision trusts the org id in the path.

## What it does

- `GET /v1/orgs/{orgId}/labels` returns the label set of an org: `custom`, the added `labels`, every `topics` entry and `total_labels`.
- `POST /v1/orgs/{orgId}/labels` adds a text label. `DELETE /v1/orgs/{orgId}/labels/{labelId}` removes an added label.
- `POST /v1/orgs/{orgId}/feedback` stores the correct label of a picture for one org.
- An org without its own set uses the default set. The first change copies the default set into the org.
- A classification uses the set and the corrections of the widget token's org.
- `orgId` is the Zitadel org id (`urn:zitadel:iam:user:resourceowner:id`), not the identity org UUID.
- Go: package `github.com/VelimirMueller/synthwerk-contracts/go/vision`. TS: `createVisionClient` and `LabelSet` in `@synthwerk/contracts`.

## Who may call

- The security scheme `service` is the vision admin token. It is a service credential. It never reaches a browser.
- The studio BFF calls these routes for a signed-in user only when the user is an active `owner` or `admin` of the org. It checks the token roles and the stored membership in identity first.
- Widget tokens and user tokens get 401 on these routes.

## How to use

1. Call the routes from Go with `vision.NewClientWithResponses(baseURL, vision.WithRequestEditorFn(...))`. The editor sets `Authorization: Bearer <admin token>`.
2. Call them from TS with `createVisionClient({ baseUrl, headers: { authorization: ... } })`. Server side only.
3. Branch on the status. The body is FastAPI `{detail}`, text for people.

## Rules and limits

| Rule | Value |
|---|---|
| `orgId` | `^[A-Za-z0-9_-]{1,64}$` |
| Label name, topic | 1 to 80 characters |
| Prompt | 200 characters or less |
| Label id | `runtime:<slug>`. Packaged labels cannot be removed. |
| Add time | up to about 30 s when the text tower must embed a new text |
| Errors | FastAPI `{detail}`, not RFC 9457 problems yet |

## Events

| Type | When |
|---|---|
| None yet. A `dev.synthwerk.vision.label_set.changed.v1` fact is open (synthwerk#44). | |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 401 | None (`{detail}`) | No or wrong admin token. |
| 404 | None (`{detail}`) | Vision has no admin token, or the label is not in the org set. |
| 409 | None (`{detail}`) | Label clash, unknown topic, packaged label, fixed-label model, or a correction for a label outside the set. |
| 413 | None (`{detail}`) | Image above the upload limit. |
| 415 | None (`{detail}`) | Not a JPEG, PNG or WebP image. |
| 422 | None (`{detail}` list) | `orgId` or body not valid. |
| 503 | None (`{detail}`) | Model not loaded, or no feedback store. |

## Tests

- `tests/contract/vision_test.go` — `LabelSet - spec example - decodes strictly`
- `tests/contract/vision_test.go` — `Label and Feedback - spec examples - decode strictly`
- `tests/contract/vision_test.go` — `Error - every response example - decodes to a string or a list detail`
- `tests/contract/vision_test.go` — `GetLabelSet - client - sends the service bearer to the org path`
- `tests/contract/vision_test.go` — `CreateLabel - 409 - typed error body`
- `tests/contract/vision_test.go` — `DeleteLabel - 204 - no body`

## Open points

- The classification and upload routes are in the same spec: see [Vision classification and upload API](vision-widget-api.md).
- Problem details (RFC 9457) for vision errors.
- The `label_set.changed` event.

## Changes

| Date | Change |
|---|---|
| 2026-10-30 | First version: label set, add, remove, corrections per org (synthwerk#44). Spec version 0.1.0. |
