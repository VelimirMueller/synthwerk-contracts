---
title: Identity API
slug: identity-api
status: planned
epic: E2
owner: contracts
paths: ["openapi/identity/**", "codegen/oapi-codegen.identity.yaml", "events/identity/**"]
api: ["GET /v1/me", "GET /v1/orgs", "POST /v1/orgs", "GET /v1/orgs/{orgId}", "GET /v1/orgs/{orgId}/members", "PATCH /v1/orgs/{orgId}/members/{userId}"]
events: ["dev.synthwerk.identity.org.created.v1", "dev.synthwerk.identity.user.created.v1", "dev.synthwerk.identity.member.role_changed.v1"]
mcp: []
updated: 2026-10-28
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
- `GET /v1/orgs/{orgId}/members` lists the members and their org roles (caller: owner, admin or operator).
- `PATCH /v1/orgs/{orgId}/members/{userId}` changes the org role of a member (owner or admin; a member gets 403).
- A role change emits `dev.synthwerk.identity.member.role_changed.v1`.
- Creating an org emits `dev.synthwerk.identity.org.created.v1`.
- A new Zitadel user (Action webhook) emits `dev.synthwerk.identity.user.created.v1`, with Zitadel ids only.
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
| Role change | owner and admin set `admin` or `member`; only owner grants `owner` or changes an owner |
| Last owner | Keeps the role: 409 `last_owner` |
| Own role | Never changed by the caller: 409 `own_role` |
| Operator | Read only on any org; a role change gets 403 |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.identity.org.created.v1` | Identity created an org and its first owner. |
| `dev.synthwerk.identity.user.created.v1` | Zitadel created a human user; identity got the Action webhook. |
| `dev.synthwerk.identity.member.role_changed.v1` | The org role of a member changed. `old_role` and `new_role`, no email. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 400 | `invalid_request` | Body or parameter not valid. |
| 401 | `unauthenticated` | Token missing or not valid. |
| 403 | `forbidden` | Role does not allow the call. Generic body. |
| 404 | `not_found` | Unknown org, or the caller is not a member. Same body. |
| 409 | `slug_taken` | Another org has the slug. |
| 409 | `last_owner` | The org would lose its last owner. |
| 409 | `own_role` | The caller tried to change their own role. |
| 502 | `upstream_unavailable` | Zitadel did not take the role change. Nothing changed. |

## Tests

- `tests/contract/identity_test.go` — `GetMe - spec example - subject, org and roles`
- `tests/contract/identity_test.go` — `GetOrg - not a member - 404 problem with code not_found`
- `tests/contract/identity_test.go` — `ListMembers - spec example - one page of members with org roles`
- `tests/contract/identity_test.go` — `ChangeMemberRole - spec example - member with the new role`
- `tests/contract/identity_test.go` — `ChangeMemberRole - last owner - 409 problem with code last_owner`
- `tests/contract/identity_test.go` — `OrgRole - enum - operator is never an org role`
- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`

## Changes

| Date | Change |
|---|---|
| 2026-10-23 | First version: stubs for the caller and orgs, `org.created` event. |
| 2026-10-23 | `org.created` carries the org id in the `orgid` extension. `tenantid` is deprecated (D-44). |
| 2026-10-23 | Add `user.created` event (synthwerk#28 task 4). |
| 2026-10-28 | Add members list, role change and `member.role_changed.v1` (synthwerk#29). |
