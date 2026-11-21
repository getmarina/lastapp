# Local development, isolated workspaces and App releases

Implemented in the existing Go service. Source and builds stay local; Lastapp
stores immutable static build files on its filesystem. Git integration, remote
build execution, object storage and browser Node/WebContainer runners remain
future work. Browser-compatible React/npm code runs directly in the browser.

Workspace/release authoring initially requires a Tenant administrator or source
Space manager. An App-only manager cannot promote shared Space configuration.

## User and agent flow

1. Connect Codex/Claude to the deployed service's `/mcp`. Sign in and approve a
   Space-scoped **Workspace author** grant. This permits bounded development,
   not production deployment. A local agent uses its own terminal/files/browser.
2. Inspect `lastapp_context`, then create a workspace with `lastapp_workspace`.
   List `resource=workspace` under the production Space; get its returned ID until
   `state=ready`. Every agent connection gets its own owned workspace.
3. Target its `backing_space_id` with the ordinary entity, role, permission and
   App tools. Inspect the full entity inventory and relevant published contracts
   before creating concepts. The private backing Space reuses the ordinary
   authorization, schema publisher and data engine; it is not another public
   hierarchy level.
4. Get `resource=starter`, ID = development App ID. Write the returned files
   locally. The console also downloads a ready-to-edit ZIP from **App → Releases**.
   The React/Vite starter embeds the authoritative SDK, one UI module and shared
   CSS. Dependency versions derive from the console's tested lockfile. Install
   dependencies, build and test with the agent's local tools.
5. Approve the exact local App origin with `lastapp_app_access_update`. An owned
   workspace can use loopback HTTP even when Lastapp is deployed with HTTPS.
   Ordinary production Apps keep the existing secure-origin rules.
6. Use browser Connect, or obtain a one-use PKCE code with `lastapp_workspace
   action=launch`. Generate the verifier/challenge/nonce locally; send only the
   challenge, nonce and exact origin to MCP. Exchange in the browser with
   `client.connectDevelopment({code, verifier, nonce})`. The SDK keeps the App
   bearer in memory. Never place launch codes, verifiers or tokens in builds.
7. Create reusable fixture scripts through the ordinary workspace App SDK. The
   management MCP still has no business-record tools. Normal ACLs apply: an
   administrator does not automatically receive business-data access. Space/App
   business roles must be assigned explicitly within the development copy.
8. Build locally. Obtain `lastapp_release action=upload_ticket`, upload the build
   with its short-lived single-content ticket, then capture a release. The local
   starter's upload script and the console's folder upload use the same endpoint.
   Pin every active micro-app **by slug** and pin shared dependencies using exact
   versions or SHA-256 identifiers. Keep versions of unchanged micro-apps.
9. Review the immutable release's full Space snapshot and manifest. Activate its
   workspace preview and test against the actual workspace APIs. A preview uses
   the workspace's live APIs; subsequent development changes require a fresh
   capture before preview activation. Activation validates the settled snapshot
   under the same mutation lock as client selection, so concurrent Space changes
   cannot slip past review. Keep fixtures reproducible locally.
10. Explicitly approve `production.deploy` on the production Space when ready.
    Deploy the reviewed release using the current production App revision (1 for
    a new App) and a fresh request key. Poll `resource=deployment` until active or
    failed. A queued response is not success. Recover a lost job ID using
   `lastapp_get resource=app_deployment`, ID = development or production App ID.
   App context also includes the latest/blocking deployment. The console restores
   this status on reload and retries transient status errors.

A console author uses **Space → Development → Create workspace**, then opens its
Apps and Releases. The Development tab appears on ordinary (non-workspace) Spaces
for authors with `workspace.manage`; an App's Releases tab requires
`release.manage`. Human-created workspaces belong to that human; a connected
agent creates its own workspace (marked **Agent workspace**). The human owner can
inspect that agent's copy. The workspace list refreshes itself while a workspace
is preparing and shows the step the worker reached (`preparation_step`, one of
`development_policy.workspace_steps` in `GET /v1/config`: `storage → tables →
publishing`, then `ready`; a failed workspace keeps the step it failed at) as
"Step N of M"; Home shows the same progress for the author's own preparing
workspaces. **Retry** re-runs a failed preparation and **Archive** asks for
confirmation.

An App's **Releases** tab is the ship flow. In a workspace App, **Ship this App**
shows the way from the workspace to an uploaded build, a release and a
production deployment, with **Get starter** and **Upload build folder**. The
**Production deployment** card follows the deployment's real states, which the
server publishes in `GET /v1/config` (`development_policy.deployment_states`:
`queued → preparing → publishing → active`, or `failed`); the console polls
while a deployment is in a running state. A failed deployment records the step
it had reached (`failed_state`: queued, preparing or publishing), and the
timeline marks that step with its stored error and **Retry deploy** (also on
Home and in the command palette).

