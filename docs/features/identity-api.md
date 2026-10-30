---
title: Identity API
slug: identity-api
status: planned
epic: E2
owner: contracts
paths: ["openapi/identity/**", "codegen/oapi-codegen.identity.yaml", "events/identity/**"]
api: ["GET /v1/me", "GET /v1/orgs", "POST /v1/orgs", "GET /v1/orgs/{orgId}", "GET /v1/orgs/{orgId}/members", "POST /v1/orgs/{orgId}/members", "PATCH /v1/orgs/{orgId}/members/{userId}", "POST /v1/orgs/{orgId}/members/{userId}:deactivate", "POST /v1/orgs/{orgId}/members/{userId}:reactivate", "GET /v1/orgs/{orgId}/invitations", "GET /v1/orgs/{orgId}/audit-events", "GET /v1/orgs/{orgId}/apps", "POST /v1/orgs/{orgId}/apps", "GET /v1/orgs/{orgId}/apps/{appId}", "PATCH /v1/orgs/{orgId}/apps/{appId}", "DELETE /v1/orgs/{orgId}/apps/{appId}", "POST /v1/apps/{appId}/widget-tokens", "GET /v1/apps/{appId}/widget-settings", "GET /v1/me/entitlements", "GET /v1/orgs/{orgId}/entitlements", "GET /v1/me/export", "DELETE /v1/me"]
events: ["dev.synthwerk.identity.org.created.v1", "dev.synthwerk.identity.user.created.v1", "dev.synthwerk.identity.member.role_changed.v1", "dev.synthwerk.identity.app.created.v1", "dev.synthwerk.identity.org.entitlements_changed.v1", "dev.synthwerk.identity.member.invited.v1", "dev.synthwerk.identity.member.invite_revoked.v1", "dev.synthwerk.identity.member.deactivated.v1", "dev.synthwerk.identity.member.reactivated.v1", "dev.synthwerk.identity.user.deleted.v1"]
mcp: []
updated: 2026-10-30
---

# Identity API

