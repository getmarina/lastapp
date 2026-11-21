# Lastapp — shared business backend and agent tooling

A Go API and React/Tailwind console with separate Platform and Tenant workspaces, email/password accounts and optional OIDC, real MariaDB provisioning, shared identities/teams and scoped roles. See [FOUNDATION.md](FOUNDATION.md) for the implemented role model and APIs. The entity designer, generated storage, business-object ACLs/rules, App sessions, TypeScript data SDK and delegated management MCP are implemented.

**Foundational principle: DRY.** Rules and policies have one authoritative source, shared by all consumers. Keep implementations simple and reuse existing contracts and components. See [AGENTS.md](AGENTS.md) for the persistent engineering rules.

**Business entities:** [Usage and implementation guide](ENTITY_RUNTIME.md) · [Design and delivery plan](ENTITY_DESIGN_IMPLEMENTATION_PLAN.md). In a Tenant workspace, open **Space → Entities**. App origins and ceilings are approved under **App → Settings → App data access**.

**Agent MCP:** connect your agent to `http://127.0.0.1:8787/mcp` (or your deployment's HTTPS URL). The client discovers OAuth and opens Lastapp for sign-in and explicit approval; there are no API keys or manual client registrations to create. The URL is also under **Connect agent** in the top bar and **My account → Agent access**. Agents manage configuration using the same guarded backend operations as the console. See [MCP.md](MCP.md) for connection, tools, context, delegation, retries and security boundaries.

**For new contributors and agents:** start with [AGENTS.md](AGENTS.md) and [DEVELOPMENT.md](DEVELOPMENT.md). [CLAUDE.md](CLAUDE.md) imports the same instructions. These current guides distinguish implemented behavior from historical plans and deferred product ideas.

**Open locally:** http://127.0.0.1:8787/console/

Use **Continue with SSO** to open development SSO, then choose **Platform operator**, password **`lastapp-local`**. The issuer also offers **Tenant administrator**, **Joe (Marketing Reviewer)** and **Outside user**, with the same development password. Signing in does not grant platform administration. The **Northstar demo** contains a CRM Space with published Account/Contact/Deal entities, Marketing/Engineering teams, and two Apps (Deal review, Forecasting), each with a deployed static release (see [FOUNDATION.md](FOUNDATION.md)). The normal login form accepts the email/password accounts created by platform administrators through Tenant provisioning or **Users → Create account**.

## Run

Requirements: Go 1.25+, working Node/npm, and either MariaDB server/client/install tools or Docker. Verified on this Mac with Go 1.27.4, Node 24.20.0 and MariaDB 12.3.3.

Without a local `mariadbd` (or with `LASTAPP_MARIADB=docker`), `make dev` runs MariaDB from [docker-compose.yaml](docker-compose.yaml) instead: the same `mariadb:12.3` image as CI, published only on `127.0.0.1:33317`, data in the `lastapp_mariadb-data` volume, root password generated into `.local/mariadb.env`. Database accounts then use host `%`, because published-port connections reach the container from the Docker gateway rather than `127.0.0.1`. `make stop` stops the container and keeps the volume.

```sh
git clone https://github.com/aantich/lastapp.git
cd lastapp
make dev
```

`make dev` builds the frontend and embeds it in the Go binary, applies migrations, seeds development identities/plans and the Northstar demo (before the server starts, so the seed runs its queued work itself), and starts three detached local processes:

| Service | Address |
| --- | --- |
| Admin UI and API | `127.0.0.1:8787` |
| Development OIDC issuer | `127.0.0.1:8788` |
| Private MariaDB | `127.0.0.1:33317` |

Data, keys, credentials, process IDs and logs are in ignored `.local/`. The script uses its own MariaDB data directory, refuses unrelated processes on its ports, and rebuilds/restarts the app and issuer while preserving data/keys. This undeployed foundation initializes `lastapp_foundation`; the earlier `lastapp_control` registry remains intact, with its previous configuration saved privately. There is no legacy-grant migration. To start already-built binaries, use `python3 scripts/dev.py --skip-build`.

When the Node on PATH fails, the script falls back to `$HOME/.nvm/versions/node/v24.20.0/bin`; it does not change global Node or Homebrew. `make` uses `node`/`npm` from PATH; pass `NODE_BIN=/path/to/node/bin` to prefer another Node.

```sh
make stop                  # stop these local services; preserve data
make test                  # Go unit tests and compilation; DB tests skip by default
make lint                  # Go and console linters/type check; make fmt formats Go
make mcp-test              # MCP/agent regressions on a disposable test MariaDB (Docker)
make client                # build the data SDK and the example App's client.js
make integration           # all Go tests on a disposable test MariaDB, then make e2e; dev data untouched
make e2e                   # Chrome tests on a disposable MariaDB/API; dev data untouched
cd web/admin && npm run storybook  # local UI library at http://127.0.0.1:6006
```

Do not delete `.local/config.json` while retaining its database: its encryption key is needed to read stored connection credentials. The development identity provider is explicitly loopback-only. Production can use local accounts, trusted OIDC, or both.

## Review the UI

1. As **Platform operator**, open **Tenants → Create Tenant**. Choose a plan, enter the administrator's name/email/initial password (or generate one), or select an existing account. An existing account's password is never replaced.
2. Sign out, then sign in with that email/password. Replace the initial password to unlock the workspace. Alternatively, use the development **Tenant administrator** SSO account to inspect the Northstar demo.
3. Tenant users land on **Home**: what needs them now (failed or running deployments of recent Apps they may release, with **Retry deploy**; their own workspaces being prepared, with the worker's step; entities with unpublished changes in Spaces they may author; databases being prepared or failed; agent access expiring within a week), counts of Spaces, Apps, micro-apps, databases and active agents, the most recent Apps, connected agents and the latest audit events with agents shown as actors. Every figure comes from the same authorized management reads as the other screens. Home greets you with the date, says how many things need you, shows Entities, Apps live and Agents active, the attention cards (or **All clear**), App cards with their production address and release, and the latest activity in plain sentences. The sidebar holds the Tenant switcher right under the Lastapp brand (a button with your current Tenant's initials and name, shown even with one Tenant; it opens a searchable list that pages through your Tenants), **Search or run…**, **Home, Apps, Entities, Access, Agents, Activity** filtered by authority, a **Spaces** list (the title opens all Spaces; each Space shows its status, and the development workspaces you manage are listed under their Space with a "workspace" tag) and your account at the foot; it collapses to an icon rail (remembered on this browser). **Apps** is the complete searchable, paginated directory; App-only managers need no parent-Space visibility. The top bar shows breadcrumbs, a search field and **Connect agent** (the MCP URL and how to connect). Your broadest fixed role in the Tenant (for example Tenant administrator) is shown under your name. **⌘K / Ctrl+K** opens the command palette: the sidebar sections, the current page's and the Members and teams/Settings tabs, Apps, Spaces and entities you have already loaded in this session, and the account actions.
4. **Spaces → Create Space:** enter a name. The current user becomes manager; managed creation uses the effective Tenant/plan database layout and an eligible server, reusing the Tenant's shared database where applicable. **Advanced** exposes slug, manager, layout/server and existing-resource choices. A Space opens on **Apps**, with **Development** (local workspaces, for workspace managers), **Entities**, **Access** and **Settings**; database details are in Settings.
5. An App opens on **Micro-apps**, with **Releases** (for release managers), **Access** and **Settings**. Create Apps and micro-app registrations by name; technical metadata and compute preferences are under Advanced. The compute preference remains editable under App **Settings → Edit App → Advanced**. These preferences and registrations do not execute Apps.
6. **Access** in the sidebar opens the current (or first) Space's **Access**. **Members and teams** (account menu, or the button on Space Access) → **Members:** Tenant administrator checkboxes and Tenant access controls are directly on member rows. Add existing local accounts by email. **Teams** opens a team's members with Administrator/Reviewer checkboxes on the same rows. Space **Access** shows, for one entity at a time, a read-only role matrix (the roles and record relationships its permission paths name × operations, published state with draft changes marked; every holder of a role is counted, beyond the first page of assignments; a cell opens the entity on its Access tab, where paths are edited), then **Assignments** (role → person or team, scope; **Remove** asks for confirmation; **Add access**) and **Roles** (create/edit custom roles). App **Access** shows its own Assignments and, marked **Inherited** and read-only, its Space's, then Roles. The last active Tenant administrator remains protected. Existing roles can be revoked while the account or access is disabled; new assignments require active recipients.
7. Tenant **Settings** (account menu → **Tenant settings**) contains **General** (information and plan limits), **Databases** and, for administrators, **Agent access** (members' agent connections: owner, scope, permissions, expiry; **Revoke** asks for confirmation). **Agents** in the sidebar opens **Agent connections**: every connection in the Tenant for an administrator, otherwise your own, with **New connection** (the MCP URL). Existing database registration preserves business tables; connection checks/retries, credential replacement and retirement use the provisioning API. The **account menu** (your name at the foot of the sidebar) contains **Members and teams**, **Tenant settings** (for administrators), **My account** (profile, password change, your agent connections), **Appearance** (a dialog: Light, Dark or System) and **Sign out**; saved forms confirm with a short notification.
8. Platform navigation is **Tenants, Users, Activity, Settings**. Open a Tenant for **Overview, Administrators, Settings**. **Administrators → Add administrator** creates or selects an account, activates its membership and grants Tenant administration atomically. Existing passwords are never replaced. **Users** contains platform administrator checkboxes and global account enable/disable; **Settings** contains database servers and plans. **Users → Add to Tenant** adds an existing local or SSO identity as an ordinary member, with no automatic administrator grant. Removing an administrator and disabling an account ask for confirmation. Platform accounts cannot author Tenant Spaces/Apps or hold Tenant/team roles.

Console URLs identify the current Tenant, Space/App and tab. Reloads, direct links and browser Back/Forward preserve that location. Searchable pickers load results and further pages inside one control; list pagination disappears when unnecessary.

Expired or revoked sessions return to login automatically, including when signing out an already-expired session. Incorrect login/current-password responses remain ordinary form errors. Starting a new password session or rotating credentials invalidates older request generations so a delayed old-session response cannot close the new session.

The `Starter` plan allows one shared managed database, three Spaces and ten Apps. `Development` permits both layouts/methods, dedicated-compute preferences, 100 Spaces, 500 Apps and 100 database resources. Plans apply to future allocations; they do not migrate existing resources.

## Implementation

- `internal/control`: HTTP transport and wiring and encrypted database credentials; the background worker leader and its ordered steps are in `internal/worker`, database provisioning in `internal/databases`; local/OIDC console sessions are in `internal/session`, and agent OAuth and the management MCP server in `internal/agent`. Domain operations live in `internal/inventory`, `membership`, `roles`, `databases` and `entities`, workspaces/builds/releases/deployments in `internal/delivery`, entity publication in `internal/publisher` and the App data API (`/data/v1`) in `internal/dataapi`, on top of `internal/op` (actors, mutation runner, errors), `internal/access` (live authorization) and `internal/registry` (typed registry queries); the full package map and dependency direction are in [DEVELOPMENT.md](DEVELOPMENT.md#contract-and-implementation-map). `internal/authz` defines the shared typed scopes and fixed control catalog. No ORM or policy service.
- `cmd/lastapp`: `serve`, `migrate`, `bootstrap`, `seed-development`, `runtime-grants`.
- `cmd/devoidc`: development-only signed OIDC issuer supporting discovery, JWKS, code exchange, PKCE and nonce.
- `web/admin`: React UI, Tailwind and shared form/table components, served from the Go binary at `/console/`.
- `design/control_plane.sql`: canonical registry schema, kept equal to the executable migrations. It includes local accounts, entities/publications, App-bound sessions and agent OAuth/delegation/receipt tables. Regenerate from migrations with `python3 design/validate_schema.py --write`; check parity with `--check`.
- `third_party/go-mariadb`: source snapshot of **[aantich/go-mariadb](https://github.com/aantich/go-mariadb)** from sibling `../go-mariadb`, including its compression implementation and license notices. `SNAPSHOT.json` records hashes. All connectivity uses its `mariadb` package and `database/sql` connector. No MySQL fallback driver is installed. The snapshot avoids silently changing Lastapp when another chat edits the driver; intentionally refresh/pin it when adopting a reviewed driver change.

The hierarchy is **Tenant → Space → App → micro-app installation**. A Space has one fixed database binding. Spaces can share only within the same Tenant, and dedicated bindings are exclusive. All identities, memberships, groups, permission definitions, roles and assignments live in the central registry. Record-specific shares and business audit stay beside the records and reference global identities and permissions.

Session tokens are hashed at rest; writes require CSRF and exact Origin checks. Authorization reads current grants on each request. List queries apply scope predicates before selecting rows. Cross-Tenant references have composite foreign keys as a second guard. Revisions prevent stale overwrites. Client-generated UUIDs make duplicate creation detectable; retries must reuse the original UUID.

Credentials use AES-GCM with random nonces, key IDs and authenticated owner/Tenant context. API responses and audit events omit secrets. Migration credentials, registry runtime credentials and target provisioning credentials are separate; the development registry account can append/read audit events but cannot update/delete them.

A pinned MariaDB advisory lock elects one provisioning worker. It reconciles durable resource rows; a separate target resource lock prevents simultaneous external DDL. Generated names, stored passwords and database ownership comments make retries stable. Unknown databases/accounts are refused. Retiring registry entries never drops physical data. Schema rollback is deliberately not destructive: restore a backup for rollback.

## Production configuration

Copy `config.example.json` to a protected file and replace its placeholders. Use a separately privileged migration DSN with `lastapp migrate -config /path/to/migration.json`, then run `lastapp serve -config /path/to/runtime.json`. Bootstrap an exact issuer/subject identity using `lastapp bootstrap -config ... -issuer ... -subject ...`; email is never a privilege source.

For local authentication without OIDC, leave the OIDC issuer/client fields empty and bootstrap a dedicated platform account with a private initial-password file:

```sh
lastapp bootstrap -config /path/to/runtime.json -name "Platform operator" -email operator@example.com -password-file /private/path/initial-password
```

The file contains only the initial password (8–128 characters); keep it private and remove it after bootstrap. The first login requires a new password. To grant an already-existing local account platform administration without resetting its password, omit `-password-file`. Platform and Tenant administrative accounts remain separate.

Local passwords use random 16-byte salts and PBKDF2-HMAC-SHA256 with 600,000 iterations, matching the [OWASP PBKDF2 work-factor guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#pbkdf2). Passwords are never encrypted/recovered or included in API/audit output. Login has per-peer/per-email throttling and bounded hashing concurrency. Changing a password revokes old sessions and rotates the current session and CSRF token. OIDC identities are never automatically linked by email. Email recovery delivery and MFA are later work.

Production requires an HTTPS public origin, trusted HTTPS OIDC when enabled, identity-verified database TLS and an operator-defined runtime account host/network policy. Terminate HTTPS at the trusted ingress. `allowed_networks` explicitly permits private database address ranges; public addresses are otherwise permitted. DNS results are checked again at connection time. Register each physical server under one canonical endpoint: equivalent DNS aliases are not automatically deduplicated.

When a reverse proxy forwards client addresses, configure `trusted_proxies` with its exact connecting IPs or CIDRs (for example, `["10.0.0.10/32"]`). The default empty list ignores forwarded headers. Trusted ingress must overwrite `X-Forwarded-For` or append the actual connecting client; Lastapp walks the chain from right to left and stops at the first untrusted hop. Login budgets then distinguish clients behind that proxy while retaining the per-email limit. Untrusted peers cannot spoof a new budget through forwarding headers; malformed/oversized chains fall back to the direct peer. Do not add client networks to the proxy trust list.

The deployment encryption key is a base64 32-byte key. This first version uses one configured key generation (`encryption_key_id`, default `primary-v1`); automatic key rotation/re-encryption is not implemented. Preserve keys with registry backups.

## Business packages

Tenant managers can clone an approved package as business objects or as objects with a prebuilt App. Installed Apps and their source can be customized in development workspaces. See [starter business packages](PACKAGES.md) for catalog publishing, installation and MCP tools.

## Scope of this phase

Implemented: centralized identity/inventory, scoped role management, fixed Tenant/team catalogs, custom Space/App business roles and assignments, plan quotas, lifecycle management, real database provisioning/registration, retries/recovery, auditing and the matching admin UI.

Implemented agent stage: OAuth discovery/automatic connection, existing-user scoped delegation, typed management tools, live context/contracts, receipt-safe mutations and console consent/access management. No business-record tools or robot accounts. See [the MCP guide](MCP.md).

Implemented entity stage: Space-owned definitions and publishing, generated storage, entity/record/field ACLs, defaults/calculations/validation/allowed changes, bulk CRUD and filtered/paginated reads, optional record sharing, App-bound authentication and a small TypeScript SDK. See [the entity guide](ENTITY_RUNTIME.md).

Implemented development/deployment stage: owned isolated workspaces, local React/SDK starter, bounded filesystem build uploads, immutable releases/previews and explicit production deployment. Open **Space → Development**, then **App → Releases**: one ship flow from workspace to build, release and deployment, with a live stepper of the production deployment's states and **Retry deploy** (a new deployment of the same release) after a failure. **Activity** is the audit feed of people, agents acting for people and the system worker, filtered on the server by actor (**All, People, Agents, System**) and kind (`GET …/audit-events?actor=&kind=`, kinds from `GET …/audit-kinds`). Remote agents connect through MCP and use their own local tools. See [WORKSPACES.md](WORKSPACES.md).

Later: remote builds, browser Node runners, Git/object-storage integrations, App container scheduling, external connectors, billing and production operations. `compute_mode` records a preference; it does not start containers. Artifact references are metadata. Inventory collections use bounded pages (`?page=1&limit=100&q=search`, maximum limit 200) with `pagination: {page, limit, has_more}`. Scope predicates apply before search and pagination. The console pages/searches those lists and selection controls, including audit history and group members. The entity designer uses the complete Space entity inventory within its entity-count limit; MCP entity inventories expose explicit pagination/completeness. Business-record queries use the separate SDK cursor contract, not management page numbers.

See [FOUNDATION.md](FOUNDATION.md), [CONCEPT.md](CONCEPT.md), and [VERIFICATION.md](VERIFICATION.md). The provisioning plan describes the earlier implementation; its Tenant custom-role model is superseded by the foundation.

## Admin appearance

The console uses one theme, **Signal** ("Ink & Signal"): warm paper canvas,
white cards, an ink sidebar, signal-blue accents, Inter body text, Inter Tight
headings and JetBrains Mono identifiers. Use **account menu → Appearance** to
choose **Light**, **Dark** or **System** color mode. The preference is saved on
this browser and applies across console screens and forms; a preset saved by
the former Neo/Studio/Evergreen themes is ignored and its mode is kept. On
narrow screens the sidebar (with the Tenant switcher and account menu) becomes
an offcanvas menu and the top bar keeps the current page, search and
**Connect agent**.

The theme is a set of semantic `--ui-*` CSS variables in
`web/admin/src/themes.css` (its parts in `src/theme/`), exposed through Tailwind 4 utilities such as
`bg-surface`, `text-ink`, `text-primary`, `border-line` and `rounded-panel`.
Browse the tokens and shared controls in the local Storybook; see the UI
library section of [DEVELOPMENT.md](DEVELOPMENT.md#console-ui-library).

Security fixes and operating boundaries are recorded in [the remediation report](review/security-2026-11-12/FIXES.md). Run `make integration` for the API/security/browser suites; every suite runs on its own throwaway database, never the development data, and `make e2e` alone runs just the browser suite. The local Vite UI on port 5173 is an explicit development-only origin; login returns there, and writes use the proxy. Production accepts only its configured admin origin and requires verified TLS for TCP registry connections.