Retrying (`POST /v1/deployments/{id}/retry` with `If-Match` = the deployment's
revision; MCP `lastapp_release action=retry`) re-queues the **same** deployment
record for the same release and target App and runs it again from its first
step, counting `attempts`. A failure discards the entity versions it queued, so
no step resumes where it stopped: the retry queues fresh versions through the
same worker steps, which recheck live authority, the build and the storage
fences as before. It is refused unless the deployment failed, production's
configuration is exactly what that failure left (`failed_base_hash`, recorded
while the Space was still fenced), every active entity draft is either
published or the one this deployment staged, and the caller has the deployment
authority now. The caller's authority (and delegation) then carries the new
attempt (`retried_by`), the audit trail records `release.deployment_retried`
and the activation under that person, and the retried job joins the end of its
Tenant's deployment queue. A second retry of the same failure is stale (412).
Release history lists each release with its age, creator
(`created_by_name`) and, for an agent workspace, the agent client
(`agent_client_name`); **Details** opens its build, micro-app pins and
captured Space backend.

## Isolation and ownership

- Each workspace has a new managed database. Production records, credentials,
  role assignments and approved App origins are never copied. Only published,
  settled configuration is copied; publish or archive unfinished active drafts
  before creating a baseline or capturing a release. Published ACL references to
  subsequently disabled users/teams remain present and inert in the copy;
  ordinary authoring still rejects new permissions for inactive recipients.
- Users and Tenant/team membership remain global. Workspace business-role IDs,
  App IDs and entity IDs are separate and map to stable production identities.
  Only typed references are rewritten; arbitrary literals/descriptions are data.
- Live ownership and source-Space authority are required. One agent grant cannot
  access another grant's copy even when both delegate the same human. Source
  suspension, membership removal, workspace archive or grant revocation stop
  access. Development App sessions are also bound to the grant revision.
- Undeployed release snapshots use the same private workspace boundary, including
  the owning agent grant. Queued or failed deployments do not make them shared.
  Successfully deployed releases remain inspectable as production history by
  authorized source-Space managers after the workspace is archived. A failed
  package activation can be retried by a current production manager using the
  recorded package plan. Private workspace and undeployed source access still
  require the workspace owner.
- Ordinary direct production mutations by an agent now additionally require
  `production.deploy`; their existing draft/access/resource permissions still
  apply. A development delegation cannot quietly switch its target to production.
- App assets run on separate App hostnames. They never run on the console origin.
  The same binary serves those hosts through a separate static-only router;
  console, auth, management and data routes are unavailable on App hosts.
- Micro-apps compiled into one App share that App's origin and authority. Separate
  App origins do not isolate hostile modules inside the same compiled App. A
  separate third-party micro-app sandbox/runner remains future work.

## Releases and production changes

An App release pins one compiled static build, micro-app/shared dependency
versions and a **shared Space backend snapshot**. The snapshot includes entities,
Space/App business-role definitions, App access policies and registrations. It
excludes records, role assignments, external origins and secrets. Production
assignments are preserved; new custom roles have no production assignments.

A release has an immutable production base hash. Deployment rejects concurrent
production configuration changes and fences other configuration mutations in
that Space while preparing. Identity mappings retain existing production IDs
and allocate stable IDs for new resources. The worker verifies immutable build files, prepares new tables,
queues captured entity definitions through the ordinary safe publisher. Deployment
publications stop at `prepared`: their storage is verified while live registry
versions, role states and App access policies remain unchanged. The final registry
transaction activates all entity ACLs, role states, App policies and the selected
static release together. New reference tables can be prepared together, including
cycles. Other Apps retain their selected static builds and origins while reusing
the updated Space entities and policies.

DDL and physical gates span separate database transactions. Changed entity APIs
can return `publication_in_progress` while preparing. Physical versions open before
the registry commit; older runtime snapshots fail the version check rather than
combining old ACLs with new authority. A failed activation rolls back registry
security changes, clears pending publications, and restores previous physical
versions only where the layout is verified compatible. Compatible DDL and draft
metadata can remain after failure; incompatible layouts stay closed for inspection.
Inspect failed jobs/publications; retry the failed deployment once the cause is
removed, or reconcile from a fresh workspace once production has changed. There
is no destructive rollback or backfill bypass.

After success, the originating workspace receives the new production base for
future releases. Previously captured releases keep their original base hashes.
This supports repeated local edit/build/test/deploy cycles without treating an
old release as an authorized rollback. Other stale workspaces must reconcile
from a fresh baseline; there is no automatic three-way merge or Git workflow.