> **In 30 seconds**
> - `synthwerk-identity` tells who the caller is and which orgs the caller belongs to.
> - The word is `org` everywhere, never `tenant` (D-44). The platform role is `operator`.
> - The spec is `openapi/identity/openapi.yaml`. It holds stubs for M2 (#29, #30).
> - An org registers apps with an origin allow-list. A widget on a listed origin gets a widget token that lives 15 minutes or less (#30).
> - Each app has widget settings. The widget reads them without a token, only from a listed origin (#38).

## What it does

- `GET /v1/me` returns the subject, the org and the roles of the token.
- `GET /v1/orgs` lists the caller's orgs. An operator sees every org.
- `POST /v1/orgs` creates an org. The caller becomes its `owner`.
- `GET /v1/orgs/{orgId}` returns one org.
- `GET /v1/orgs/{orgId}/members` lists the members and their org roles (caller: owner, admin or operator).
- `PATCH /v1/orgs/{orgId}/members/{userId}` changes the org role of a member (owner or admin; a member gets 403).
- A role change emits `dev.synthwerk.identity.member.role_changed.v1`.
- `POST /v1/orgs/{orgId}/members` invites by email (owner or admin) and answers 202 with the pending invitation. For a new email, Zitadel creates the user and sends the invite mail.
- A user of another org gets the same answer, the same pending invitation and the same audit event, but is never added (not supported yet). The API does not show which emails have an account.
- `GET /v1/orgs/{orgId}/invitations` lists pending invitations with the typed email (owner or admin). The invitee moves to `listMembers` at the first sign-in to the org.
- A repeat invite of an email with a pending invitation answers 202 with that invitation, unchanged, for every email.
- `DELETE /v1/orgs/{orgId}/invitations/{invitationId}` revokes a pending invitation (owner or admin) and emits `dev.synthwerk.identity.member.invite_revoked.v1`. A Zitadel user identity made for it is deleted.
- `POST /v1/orgs/{orgId}/members/{userId}:deactivate` and `:reactivate` switch the Zitadel user grant off and on. The member keeps the row and the role.
- `GET /v1/orgs/{orgId}/audit-events` lists the org events identity wrote, newest first (owner or admin only, no operator). `data` is the event data.
- `GET|POST /v1/orgs/{orgId}/apps` and `GET|PATCH|DELETE /v1/orgs/{orgId}/apps/{appId}` manage the apps of an org. Read: any active member or operator. Write: owner or admin.
- Creating an app emits `dev.synthwerk.identity.app.created.v1`. Update and delete emit no event yet.
- A create past `apps_per_org` gets 403 `app_limit_reached` (#38).
- `updateApp` also sets the widget settings: position, theme, mode, language, accent group, greeting. New settings replace the old ones as a whole.
- `GET /v1/apps/{appId}/widget-settings` returns the widget settings without a token, only to an `Origin` in the allow-list. It is the one public route of the spec; the Spectral rule `synthwerk-security` lists it.
- `POST /v1/apps/{appId}/widget-tokens` issues a widget token to a signed-in member of the app's org, only when the `Origin` header is in the allow-list.
- `GET /v1/me/entitlements` returns the plan and the limits of the token's org. A service that acts for a user forwards the caller's token (llm quotas).
- `GET /v1/orgs/{orgId}/entitlements` returns the plan and the limits of one org (any org role with an active membership, or operator).
- A new org gets the plan `free`. Every change of a plan or its limits emits `dev.synthwerk.identity.org.entitlements_changed.v1` with the full new state (#31).
- `GET /v1/me/export` returns every row Synthwerk keeps about the caller: the identity rows, plus the export of each service that keeps user rows (today llm), asked with the caller's token (#41).
- `DELETE /v1/me` deletes the caller's account. It needs a sign-in with MFA in the last 15 minutes. It emits `dev.synthwerk.identity.user.deleted.v1`; the other services delete the user's rows when they get it (#41).
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
| Invite | owner or admin; only owner invites an `owner`; the email is an active or deactivated member: 409 `member_exists`; a pending invitation for the email: 202 with that invitation; per caller and per org rate limit: 429 `rate_limited` with `Retry-After` |
| Deactivate | owner or admin; only owner deactivates or reactivates an owner; never the own membership (409 `own_membership`); the last active owner stays (409 `last_owner`) |
| Member status | `active` or `deactivated`. A deactivated member has no org role in tokens and fails every stored-role check |
| Audit list | owner or admin, stored membership active; operator 403; newest first; clients accept unknown event types |
| Operator | Read only on any org; a role change gets 403 |
| Entitlement limits | `0` means no limit. A day is a UTC day. `version` goes up by one on every change |
| Plan name | `free` today. Clients read `limits`, never the plan name, and accept unknown plans |
| Stale token | Writes and `listMembers` check the stored membership too; a demoted admin gets 403 with the old token |
| App origin | `scheme://host[:port]`; `https`, or `http` for `localhost`, `127.0.0.1`, `[::1]`; no path, query, fragment, user info or wildcard |
| Origin match | Exact on the canonical form (lower-case host, default port removed). No prefix, suffix or subdomain match |
| Allow-list size | 1 to 20 origins per app, no duplicates |
| Widget token | JWT, `typ` `at+jwt`, `ES256`, signed by identity; `expires_in` 900 s or less; claim `synthwerk:origin` = the canonical origin; no roles |
| App limit | `apps_per_org` of the entitlements; `0` means no limit; one more gets 403 `app_limit_reached` |
| Widget settings | `position` `bottom-right`/`bottom-left`, `theme` `default`/`contrast`, `mode` `system`/`light`/`dark`, `lang` `auto`/`de`/`en`, `accent` `flagship`/`lab`/`ops`/`venture`, `greeting` 0 to 280 characters, plain text |
| Widget settings read | No token. `Origin` must be in the allow-list. `Cache-Control: no-cache`, refusals `no-store`. Settings only: no origins, org or name |
| CORS caching | Widget settings and widget token routes send `Vary: Origin` on every answer, preflight included. Token answers and refusals send `Cache-Control: no-store` |
| Export | One JSON document, `format_version` 1. `complete` is false when a service did not answer; `services[].data` is that service's own export schema (`LlmExport`) |
| Account delete | Token with `amr` `mfa` and a gated sign-in at most 15 minutes old, else 403 `recent_mfa_required`. Operators get 403 `forbidden` |
| Last owner on delete | Only owner of an org with other members: 409 `last_owner`, nothing changes. Only member of an org: the org closes with the account (`deleted_org_ids`) |
| After delete | Identity rows go in one transaction with the event. The Zitadel user is removed after the answer, with retries. Old tokens stay valid until they expire |
| Widget token keys | Public keys at `/.well-known/jwks.json` of identity (outside the OpenAPI, no token) |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.identity.org.created.v1` | Identity created an org and its first owner. |
| `dev.synthwerk.identity.user.created.v1` | Zitadel created a human user; identity got the Action webhook. |
| `dev.synthwerk.identity.member.role_changed.v1` | The org role of a member changed. `old_role` and `new_role`, no email. |
| `dev.synthwerk.identity.app.created.v1` | An owner or admin created an app. `app_id`, `org_id`, `name`; no origins. |
| `dev.synthwerk.identity.org.entitlements_changed.v1` | The plan or the limits of an org changed. Full new state, `version`, `reason`. |
| `dev.synthwerk.identity.member.invited.v1` | An owner or admin invited an email. `invitation_id` and `role`, no email, no subject id. Same for every email outside the org. |
| `dev.synthwerk.identity.member.invite_revoked.v1` | An owner or admin revoked a pending invitation. `invitation_id` and `role`, no email. |
| `dev.synthwerk.identity.member.deactivated.v1` | An owner or admin deactivated a member. `user_sub` and the kept `role`. |
| `dev.synthwerk.identity.member.reactivated.v1` | An owner or admin reactivated a member. `user_sub` and `role`. |
| `dev.synthwerk.identity.user.deleted.v1` | A user deleted the own account. `user_sub` and the orgs that closed with it. Consumers delete or anonymise the user's rows. |

## Errors

| Status | Problem type | Cause |
|---|---|---|
| 400 | `invalid_request` | Body or parameter not valid. |
| 401 | `unauthenticated` | Token missing or not valid. |
| 403 | `forbidden` | Role does not allow the call. Generic body. |
| 403 | `app_limit_reached` | The org has `apps_per_org` apps already. |
| 403 | `recent_mfa_required` | Account delete without a sign-in with MFA in the last 15 minutes. |
| 403 | `origin_not_allowed` | The `Origin` header is missing or not in the allow-list of the app. |
| 404 | `not_found` | Unknown org or app, or the caller is not a member. Same body. |
| 409 | `slug_taken` | Another org has the slug. |
| 409 | `last_owner` | The org would lose its last owner (role change or account delete). |
| 409 | `own_role` | The caller tried to change their own role. |
| 409 | `own_membership` | The caller tried to deactivate their own membership. |
| 409 | `member_exists` | The invited email is an active or deactivated member of this org. |
| 429 | `rate_limited` | Too many invites from the caller or for the org. |
| 502 | `upstream_unavailable` | Zitadel did not take the change. Nothing changed. |

## Tests

- `tests/contract/identity_test.go` — `GetMe - spec example - subject, org and roles`
- `tests/contract/identity_test.go` — `GetOrg - not a member - 404 problem with code not_found`
- `tests/contract/identity_test.go` — `ListMembers - spec example - one page of members with org roles`
- `tests/contract/identity_test.go` — `ChangeMemberRole - spec example - member with the new role`
- `tests/contract/identity_test.go` — `ChangeMemberRole - last owner - 409 problem with code last_owner`
- `tests/contract/identity_test.go` — `CreateApp - org at its app limit - 403 problem with code app_limit_reached`
- `tests/contract/identity_test.go` — `UpdateApp - widget settings - the client sends them, the answer carries them`
- `tests/contract/identity_test.go` — `GetWidgetSettings - spec example - settings only, Origin sent, no token`
- `tests/contract/identity_test.go` — `WidgetSettings - enums - only token themes and accent groups`
- `tests/contract/identity_cors_test.go` — `CORS answer <route> - Vary and Cache-Control documented`
- `tests/contract/identity_test.go` — `OrgRole - enum - operator is never an org role`
- `tests/contract/identity_test.go` — `InviteMember - spec example - 202 with the pending invitation`
- `tests/contract/identity_test.go` — `ListInvitations - spec example - pending invitations`
- `tests/contract/identity_test.go` — `InviteMember - too many invites - 429 problem with code rate_limited`
- `tests/contract/identity_test.go` — `InviteMember - email is a user already - 409 problem with code member_exists`
- `tests/contract/identity_test.go` — `DeactivateMember - spec example - member with the new status` (and `ReactivateMember`)
- `tests/contract/identity_test.go` — `ListAuditEvents - spec example - newest first, event data, next cursor`
- `tests/contract/identity_test.go` — `MemberStatus - enum - active and deactivated only`
- `tests/contract/identity_test.go` — `ListApps - spec example - one page of apps with origins`
- `tests/contract/identity_test.go` — `CreateApp - spec example - app with the canonical allow-list`
- `tests/contract/identity_test.go` — `UpdateApp - spec example - app with the new allow-list`
- `tests/contract/identity_test.go` — `CreateWidgetToken - spec example - bearer token, 15 minutes or less`
- `tests/contract/identity_test.go` — `CreateWidgetToken - origin not listed - 403 problem with code origin_not_allowed`
- `tests/contract/identity_client_test.go` — `Client <operation> - 404 problem - method and path match the spec` (every operation)
- `tests/contract/identity_entitlements_test.go` — `GetMyEntitlements - spec example - free plan with its limits` (and `GetOrgEntitlements`)
- `tests/contract/identity_entitlements_test.go` — `GetOrgEntitlements - unknown org - 404 problem, method and path match the spec` (and `GetMyEntitlements`)
- `tests/contract/identity_account_test.go` — `ExportMe - spec example - identity rows and the llm part`; `ExportMe - no token - 401 unauthenticated`
- `tests/contract/identity_account_test.go` — `DeleteMe - spec example - accepted with the closed org`
- `tests/contract/identity_account_test.go` — `DeleteMe - no recent MFA - 403 recent_mfa_required`; `last owner of an org with members - 409 last_owner`; `no token - 401 unauthenticated`
- `tests/contract/events_test.go` — `Catalog - each entry - names, schema $id and example match`

## Changes

| Date | Change |
|---|---|
| 2026-10-23 | First version: stubs for the caller and orgs, `org.created` event. |
| 2026-10-23 | `org.created` carries the org id in the `orgid` extension. `tenantid` is deprecated (D-44). |
| 2026-10-23 | Add `user.created` event (synthwerk#28 task 4). |
| 2026-10-28 | Add members list, role change and `member.role_changed.v1` (synthwerk#29). |
| 2026-10-29 | Add apps with an origin allow-list, the widget token and `app.created.v1` (synthwerk#30). |
| 2026-10-29 | Add entitlements: `GET /v1/me/entitlements`, `GET /v1/orgs/{orgId}/entitlements` and `org.entitlements_changed.v1` (synthwerk#31). Spec 0.2.0. |
| 2026-10-30 | Add widget settings (`updateApp`, `GET /v1/apps/{appId}/widget-settings`) and 403 `app_limit_reached` on `createApp` (synthwerk#38). Spec 0.3.0. |
| 2026-10-30 | Add invite, deactivate and reactivate members, the member `status`, the invite rate limit, pending invitations and their revoke, the audit list and `member.invited.v1`, `member.invite_revoked.v1`, `member.deactivated.v1` and `member.reactivated.v1` (synthwerk#37). Spec 0.4.0. |
| 2026-10-30 | Add `GET /v1/me/export`, `DELETE /v1/me`, problem code `recent_mfa_required` and `user.deleted.v1` (synthwerk#41). Spec 0.5.0. |
| 2026-10-30 | Org routes, entitlements and the widget token need an active stored membership; a deactivated or invited member gets 404 (synthwerk#37 security review). Wording only. Spec 0.5.1. |
