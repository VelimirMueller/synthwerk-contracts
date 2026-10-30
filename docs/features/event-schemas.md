---
title: Event schemas
slug: event-schemas
status: beta
epic: E1
owner: contracts
paths: ["events/**"]
api: []
events: ["dev.synthwerk.pulse.target.status_changed.v1", "dev.synthwerk.llm.toolcall.executed.v1", "dev.synthwerk.llm.toolcall.executed.v2", "dev.synthwerk.llm.message.completed.v1", "dev.synthwerk.llm.ai_settings.changed.v1", "dev.synthwerk.identity.org.created.v1", "dev.synthwerk.identity.user.created.v1", "dev.synthwerk.identity.member.role_changed.v1", "dev.synthwerk.identity.app.created.v1", "dev.synthwerk.identity.org.entitlements_changed.v1", "dev.synthwerk.identity.member.invited.v1", "dev.synthwerk.identity.member.invite_revoked.v1", "dev.synthwerk.identity.member.deactivated.v1", "dev.synthwerk.identity.member.reactivated.v1", "dev.synthwerk.identity.user.deleted.v1"]
mcp: []
updated: 2026-10-30
---

# Event schemas

> **In 30 seconds**
> - Every Synthwerk event is a CloudEvent 1.0.2. `events/envelope.schema.json` defines the envelope.
> - Each type has a JSON Schema 2020-12 for its `data`, one example and one catalog entry.
> - This repo defines events. It publishes none.

## What it does

- `events/envelope.schema.json`: the CloudEvents attributes, the Synthwerk rules and the extensions `orgid`, `traceparent`, `actor`. The `tenantid` extension is deprecated (D-44): producers must not set it, consumers must still accept it.
- `events/catalog.yaml`: per type the NATS subject, stream, producer, consumers, status, schema and example.
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
| Vocabulary | `org`, never `tenant`; `operator`, never `platform-admin` (D-44) |
| Unknown fields | rejected (`additionalProperties: false`) |
| Personal data | no email, IP, prompt or content in `data`. `actor` holds a subject id only. |
| Breaking change | new type `.v<N+1>`, see `docs/features/contract-checks.md` |

## Events

| Type | When |
|---|---|
| `dev.synthwerk.pulse.target.status_changed.v1` | Pulse sees a new readiness status of a target. |
| `dev.synthwerk.llm.toolcall.executed.v1` | Audit fact (D-26): the AI called one read-only MCP tool. Deprecated (D-44): the scope vocabulary moved to v2. |
| `dev.synthwerk.llm.toolcall.executed.v2` | Same audit fact. Scope is `org`, `user`, `operator` (D-44). Replaces deprecated v1. |
| `dev.synthwerk.llm.message.completed.v1` | An LLM answer ends: completed, failed or canceled. Metadata only. |
| `dev.synthwerk.identity.org.created.v1` | Identity created an org and its first owner. |
| `dev.synthwerk.identity.user.created.v1` | Zitadel created a human user (Action webhook). Zitadel ids only, no `orgid` until #29. |
| `dev.synthwerk.identity.member.role_changed.v1` | The org role of a member changed. `org_id`, `user_sub`, `old_role`, `new_role`; never `operator`. |
| `dev.synthwerk.identity.app.created.v1` | An org created an app. `app_id`, `org_id`, `name`; no origins. |
| `dev.synthwerk.identity.org.entitlements_changed.v1` | The plan or the limits of an org changed. Full state: `plan`, `limits`, `version`, `reason`. |
| `dev.synthwerk.identity.member.invited.v1` | An owner or admin invited an email. `org_id`, `invitation_id`, `role`; no email, no subject id. |
| `dev.synthwerk.identity.member.invite_revoked.v1` | An owner or admin revoked a pending invitation. `org_id`, `invitation_id`, `role`. |
| `dev.synthwerk.identity.member.deactivated.v1` | A member was deactivated. `org_id`, `user_sub`, `role`. |
| `dev.synthwerk.identity.member.reactivated.v1` | A member was reactivated. `org_id`, `user_sub`, `role`. |
| `dev.synthwerk.identity.user.deleted.v1` | A user deleted the own account. `user_sub` and `deleted_orgs`. Consumers delete the user's rows; a repeat is harmless. |

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
| 2026-10-23 | Add `llm.message.completed.v1` and `identity.org.created.v1` (stream `EVT_IDENTITY`). |
| 2026-10-23 | Add `identity.user.created.v1`: `user_sub` and `zitadel_org_id`, no email or name (synthwerk#28 task 4). |
| 2026-10-23 | D-44: `orgid` extension added, `tenantid` deprecated; `toolcall.executed.v2` replaces v1 (scope `org`, `user`, `operator`). |
| 2026-10-28 | Add `identity.member.role_changed.v1` (synthwerk#29). |
| 2026-10-29 | Add `identity.app.created.v1` (synthwerk#30). |
| 2026-10-29 | Add `identity.org.entitlements_changed.v1` (synthwerk#31). |
| 2026-10-30 | Add `identity.member.invited.v1`, `member.invite_revoked.v1`, `member.deactivated.v1`, `member.reactivated.v1` (synthwerk#37). |
| 2026-10-30 | Add `identity.user.deleted.v1` (synthwerk#41). |
| 2026-10-30 | Add `llm.ai_settings.changed.v1` (synthwerk#39). Consumer: identity, for the audit list. |