## Deployment configuration and filesystem limits

Set `artifact_directory` to an absolute persistent directory on the service
host. Development defaults to `.local/artifacts`. Set `app_origin` to a separate
HTTPS base origin, for example `https://apps.example.com`, and route wildcard
DNS/TLS for `<app UUID>.apps.example.com` to Lastapp while preserving Host.
Startup rejects a console hostname equal to, or below, the App base hostname;
the entire App wildcard namespace is static-only. Ports, case and a trailing dot
do not make overlapping hostnames safe.
Remote OAuth clients still connect to the main HTTPS API/MCP origin.
Local development defaults to `<app UUID>.localhost:8787` for App hosting.
The browser obtains its actual API origin and environment App ID from
`/_lastapp/config.json`; those values are not compiled into the promoted bundle.

`workspace_database_target_id` optionally selects a platform-managed development
database target, useful when production uses a registered/external database.
Otherwise allocation uses the source Space's database target. The selected
target must permit managed provisioning. Ordinary workspaces count against the
plan database quota. Catalog package stages have a separate per-Space preview
capacity. Archiving one of those previews permanently removes its generated
database and test data through the background worker.
Workspace Apps use the separate workspace metadata cap exposed in the development
policy. A full production App quota does not block development creation or
restoration; production creation/restoration and deployment still enforce it.

Compiled builds upload as a file tree, with an optional private source ZIP.
The server bounds the body, total/file sizes, file count, simultaneous uploads and
outstanding tickets. Paths reject traversal, dotfiles, reserved runtime paths
and unsafe encoding; filesystem reads stay confined to the storage root.
Tickets are hashed, scoped, short-lived and checked against current authority.
The server checks build quota before storing artifacts and reclaims unreferenced
files if the upload transaction fails. Identical retries return the existing
build, and changed-content replay fails.

Current limits are owned by `internal/artifact/store.go` and
`internal/registry/policy.go` (served by `delivery.Policy` in `internal/delivery/policy.go`), exposed by `/v1/config` and MCP context. Builds and releases are capped per development App;
workspaces are capped per author/source Space. Archive stops development access
and previews while retaining storage and deployed releases. Automatic storage
reclamation is not implemented. Static bundles are public files: secrets and
sensitive fixture data must never be included in them.

A single local volume supports one instance. Multiple deployed replicas require
one shared persistent filesystem until the artifact storage implementation is
replaced with object storage. This stage supplies no replica scheduler, remote
build farm, arbitrary Node runtime or automatic release rollback.

## Source map and checks

The operations are in `internal/delivery` (`delivery.Service`, wired in
`control.Open`); console handlers, the upload endpoint, the App host server and
the worker leader loop stay thin in `internal/control` and call them, as the MCP
tools do. `internal/delivery` does not import `net/http`.

- `internal/delivery/workspaces.go`, `snapshot.go`: ownership, copying,
  identity mapping, workspace preparation (`WorkspaceJob`) and the shared
  publication capture (`QueueDefinition`, also used by entity authoring).
- `internal/delivery/sessions.go`, `internal/dataapi/sessions.go`: development and
  console launch codes, console/delegation parent checks, PKCE and live
  App-session revocation; `packages/client/src/index.ts`: SDK.
- `internal/delivery/artifacts.go`, `internal/artifact`: tickets, upload
  authorization and storage, hosted build lookup; `internal/control/artifacts.go`:
  the upload request limits/decoding and isolated host routing.
- `internal/delivery/releases.go`, `deployments.go`: captured plans, concurrency
  fences and durable preparation/activation (`DeploymentJob`, run by the
  deployments loop of `internal/worker`: Tenants take turns, a Tenant whose
  oldest deployment is waiting for its publications is skipped, and each
  Tenant's deployments apply strictly in the order they were queued).
- `internal/agent/development.go`: two typed MCP operation tools. Ordinary list/get/context
  expose workspaces, builds, releases, deployment status and local starter files.
- `packages/starter`: authoritative starter templates; `web/admin` supplies the
  console workspace/release screens and tested package versions.

`go test ./internal/delivery` runs database-free tests of snapshot remapping,
contract hashes, pinned versions and hosted origins, and (with
the disposable test database of `scripts/testdb.py`) disposable-registry tests of deployment candidates, the
production App quota and capture readiness. The HTTP/MCP flows below are
black-box tests in `internal/control`.

Tests include isolated record storage, compatible production schema changes,
file confinement/replay, cross-agent authorization, development-only grants,
PKCE/revocation and a real local React build → workspace browser API → production
browser API flow. See `VERIFICATION.md` for executed results and limits.
