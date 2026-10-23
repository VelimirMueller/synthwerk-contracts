---
title: Identity API
slug: identity-api
status: planned
epic: E2
owner: contracts
paths: ["openapi/identity/**", "codegen/oapi-codegen.identity.yaml", "events/identity/**"]
api: ["GET /v1/me", "GET /v1/orgs", "POST /v1/orgs", "GET /v1/orgs/{orgId}"]
events: ["dev.synthwerk.identity.org.created.v1"]
mcp: []
updated: 2026-10-23
---

# Identity API

> **In 30 seconds**
> - `synthwerk-identity` tells who the caller is and which orgs the caller belongs to.
> - The word is `org` everywhere, never `tenant` (D-44). The platform role is `operator`.
> - The spec is `openapi/identity/openapi.yaml`. It holds stubs for M2 (#29).

## What it does

- `GET /v1/me` returns the subject, the org and the roles of the token.
- `GET /v1/orgs` lists the caller's orgs. An operator sees every org.
- `POST /v1/orgs` creates an org. The caller becomes its `owner`.
- `GET /v1/orgs/{orgId}` returns one org.
- Creating an org emits `dev.synthwerk.identity.org.created.v1`.
- Go: package `github.com/VelimirMueller/synthwerk-contracts/go/identity`. TS: `createIdentityClient` in `@synthwerk/contracts`.

## How to use

1. Call the routes from Go with `identity.NewClientWithResponses(baseURL)`.
2. Call them from TS with `createIdentityClient({ baseUrl })`.
3. Branch on the problem `code`, never on `title`. Accept unknown codes.

## Rules and limits

| Rule | Value |
|---|---|
| Org roles | `owner`, `admin`, `member` |
| Platform role | `operator`, never an org membership |
| Page size | 1 to 100, default 50, cursor paging |
| Slug | `^[a-z][a-z0-9-]*$`, 2 to 64 characters, unique |
| Token | Bearer JWT on every route |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.identity.org.created.v1` | Identity created an org and its first owner. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 400 | `invalid_request` | Body or parameter not valid. |
| 401 | `unauthenticated` | Token missing or not valid. |
| 403 | `forbidden` | Role does not allow the call. Generic body. |
| 404 | `not_found` | Unknown org, or the caller is not a member. Same body. |
| 409 | `slug_taken` | Another org has the slug. |

## Tests

- `tests/contract/identity_test.go` — `GetMe - spec example - subject, org and roles`
- `tests/contract/identity_test.go` — `GetOrg - not a member - 404 problem with code not_found`
- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`

## Changes

| Date | Change |
|---|---|
| 2026-10-23 | First version: stubs for the caller and orgs, `org.created` event. |
| 2026-10-23 | `org.created` carries the org id in the `orgid` extension. `tenantid` is deprecated (D-44). |
