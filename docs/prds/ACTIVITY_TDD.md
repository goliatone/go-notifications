# go-admin Activity Tracking – Technical Design Notes

This document is meant to be shared with teams working on sibling modules (go-cms, go-notifications, go-options, go-dashboard, go-crud, go-job, go-auth, go-users, etc.). Please read it as a standalone spec; it should not assume file paths or context from any specific repository. Where we say “go-admin repo,” that refers to `github.com/goliatone/go-admin`; all other work belongs in the named upstream module.

## Goal
Define a cross-package activity tracking model (aligned with go-users activity interfaces) and the changes needed across goliatone packages to produce/consume activity events with configurable persistence and metadata.

## Scope
- Standardize on go-users activity interfaces (actor, action, object, context, metadata, timestamp).
- Allow pluggable persistence (DB-backed stores for production, in-memory for tests).
- Propagate activity emission through admin slices and dependent packages (CMS, notifications, options, dashboard, jobs, CRUD adapters).
- Provide hooks for hosts to observe/filter/forward activity (e.g., audit, analytics).

## Core Contracts
- **Reference implementation target (from go-users)**: the shared contracts live in `github.com/goliatone/go-users/pkg/types`:
  ```go
  type ActivityRecord struct {
      ID         uuid.UUID
      UserID     uuid.UUID
      ActorID    uuid.UUID
      Verb       string
      ObjectType string
      ObjectID   string
      Channel    string
      IP         string
      TenantID   uuid.UUID
      OrgID      uuid.UUID
      Data       map[string]any
      OccurredAt time.Time
  }

  type ActivitySink interface {
      Log(context.Context, ActivityRecord) error
  }
  ```
  Any sink/query implementation in this repo or sibling modules must satisfy `types.ActivitySink` (method `Log(context.Context, types.ActivityRecord) error`) to remain drop-in compatible with go-users. When emitting, prefer the field names/types above; extra metadata goes in `Data`.
- **ActivityEntry**: `{Actor, Action, Object, Context, Metadata, Timestamp}`; align field names/types with go-users activity.
- **ActivitySink**: `Record(ctx, ActivityEntry) error`; `List(ctx, limit, filters...) ([]ActivityEntry, error)`; optional pagination/filtering by actor/object/action/date.
- **ActivityProvider** (optional): surfaces source-specific metadata/schema for UI rendering.
- **Configuration**: storage driver (in-memory, SQL via go-users), retention policy, enrichment hooks (add locale/resource IDs), filters (allow/deny actions).

## Cross-Package Expectations (who owns what)
1) **go-admin repo**
   - Inject a go-users-compatible `ActivitySink` into Admin; default in-memory for tests, pluggable DB sink for production.
   - Emit activity on: panel CRUD, settings changes, jobs trigger/completion, notifications create/read, CMS menu/widget/page/block edits (via adapters), module contributions where applicable.
   - Expose activity feed API and dashboard widget backed by the shared sink (not just in-memory).
   - Config surface: allow persistence selection, retention, and filtering.

2) **go-cms**
   - Add hooks to emit activity on content/page/block/menu/widget create/update/delete, publish/unpublish, reorder, and translation events.
   - Accept an injected `ActivitySink` (no hard dependency) and no-op gracefully when absent.
   - Include optional metadata (locale, path, menu code, widget area).

3) **go-notifications**
   - Emit activity on notification creation/update/read/unread.
   - Accept injected `ActivitySink`; include metadata (notification type, recipient).

4) **go-options**
   - Emit activity on settings create/update/delete (system/site/user scopes).
   - Accept injected `ActivitySink`; include provenance and scope metadata.

5) **go-dashboard**
   - Emit activity on widget layout changes (add/remove/reorder) and preference updates.
   - Accept injected `ActivitySink`; include user/area/definition metadata.

6) **go-crud and repository adapters**
   - Provide hooks so CRUD operations can call an injected `ActivitySink` with model/entity metadata.
   - Keep hook optional to avoid breaking existing consumers.

7) **go-job**
   - Emit activity on job registration, trigger, success/failure, schedule changes.
   - Accept injected `ActivitySink`; include job name/spec/status metadata.

8) **go-auth**
   - Optionally emit auth/session lifecycle events via `ActivitySink` (login/logout/password change) for admin auditing; keep adapters injectable and optional.

9) **go-users**
   - Expose activity-friendly user lifecycle hooks (create/update/role changes) and ensure the shared activity contracts remain the canonical definitions.

## Implementation Notes
- Keep interfaces small; avoid direct coupling to storage packages.
- Feature-gate activity emission where appropriate.
- Provide test fakes/stubs for sinks and add unit tests in each package to assert emission.
- Document expected metadata keys per package to aid UI rendering.

## Deliverables for External Teams
- Adopt the `ActivitySink` interface and inject it where mutations occur (in your package).
- Emit `ActivityEntry` with meaningful metadata per operation.
- Add minimal tests using an in-memory sink/fake to verify emission.
- Document integration points and configuration flags so hosts can wire the sink.

## go-cms Integration Notes (see CMS_TSK_7 Phase 11)
- Dependency: use go-users activity contracts directly; helpers available in `github.com/goliatone/go-users/activity`:
  - `BuildRecordFromUUID(actorID, verb, objectType, objectID string, meta map[string]any, opts ...RecordOption)` for services that only have actor IDs.
  - `BuildRecordFromActor(actorCtx, verb, objectType, objectID string, meta map[string]any, opts ...RecordOption)` for hosts that pass `go-auth` actor context.
- DI/config: expose an optional `ActivitySink` (default no-op) via container options; keep emissions feature-gated.
- Emission scope: pages/content/blocks/widgets/menus services should log create/update/delete/publish/unpublish/move/reorder events, using actor IDs from DTOs and metadata like `{locale, path, status, template_id, menu_code, region}`.
- Testing: add fakes and assertions that emissions occur on success and are skipped on failure; include a DI smoke test to confirm the injected sink is invoked.
- Docs: record configuration and expected metadata in README/docs and reference these notes from `CMS_TSK_7.md` Phase 11.
