---
title: Identity API
slug: identity-api
status: planned
epic: E2
owner: contracts
paths: ["openapi/identity/**", "codegen/oapi-codegen.identity.yaml", "events/identity/**"]
api: ["GET /v1/me", "GET /v1/orgs", "POST /v1/orgs", "GET /v1/orgs/{orgId}", "GET /v1/orgs/{orgId}/members", "PATCH /v1/orgs/{orgId}/members/{userId}", "GET /v1/orgs/{orgId}/apps", "POST /v1/orgs/{orgId}/apps", "GET /v1/orgs/{orgId}/apps/{appId}", "PATCH /v1/orgs/{orgId}/apps/{appId}", "DELETE /v1/orgs/{orgId}/apps/{appId}", "POST /v1/apps/{appId}/widget-tokens"]
events: ["dev.synthwerk.identity.org.created.v1", "dev.synthwerk.identity.user.created.v1", "dev.synthwerk.identity.member.role_changed.v1", "dev.synthwerk.identity.app.created.v1"]
mcp: []
updated: 2026-10-29
---

# Identity API

> **In 30 seconds**
> - `synthwerk-identity` tells who the caller is and which orgs the caller belongs to.
> - The word is `org` everywhere, never `tenant` (D-44). The platform role is `operator`.
> - The spec is `openapi/identity/openapi.yaml`. It holds stubs for M2 (#29, #30).
> - An org registers apps with an origin allow-list. A widget on a listed origin gets a widget token that lives 15 minutes or less (#30).

## What it does

- `GET /v1/me` returns the subject, the org and the roles of the token.
- `GET /v1/orgs` lists the caller's orgs. An operator sees every org.
- `POST /v1/orgs` creates an org. The caller becomes its `owner`.
- `GET /v1/orgs/{orgId}` returns one org.
- `GET /v1/orgs/{orgId}/members` lists the members and their org roles (caller: owner, admin or operator).
- `PATCH /v1/orgs/{orgId}/members/{userId}` changes the org role of a member (owner or admin; a member gets 403).
- A role change emits `dev.synthwerk.identity.member.role_changed.v1`.
- `GET|POST /v1/orgs/{orgId}/apps` and `GET|PATCH|DELETE /v1/orgs/{orgId}/apps/{appId}` manage the apps of an org. Read: any member or operator. Write: owner or admin.
- Creating an app emits `dev.synthwerk.identity.app.created.v1`. Update and delete emit no event yet.
- `POST /v1/apps/{appId}/widget-tokens` issues a widget token to a signed-in member of the app's org, only when the `Origin` header is in the allow-list.
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
| Stale token | Writes and `listMembers` check the stored membership too; a demoted admin gets 403 with the old token |
| App origin | `scheme://host[:port]`; `https`, or `http` for `localhost`, `127.0.0.1`, `[::1]`; no path, query, fragment, user info or wildcard |
| Origin match | Exact on the canonical form (lower-case host, default port removed). No prefix, suffix or subdomain match |
| Allow-list size | 1 to 20 origins per app, no duplicates |
| Widget token | JWT, `typ` `at+jwt`, `ES256`, signed by identity; `expires_in` 900 s or less; claim `synthwerk:origin` = the canonical origin; no roles |
| Widget token keys | Public keys at `/.well-known/jwks.json` of identity (outside the OpenAPI, no token) |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.identity.org.created.v1` | Identity created an org and its first owner. |
| `dev.synthwerk.identity.user.created.v1` | Zitadel created a human user; identity got the Action webhook. |
| `dev.synthwerk.identity.member.role_changed.v1` | The org role of a member changed. `old_role` and `new_role`, no email. |
| `dev.synthwerk.identity.app.created.v1` | An owner or admin created an app. `app_id`, `org_id`, `name`; no origins. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 400 | `invalid_request` | Body or parameter not valid. |
| 401 | `unauthenticated` | Token missing or not valid. |
| 403 | `forbidden` | Role does not allow the call. Generic body. |
| 403 | `origin_not_allowed` | The `Origin` header is missing or not in the allow-list of the app. |
| 404 | `not_found` | Unknown org or app, or the caller is not a member. Same body. |
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
- `tests/contract/identity_test.go` — `ListApps - spec example - one page of apps with origins`
- `tests/contract/identity_test.go` — `CreateApp - spec example - app with the canonical allow-list`
- `tests/contract/identity_test.go` — `UpdateApp - spec example - app with the new allow-list`
- `tests/contract/identity_test.go` — `CreateWidgetToken - spec example - bearer token, 15 minutes or less`
- `tests/contract/identity_test.go` — `CreateWidgetToken - origin not listed - 403 problem with code origin_not_allowed`
- `tests/contract/identity_client_test.go` — `Client <operation> - 404 problem - method and path match the spec` (every operation)
- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`

## Changes

| Date | Change |
|---|---|
| 2026-10-23 | First version: stubs for the caller and orgs, `org.created` event. |
| 2026-10-23 | `org.created` carries the org id in the `orgid` extension. `tenantid` is deprecated (D-44). |
| 2026-10-23 | Add `user.created` event (synthwerk#28 task 4). |
| 2026-10-28 | Add members list, role change and `member.role_changed.v1` (synthwerk#29). |
| 2026-10-29 | Add apps with an origin allow-list, the widget token and `app.created.v1` (synthwerk#30). |
