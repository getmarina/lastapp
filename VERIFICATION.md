# Lastapp verification

## Package review fixes — 21 November 2026

- Disposable MariaDB confirmed objects-only installation and App attachment on
  Starter, planning rejection when the managed target is disabled, and physical
  preview database cleanup after the target is re-enabled. A second authorized
  Space manager reviewed and retried a failed App installation after its private
  workspace was archived, without gaining access to the undeployed release.
- The full Go race suite passed. A focused race rerun on the final receipt code covered package, upload,
  inventory and MCP operations. They verified that upload quota rejection and a
  failed upload transaction leave no uncommitted artifacts, successful preview
  and activation no-ops bind their request keys, and source streaming uses a
  download slot while the MCP authentication pool is free. The attachment unit
  test distinguishes a newly required field from one supplied by a default.
- Eight Playwright package, workspace and release tests passed, including the
  discard confirmation at desktop and mobile widths. Lint, type checks, Go vet,
  migration parity and pinned-driver verification passed.

## Tenant-installable packages — 21 November 2026

- On disposable MariaDB 12.3, the full Go race/integration suite passed. The
  package lifecycle fixture published two synthetic versions, installed
  objects-only and full App packages in different Tenants, bound an explicit
  dependency via delegated MCP, and attached an App after an entity edit.
  Existing records and the sibling App revision survived. Coverage also
  includes stable plan and request replay, ordinary member discovery through
  role assignments, and retention of referenced artifacts during cleanup.
- A fault injected into final publication exercised the durable failed step.
  The original retry was rejected after an intervening production change.
  Reviewing the new base allowed the same installation to activate.
- The full browser suite passed all 40 tests. A later focused run passed all
  five package and development release tests. The package test checked desktop
  and 390 px review layouts, preview on its App origin, production activation,
  and retrieval of the registered source.
- Package contract tests exercised the 64-item bound and rejected an extra
  item and oversized manifest input. A separate case verified cyclic references
  and independent destination IDs. The 64-entity
  decoder and validation fixture averaged 232 µs and 235 KiB allocated per
  operation on an Apple M4 Max. This measures the decoder and validator.
  Production throughput and peak memory under concurrent installations remain
  unmeasured.
- Go and console lint, TypeScript, Go vet, generated schema parity, and pinned
  driver integrity passed. [Package guide](PACKAGES.md) records the operator
  migration and artifact retention procedure.

## Console UI code rules: tokens only, kit only, 200-line files — 19 November 2026

- `npm run lint` (ESLint plus `scripts/check-css.mjs`) and `npm run
  typecheck` clean on `web/admin`; `go test ./...` passes (database tests
  skipped as usual). A probe screen with `w-[13px]`, a hex color, a raw
  `<button>`/`<input>`, a `style` prop, `<Plus size={16} />` and a `"12px"`
  string drew one error each; a probe stylesheet with a named color, a hex
  border, an `rgb()` shadow, a `0.2s` transition and `z-index: 5` drew six.
- Built stylesheet compared with `main`'s (`vite build --minify false`, the
  new scale tokens substituted, `4px * n` products evaluated): 957 rules on
  both sides; the only differences are equivalent spellings (flex
  shorthands such as `1 1 420px` / `420px`, nested `calc()`, animation
  shorthand order, `var(--ui-shadow-modal)` for the same shadow).
- Every `.ts`/`.tsx` (stories excepted) and `.css` file under `src` is at
  most 200 lines without blanks and comments; the largest before were
  `agent-access.tsx` 807, `tenant-workspace.tsx` 770 and `ui/kit.css` 3926
  (raw). Splits are moves; hook order, query keys, markup and class names
  are unchanged.
- Local Storybook build succeeds (`npm run build-storybook`). Browser e2e
  was not run locally; it ran in CI on the pushed branch.

## Tests on a disposable test database only — 19 November 2026

Earlier entries that mention `LASTAPP_TEST_CONFIG`/`LASTAPP_SECURITY_CONFIG` on
`.local/config.json` ran on the development MariaDB; that is no longer possible.

- `python3 scripts/testdb.py -race -count=1 ./...` (the `make integration`
  backend half) on a fresh `mariadb-test` container (port 13319, tmpfs,
  per-run secrets in a temp dir): every package **PASS**, control 278.5 s,
  no skipped tests. Container and temp dir are removed afterwards.
- The first run found tests that had depended on the dev server: the
  registered business-database fixture (`businessDatabaseFixture`) targeted
  127.0.0.1:33317, and `TestMariaDBControlPlane` relied on the dev API's
  worker to provision. Both now use the test server (target address from the
  registry DSN; the test runs its own worker). Metadata-only targets use 3306.
- Guard: with `LASTAPP_TEST_CONFIG` pointing at the dev `.local/config.json`,
  the database tests fail immediately with "registry port 127.0.0.1:33317 is
  the development MariaDB". `TestGuardRefusesDevelopmentData` covers the
  port, the `lastapp_foundation` name, socket DSNs and non-development config.
- Dev registry, read-only `SELECT` before and after both full runs:
  `lastapp_foundation.tenants` 12 (latest 2026-11-18 22:50), `audit_events`
  385, 46 schemas, 43 MariaDB accounts. No change.
- `make lint` clean. Browser e2e (now sharing `scripts/testdb.py`) was run
  in CI.
- A merge-queue run then failed to bind 127.0.0.1:33318 for `mariadb-e2e`
  although nothing listened there. 33318/33319 lie in the Linux ephemeral
  range (32768-60999), so any outgoing connection could hold them. The
  disposable ports moved to 13318/13319, and `up` is retried (container
  removed in between) up to three times.

## Entity designer review fixes — 19 November 2026

- Backend (MariaDB, `TestEntityRuntime`): after v1 is published, publishing
  the unchanged draft returns 409 `conflict` ("The draft matches the published
  version…") and queues no version; `PublishEntity` is the one operation
  behind the console route and `lastapp_entity_publish` (tool description and
  its golden updated). All Go packages passed with
  `LASTAPP_TEST_CONFIG`/`LASTAPP_SECURITY_CONFIG` set; `make lint` clean.
- Browser (`tests/entities.spec.ts`, `tests/access.spec.ts`,
  `tests/layout.spec.ts` on the disposable e2e stack): the page has exactly
  one h1 (the entity name, already rendered by `RecordHeader`; h2s follow);
  the Fields tab shows the read-only permission paths beside the fields with
  the "not in any read path yet" callout; with no changes the Review dialog's
  **Publish v2** is disabled, a reason names it (`aria-describedby`) and the
  Drafted step reads "No changes to publish" (not done); on Space Access the
  sidebar has one `aria-current` item.
- Probe (same stack, not committed): the five pipeline steps share one row in
  the dialog and in the Fields-tab card at 1440 px; Fields (605 px) and
  Permission paths sit side by side at 1440 px; axe (wcag2a/aa, wcag21a/aa,
  best-practice) 0 violations on the Fields tab with and without a draft, the
  Review dialog, the Fields tab at 390 px and Space Access; no horizontal
  overflow at 390 px. A text-only visual review against `Entity.dc.html` led
  to a wider Fields track and removed a repeated reason in the dialog. Not
  addressed here: the pipeline/settings cards span the content column, not the
  entity list column as in the mockup, and the fields grid clips its
  Behaviour column at 390 px (existing grid behaviour).

## Access navigation, control role and inherited App assignments — 19 November 2026

- Backend (MariaDB, `TestScopedRecipientOptions`): `GET …/capabilities`
  returns `control_roles` broadest first (an App manager gets only App
  manager; the Tenant administrator gets Tenant administrator, then Space
  manager); `GET …/apps/{app}/role-assignments?include=space` lists the App's
  own assignments before its Space's, returns 403 to an App-only manager
  (who still reads the App's own) and 400 on a Space. All Go packages
  passed with `LASTAPP_TEST_CONFIG`/`LASTAPP_SECURITY_CONFIG` set
  (`internal/control` 119 s); `make lint` clean.
- Browser (`tests/access.spec.ts`): the sidebar's Access opens the Space's
  Access tab and stays highlighted on Members and teams; the account block
  names the role from `control_roles`; with 53 Space assignments the matrix
  names a holder whose only assignment is on the second page and counts
  "+49" for a role with 51 holders; a cell opens the entity on its Access tab
  (this also fixed a matrix link that changed the address without opening
  the entity: a second `useConsoleURL` user's state was not shared); App
  Access lists its App manager (removable) first, then inherited Space
  assignments with no action. Members and teams specs reach the page from
  the account menu.
- axe (wcag2a/aa, wcag21a/aa): 0 violations on Space Access, App Access and
  Members and teams at 1440 and 390 px; no horizontal overflow at 390 px.
  Local runs shared the machine's single e2e database port with other
  stacks; two runs were lost when another run replaced the container.
- CI run 44397554328 (commit 8ded208, rebased on main after #29) passed
  lint, schema, race tests and the full browser suite. A text-only visual
  review against the mockup found no material problem; it noted that the
  open Space stays highlighted in the Spaces list beside Access (as it does
  beside Entities).

## Workspace preparation steps, Home attention and deployment retry — 19 November 2026

- `TestDevelopmentWorkspaceReleaseFlow` (MariaDB) checks that a new workspace
  starts at `storage`, that the list shows `publishing` after the first worker
  pass, and that the step is cleared once the workspace is ready. For both
  injected failures (activation conflict, corrupt build) it removes the fault
  and then checks the retry rules. A stale revision gets 412. A production
  Space change after the failure gets 409. Of two concurrent retries, exactly
  one gets 202 and the other 412. The same deployment then becomes active with
  `attempts` 2 and `retried_by` set. `release.deployment_retried` and
  `release.deployed` are audited under the retrying person. Retrying a
  deployment that is no longer failed gets 409.
  `TestWorkspaceStepsMatchSchema` keeps `WorkspaceSteps` equal to the
  `preparation_step` CHECK constraint.
- Goldens were regenerated for intended contract changes: the
  `lastapp_release` retry schema; retry stale/not-failed shapes, including over
  MCP without `production.deploy`; `attempts`/`retried_by`;
  `preparation_step`/`agent_client_name`; and
  `development_policy.workspace_steps`. `design/control_plane.sql` was
  regenerated for migration 00014.
- Local runs: the full Go suite passed, except `TestMariaDBControlPlane`. That
  test uses the shared development registry, which was not migrated to 00014
  (the dev stack was deliberately left untouched). `make lint` reported 0
  issues. Browser specs passed: the deployment recovery spec (now also covers
  the Retry dialog and its If-Match) and the new Home/Development progress and
  unpublished-entity spec.
- axe (wcag2a/aa, wcag21a/aa, best-practice) on Home, Space → Development and
  the App Retry dialog, in light and dark at 1440 and 390 px: 0 violations in
  all 12 states and no horizontal overflow. A text-only visual review against
  the mockup found the Development list's Preparing badge stretched across its
  column (fixed). Its remaining findings are pre-existing shared-kit details:
  database card title wrapping, the failed card border, attention-card
  spacing, and the field-less confirmation dialog.

## Apps list and App detail on the approved design — 18 November 2026

- The Apps list and the App page (Releases tab of a live App) were measured
  against the mockup's prototype in Chrome, region by region (box, padding,
  radius, colors, font shorthand), at 1440×900 and 1920×1080 on a private
  probe stack seeded with the Northstar demo. Header, "New app", the table
  card, header row, rows, tiles, status badges, the App header tile and
  title, page tabs, the Production deployment card, its steps (label and
  note type) and the Releases header are within 2 px with identical type.
  Before the change the page title was 26 px instead of 28 px and sat 4 px
  low. Remaining differences are data or mockup artefacts: hostnames are the
  real ID-based origins (widths), the mockup's rows overflow its card by
  31 px (its Preview button is clipped) while real rows fit, release rows have
  no "r23" number column (no safe source; titles start 93 px further left),
  the mockup's "↗" glyph makes its header buttons 41 px tall instead of 37 px,
  and the content height follows the number of releases.
- A failed deployment (patched into the deployment response, verification
  only) shows "<release> failed" in the header, a danger tile, "Production
  still serves <active release>.", Retry deploy, the failed step marked with
  the stored error, and the failed release listed with a Failed chip.
- axe (wcag2a/aa, wcag21a/aa, best-practice): 0 violations on Apps and on
  each App tab (Releases, Micro-apps, Data access, Settings) for two Apps, in
  light and dark, at 1440 and 390 px; no horizontal overflow at 390 px.
- A separate visual review of the screenshots found no material difference;
  its minor findings (the Launch app arrow) were fixed or are listed above.
- Backend: `TestDevelopmentWorkspaceReleaseFlow` (MariaDB) checks the App
  list's `hosted_origin`, `latest_deployment` and `active_release_id` against
  the deployment, that an App manager without `release.manage` gets no
  `latest_deployment`, `failed_state` "publishing" on a failed activation and
  none on success, and the release creator; goldens and
  `design/control_plane.sql` were regenerated for migration 00013.
- CI run 44343686226 (commit bf4941c, on main after #24) passed the
  backend, MariaDB and all 37 browser regressions. Local browser runs were
  not usable: with several stacks sharing the machine (load average ~50) the
  console took over 5 s to leave "Opening Lastapp…", so every spec timed out
  at sign-in. The earlier CI failures were real and fixed: the typed/untyped
  App row comparison lacked the new fields, the App page had lost Refresh
  with the shell header, and "active" matched the running note before the
  deployment was live.

## Tenant switcher under the brand row; floating list popup — 18 November 2026

- Storybook (App shell, ContextSwitcher, Select, RemoteSelect, FormModal),
  headless Chromium: the switcher is a 212×47 control under the brand row;
  its popup is portalled to `<body>` (inside the dialog for a FormModal
  Select), 4 px under the trigger (beside the 40 px tile on the rail), with
  the page surface colour in both modes, and opening it does not move the
  sidebar search field. Focus goes to "Find a Tenant"; Load more (5 per
  mocked page) appended a page with the popup open and focus kept; Escape
  closed it; a search with no result shows "No Tenants match". Select
  (keyboard and mouse, outside press), the FormModal Select and RemoteSelect
  still choose and close.
- axe-core on the sidebar and open popup: no violations in light and dark at
  1440 px, 390 px (drawer) and on the collapsed rail, except the drawer's
  existing `aria-allowed-role` (role=dialog on `aside`), unchanged here.
- Not run locally: the Playwright e2e suite (left to CI's `test` check).

## Access, Agents and agent consent on the approved design — 18 November 2026

- Space Access, the Agents page and the consent page were measured against
  the mockup's `access` and `agents` screens and its consent modal, region by
  region (position, size, padding, radius, colors, font shorthand), at
  1440×900 and 1920×1080, on a disposable stack seeded with real records (a
  Space with two custom roles, an entity with published paths and a draft
  change, an assignment, two consented connections and a pending consent).
  Agent cards match except text-length widths and the danger token (Revoke is
  `#7d150d`, the mockup `#9a1c12`; small button padding 7 px vs 6 px). Access
  matrix and assignment rows match in size and type; the whole block sits
  66 px lower under the Space tabs, and op columns start 35 px further left
  because `acl.manage` (the real operation name) is wider than the mockup's
  `acl` and the mockup's row button overflows its card by 32 px. The consent
  card's title, labels, preset cards and expiry control match in size and
  type; it is taller because it also shows the client ID, the unverified-name
  warning, the scope pickers and the single permissions disclosure, and the
  expiry choices are the served 1/7/30 days.
- axe-core reports no violations in light and dark mode on Space Access, App
  Access, Agents, My account and the consent page (the consent card's title is
  now the page's `h1`), and none of them overflows horizontally at 390 px
  (the matrix scrolls inside its own region).
- A separate text-only visual review of the screenshots found Agents faithful
  and Access/consent close; its findings on centred preset-card text and the
  selected-card tint were fixed and remeasured.
- `go test ./internal/agent` (preset descriptions, day choices within
  1..`max_days` including the default) and the integration golden
  (`config` shape with `day_choices`, `max_connections`, preset
  `description`) pass against the local MariaDB; `make lint` is clean. The
  targeted browser specs (agent access, review fixes, security, database
  access) were run against the disposable stack; the full suite runs in CI.
- Not verified: Space-wide assignments shown with App assignments in one list
  (the Space list shows Space assignments only) and opening an entity directly
  on its Access tab from a matrix cell (it opens the entity).

## Activity and the command palette on the approved design — 18 November 2026

- Server: `GET …/audit-events` accepts `resource_id` (at most 36 bytes),
  applied with `actor` and `kind` before search and paging under the same
  `audit.read` check. `TestAuditAttributionGolden` asserts that the filter
  returns exactly the App's events and no other, survives `limit=1`, rejects
  a 37-byte ID with 400 and still gives a member without `audit.read` 403.
  The golden output is unchanged.
- Fidelity: the mockup's Activity screen and ⌘K palette were rendered from
  the prototype source and measured against the real screens on a private
  disposable stack (the mockup's six events routed into the API response for
  geometry only) at 1440×900 and 1920×1080. Heading, actor filter, card,
  rows, avatar marks, palette dialog, input row, Esc button, list, rows,
  glyph tiles and kind labels are within 2 px with identical leaf type. The
  only difference: the mockup's rows are 32 px wider than their card
  (content-box padding), so its via/when columns sit 24–33 px further right
  and its time column is clipped; the real rows fit the card. Before the
  fix the heading wrapped (a full-width Select) and the palette was centred
  vertically (the generic overlay rule won); both corrected.
- Real data (a fresh Tenant with a Space, an App and three renames): the
  renames read as one line ending "3 changes", which opens the App's trail
  (`resource_id=` sent) and "Show all activity" returns; Enter on **Agents**
  sends `actor=agent`. The palette lists "Go to …" from the navigation,
  "Open …" for loaded Apps and Spaces, Connect a new agent (opens the
  dialog) and the three "Activity · … only" filters; typing "agents only"
  and Enter opens `…/activity?actor=agent` with Agents pressed; the Esc
  button closes it; an unmatched query shows "Nothing matches.".
- axe-core (WCAG 2.0/2.1 A/AA + best practice) on Activity and on the open
  palette at 1440 and 390 px, light and dark: 0 violations, no horizontal
  overflow. A separate visual review of the screenshots found no
  high-severity difference; the shell's 1–3 px sidebar offsets belong to the
  shell, the kind Select (a real filter the mockup lacks) keeps the kit
  Select look.
- Content audit: no mockup content was copied. Omitted for lack of a
  backing operation or read: "Publish {entity}" (needs the entity draft
  flag, G3), "Role matrix for {entity}" and "Test {entity} as {user}" (no
  designer deep link to those tabs), the agent's preset where its grant is
  unreadable or matches no preset.

Executed checks: `go test ./...`; `TestAuditAttributionGolden` against the
local MariaDB; `make lint` 0 issues; console lint and typecheck;
`python3 design/validate_schema.py --check`; `python3
scripts/verify_driver.py`. The full browser suite ran in CI.

## Entity designer on the approved design — 18 November 2026

- `has_draft_changes` on `GET /v1/spaces/{space}/entities`: the MariaDB
  integration tests (`TestEntityRuntime`, `TestRegistry*`) assert it is true
  before the first publication, false after it, true after a PATCH that
  changes the definition and false again after republishing; the untyped-row
  registry comparison checks it separately.
- The Entities screen was measured against the prototype's `entities`
  screen rendered statically in Chrome (same shell head), region by region,
  at 1440×900 and 1920×1080, with a populated Space (Company, Contact, Deal;
  two new draft fields). Header tile, title, meta, chip and tabs, the entity
  list (212 px card, 35 px items, the selected item's 600 weight, background
  and 3 px bar), grid header/row (43 px rows, 12px 16px padding, 14 px type)
  and pipeline steps match within 1 px with identical type. Remaining
  differences are content or composition: text-length widths (title/meta,
  "Review & publish"), the Fields card's heading row with Add field and the
  row-actions column (from the full comp; they move the grid 61 px down and
  narrow the columns), the templates note under the list, and one-line step
  notes (63 px steps instead of 79 px). The done-step green is the shared
  `--ui-success` (#1a603d vs the mockup's #1f6b45).
- axe (WCAG 2.1 AA + best practice): 0 violations on Fields, Access, Rules,
  Test and History in light and dark mode at 1440 and on Fields/Access at
  390; no horizontal overflow at 390 px (the field grid scrolls inside its
  card). A text-only visual review found the composition close; its findings
  (a list item without a dot centred, choices/calculations not in body text,
  the list card not full width on a phone, a duplicate "Publications"
  landmark) were fixed.

## Console shell and Tenant Home on the approved design — 18 November 2026

- The shell (sidebar, top bar, centred content) and Home were measured
  against the mockup's interactive prototype rendered in Chrome, region by
  region (position, size, padding, radius, colors, font shorthand), at
  1440×900 and 1920×1080, and checked for horizontal overflow at 390×844.
  With the dev seed, every shell region and the header, stats, App card and
  activity components are within 1 px of the mockup with identical type;
  before the change the sidebar nav sat 55 px higher, the h1 was "Home" at
  26 px instead of the 34 px greeting, the stat cards spanned the content
  width (278×93) and at 1920 the content stretched to 1480 px instead of the
  1240 px column. With the mockup's own data patched into the API responses
  (verification only), the remaining differences are data-driven: two
  attention cards instead of three (wider, one fewer wrapped line) and
  text-length widths. The Spaces group sits 3 px higher than the mockup,
  whose Agents row is 36 px tall only because its "◉" placeholder glyph
  falls back to a taller font; real rows use 18 px icons.
- A separate visual review of the screenshots (mockup, real with mockup data,
  real with the seed, 390 px and dark) found layout, type scale, spacing and
  components faithful; its findings (dev Space dot colour, a red border on
  the failed card, a chevron on the account, the date comma, bold names in
  activity sentences, an orphaned breadcrumb separator at 390 px) were fixed.
- "Load more" in the Tenant switcher: the popup stayed open in headless
  Chrome, but focus fell to the page when the clicked button was disabled
  and removed, the status line switched to "Searching…" and the new options
  landed below the scrolled view, so the list looked unchanged and stopped
  answering the keyboard. The button now keeps focus in the search field
  (no mouse-down focus), shows "Loading…" while only the next page loads and
  brings the first new option into view. `tests/tenant-switcher.spec.ts`
  creates 52 Tenants on the e2e stack and asserts more than 50 options, the
  popup still open, focus in the search field, option 51 in view and Escape
  closing it.

- Owner review of the sidebar: Spaces carry no environment tag (there is no
  production environment locally); development workspaces are listed under
  their Space with a branch icon and "workspace"; every row keeps its status
  dot. The Tenant switcher is the sidebar section label for every account
  with a Tenant, even one, and `tests/tenant-switcher.spec.ts` reaches it by
  role and name.
- Content audit: the mockup sets layout and style only. The diff was searched
  for mockup content (names, releases, hostnames, step counts, environment
  tags, sample sentences); the real screens derive every value from API data
  or contracts, the Connect agent dialog names no particular client, and the
  stories use neutral sample data. Mockup elements without a data source are
  omitted: an "unpublished entity changes" attention card, the account's
  role label under the name and an agent's role in activity meta.

Executed checks:

- `go test -race` with `LASTAPP_TEST_CONFIG` over `./internal/...` (17
  packages) passes; `TestRegistryRecordsMatchUntypedRows` covers the audit
  trail's new `resource_name` and the overview's micro-app counts; the
  delivery shape golden gained `deployed_by` and `release_name` only.
- `make lint` 0 issues; console lint, typecheck, build and Storybook build
  pass.
- axe (WCAG 2.0/2.1 A/AA and best practice) over all 113 Storybook stories
  in light and dark at 1440 px, and the changed groups at 390 px: no
  violations beyond the page-level rules a component story cannot meet
  (`landmark-one-main`, `page-has-heading-one`, `region`, as in the earlier
  baseline) once App cards were named by their visible App name. Real Home,
  Activity and the Connect agent dialog at 1440 px (light): the same App-card
  finding, since fixed; no other violations.
- Platform screens at 390 px with a long Tenant name: no horizontal overflow
  after the current breadcrumb was allowed to shrink.
- e2e: on this machine every browser login in the Playwright runner took 4–8 s
  (the same steps outside the runner: under 1 s; the commit before this UI
  work failed its login wait identically), so local runs at stock timeouts
  fail at login. With a temporary 20 s expect timeout (not committed)
  `tests/tenant-switcher.spec.ts` passed both tests. The full suite ran in
  CI at stock timeouts (GitHub Actions `test` run 44325920770): green, all
  browser regressions passing, after the specs were moved to where the new
  shell puts Spaces, Tenant settings and Refresh.

## Trimmed Northstar development demo — 18 November 2026

- The seed no longer creates the mockup-named workspace `crm-pricing-exp` or
  the staged Deal `forecast_category` field that Forecasting's release used to
  promote. It publishes Account, Contact and Deal with their own fields only,
  captures release 1.0.0 of Deal review and Forecasting in a workspace named
  `demo-release`, deploys both, and then archives that workspace with
  `delivery.ChangeWorkspace` (the console's archive operation). The archived
  workspace stays listed with its Archived status; no active workspace remains.
- Local registry: backed up to
  `.local/backups/lastapp_foundation-20261118-2232-before-trim.sql.gz`, then the
  existing `crm-pricing-exp` workspace was archived through
  `POST /v1/workspaces/{id}/archive` as the Tenant administrator. The Deal's
  `forecast_category` field was already published to production by the earlier
  seed (no unpublished draft existed) and stays: the publisher rejects removing a
  published field until a destructive-change workflow exists.

Executed checks:

- `TestNorthstarDemoSeed` (updated): entities publish exactly the demo fields,
  the workspace is `demo-release`, archived and owned by the Tenant
  administrator, no workspace remains unarchived, one active selected
  deployment per App, and a second `SeedDevelopment` leaves the counts
  unchanged. `TestNorthstarDemoDefinitions` **PASS** against the local MariaDB.

## Populated Northstar development demo — 18 November 2026

- `seed-development` now also fills the Northstar demo through the real
  operations, as the Tenant administrator: CRM entities Account, Contact and
  Deal created and published (`entities.Authoring`), the development workspace
  `crm-pricing-exp` (`delivery.CreateWorkspace`), and for Deal review and
  Forecasting a static build stored through an upload ticket
  (`UploadTicket`/`AuthorizeUpload`/`StoreBuild`), release 1.0.0
  (`CreateRelease`) and a production deployment (`DeployRelease`). Forecasting's
  release promotes a new Deal field published in the workspace. No registry
  table of these resources is written directly.
- Queued work runs in the seed through the worker's own steps:
  `worker.Leader.Settle` takes the worker lock, runs the starters and drains the
  steps until a pass does no work, and defers (skipping the demo content with a
  warning) while a server's worker leads. Each piece is created only while it is
  missing; an App with any deployment is never shipped again.
- Not seeded: a failed deployment (no genuine check fails without faking state)
  and an agent grant (created only by the OAuth consent flow).

Executed checks:

- `TestNorthstarDemoSeed` (new): all three entities published with no pending
  version, the forecast field only on Deal, the workspace ready and owned by the
  Tenant administrator, exactly one active deployment per App that is the App's
  selected release; a second `SeedDevelopment` leaves the counts of audit
  events, entities, entity versions, workspaces, builds, releases, deployments,
  upload tickets and database resources unchanged. `TestNorthstarDemoDefinitions`
  validates the definitions. `TestSettleDrainsUntilIdle` and
  `TestSettleDefersAndReportsFailures` cover `Settle`. `TestSecurityRegressions`
  (which seeds with the provisioner) now shares the disposable-registry helper.
- `go build ./...`, `go test ./...`, `LASTAPP_SECURITY_CONFIG=… LASTAPP_TEST_CONFIG=…
  go test -race -count=1 ./internal/control ./internal/worker` **PASS**;
  `golangci-lint run ./...` (v2.15.0, 0 issues), `python3
  design/validate_schema.py --check`, `python3 scripts/verify_driver.py`
  **PASS**. No console change; `make e2e` and the development database were not
  run or reseeded.

## Entities, Access, Agents and Platform screens on the UI kit (U4b) — 18 November 2026

- Every console screen outside `src/ui` is now kit-only: the entity designer
  (list beside the open entity; Fields, Access, Rules, Test, History; permission
  paths as who + operation + fields + record condition; the publication
  pipeline), App data access, the App launch page, Space/App Access,
  People/Teams, agent connections and consent, Platform Tenants/Users/
  Settings and a Tenant's Overview/Administrators/Settings, account,
  appearance, sign-in and first-login password, Tenant settings, and the
  U4a remainder (Home, Releases, Development, the shell). A scan of the
  screens finds no raw buttons, inputs, headings or layout markup and no
  `className` other than e2e hooks on kit components. `entity.css` and
  `agent-access.css` are deleted; dead `style.css` rules were removed
  (2368 → 1213 lines). No Go change.
- New kit parts, each with stories: `DatePicker` (+ `days.ts`), `Columns`,
  `Section`/`Lead`, `RecordList`, `FieldGroup`, `UploadButton`,
  `LoadingScreen`, `AppShell`/`QuickSearch`/`AccountMenu`/`ContextLabel`,
  `AuthLayout`, `StandalonePage`; DataTable stacks rows as label/value cards
  below 640 px; fields take `hideLabel` and keep the required mark beside the
  label; RemoteSelect, NumberInput and NewPasswordInput carry `ui-input`.
- New confirmations: removing a Space/App access assignment, revoking an
  agent connection, removing a Tenant administrator, disabling an account.
- Findings fixed during verification: the required asterisk inside `<label>`
  made label lookups ("Email", "Entity name", "New password") fail (8 e2e
  failures on the first full run); a skipped heading level in the designer;
  `aria-required` on the date trigger button; two parallel fieldset parts
  merged into `FieldGroup`; FormModal's Advanced used the native triangle;
  form inputs and select triggers used a fainter border than kit inputs.

Executed checks:

- `npm run lint` (0 problems), `npm run typecheck`, `npm run build`, `npm run
  build-storybook` (111 stories; output deleted), `make lint` (0 issues),
  `git diff --check`, `python3 design/validate_schema.py --check`, `python3
  scripts/verify_driver.py` **PASS**.
- Static Storybook probe (every story at 1440×900 and 390×844, light and
  dark, 444 runs, axe-core WCAG 2.0/2.1 A/AA + best-practice without the
  story-frame landmark rules): **0 violations, no horizontal overflow, no
  page errors**.
- Temporary console probe on the e2e stack (not committed): 45 screens and
  states (signed out, password welcome, all Platform pages, account,
  appearance dialog, every Tenant page and tab, entity list, designer tabs
  before/after publishing, field editor and review dialogs, Access "Manage
  roles", team detail, App launch, agent consent and its open expiry picker,
  Tenant agent access and account with a grant) at 1440, 1920 and 390 px in
  light and dark: **270 states, 0 axe violations, no horizontal overflow, no
  page errors**; content is 1196 px wide at 1440 and capped at 1480 px at
  1920. Keyboard: designer tabs move with arrows, Enter opens the field
  editor and Escape returns focus to "Add field"; "Manage roles" activates
  with Enter; the expiry picker opens with Enter, arrows move days, Enter
  chooses (2026-11-25 → 2026-12-03) and returns focus to the trigger, Escape
  closes; a preset presses with Enter and ticks its permissions; Enter on
  "Approve agent access" returns to the client callback. One probe run hit a
  single transient first-screen wait on My account; the rerun passed.
- `make e2e` with the stock timeouts: first run **27/35** (the label finding
  above); after the fix the affected specs 17/17 and the full suite **35/35**
  (2.6 min), again **35/35** (2.7 min) after the last styling fixes. No
  config change; no assertion relaxed.
- Spec changes: `agent-access.spec` answers the new revoke confirmation and
  also checks the date trigger's height.
- A text-only review of the e2e screenshots led to the Advanced and border
  fixes; its other notes were full-page capture artifacts (sticky header and
  fixed sidebar in `fullPage` shots) or test-fixture data.

## Delivery screens, Activity and their server contracts on the UI kit — 18 November 2026

- Apps and Spaces lists, App/Space detail tabs (micro-apps, settings), Space →
  Development, App → Releases (ship flow, production deployment stepper,
  release history and review, uploaded builds) and Activity are rebuilt from
  kit parts only. New kit pieces, each with axe-clean stories in light and
  dark: `FactList`, `Disclosure`, `CodeBlock`, `PickList`,
  `SegmentedControl`, `LinkButton`, `Stack`/`Inline`/`Split`, and Collection's
  `header`, `params`/`filters`, `render` and `poll`.
- Server contracts: `GET /v1/config` publishes
  `development_policy.deployment_states` from `delivery.DeploymentSteps`/
  `DeploymentFailed` (`TestDeploymentStatesMatchSchema` keeps them equal to
  the `release_deployments` CHECK); audit events carry `actor_kind`, the trail
  accepts `?actor=` and `?kind=` before search and paging, and
  `GET …/audit-kinds` lists the kinds (`TestAuditAttributionGolden` asserts
  each filter, a combined filter, a rejected actor, the kinds list and a 403
  for a member without `audit.read`). `session_shapes` golden regenerated for
  the new config field; the other goldens are unchanged.
- Findings fixed during verification: the running deployment's description
  contained the word "active", so the deployment-status regression saw the
  state before it was reached (copy changed; the assertion is unchanged);
  `TestRegistryRecordsMatchUntypedRows` needed the new column; a visual review
  found legacy `.workspace-facts` styles overriding FactList, a wrapping
  badge in the release list, an over-wide kind filter, mid-word breaks in
  phone tables and "1 records" — all fixed.

Executed checks:

- `npm run lint`, `npm run typecheck`, `npm run build`, `npm run
  build-storybook` (output deleted; the 9 new stories axe-clean in light and
  dark), `make lint` (0 issues), `git diff --check`, `python3
  design/validate_schema.py --check`, `python3 scripts/verify_driver.py`
  **PASS**. `go test ./...` and, against the local MariaDB,
  `go test ./internal/control ./internal/agent ./internal/delivery` with
  `LASTAPP_TEST_CONFIG`/`LASTAPP_SECURITY_CONFIG` **PASS**.
- Temporary probe on the e2e stack (not committed): a fresh Tenant, Space,
  App and micro-app with deployment, release and workspace responses routed
  to fixed states; axe-core WCAG 2.0/2.1 A/AA + best-practice and
  `scrollWidth <= innerWidth` on Apps, Spaces, Space settings and
  development, App micro-apps and settings, Releases (publishing, failed,
  active) and Activity at 1440, 1920 and 390 px in light and dark: **50
  states, 0 violations, no horizontal overflow**; content is 1196 px wide at
  1440 and capped at 1480 px, centred, at 1920. Keyboard: the actor filter
  (Enter on **Agents** sends `actor=agent`), the kind Select (Enter, Arrow,
  Enter sends `kind=`) and opening a release with Enter.
- `make e2e` with the stock timeouts and no retries: the first full run was
  **34/35** (the deployment-status finding above); after the fix
  `development-releases.spec` passed 3/3 and the full suite **35/35**
  (2.7 min). No config change; no assertion relaxed.
- Spec changes: `review-fixes.spec` reads the compute preference as the
  definition paired with its term (was one text node);
  `workspace-regressions.spec` accepts "1 record" in the load-finished count.

## Console shell and Tenant Home on the UI kit — 18 November 2026

- The shell (sidebar, top bar, page header) is composed from kit parts. The
  top bar holds the Tenant and Space ContextSwitchers (the Tenant switcher
  searches and pages `/v1/my-tenants` through `useRemoteOptions`, shared with
  RemoteSelect), a ⌘K/Ctrl+K CommandPalette fed from the same
  `tenant/navigation.ts` definition as the sidebar and tabs plus cached Apps,
  Spaces and entities, and the account menu on the new kit `Menu`. Tenant
  users land on **Home**, composed only from existing authorized reads. The
  workspace, Tenant, platform-settings and entity-designer tab strips are the
  kit Tabs; FormModal confirms saves with a toast.
- Findings fixed during verification: the quick-search and account triggers
  were named by an `aria-label` that did not contain their visible text (axe
  `label-content-name-mismatch`; they are now named by their visible text
  plus visually hidden words); a long Tenant name in the single-membership top
  bar pushed the account menu 11 px past a 390 px viewport (found by
  `admin.spec`); the App-directory regression now opens Apps from Home, and
  the expired-session regression waits for the recovery instead of checking
  the sign-in screen once.

Executed checks:

- `npm run lint`, `npm run typecheck`, `npm run build`, `npm run build-storybook`
  (81 stories, output deleted), `make lint` (0 issues), `git diff --check`,
  `python3 design/validate_schema.py --check`, `python3
  scripts/verify_driver.py` **PASS**. No Go code or golden file changed.
- `make e2e` under a heavily loaded machine (load average 5–16; Time Machine
  and other processes): with the stock 5 s expect timeout most specs timed out
  waiting for the first screen after login, and an `origin/main` worktree
  measured the same login times (8–14 s) and post-load script time, so the
  timeouts are environmental. With a temporary, uncommitted config raising the
  test/expect timeouts to 180 s/20 s and no retries: **32/35 passed**; the 3
  failures are the findings above. After the fixes the full suite passed
  **35/35** with the same config and no retries (36.4 min, load average
  5.7–7.4). No assertion was relaxed in a committed spec.
- Temporary console probe on the e2e stack (axe-core WCAG 2.0/2.1 A/AA and
  best-practice, plus `scrollWidth <= innerWidth`) at 1440, 1920 and 390 px in
  light and dark: Platform Tenants and Users, and Tenant Home, Apps, Spaces, a
  Space's Entities, an App's Micro-apps, People, Activity and Settings — 60
  page audits, **0 violations, no horizontal overflow**. Keyboard flows:
  Ctrl+K opens the palette with the filter focused; typing an App name and
  Enter opens that App; "spaces" + Enter navigates; the Space switcher opens
  on ↓ with its filter focused, filters, and Enter switches Space with focus
  back on its trigger; the Tenant switcher lists 2 Tenants and Escape returns
  focus to it; the account menu opens on Enter with "My account" focused, ↓
  moves to "Appearance", Escape returns focus to the trigger; → on the Space
  tabs moves to the next tab; the collapsed sidebar (68 px rail) survives a
  reload. The open palette, switcher, menu and rail are axe-clean too.
- Static Storybook probe (every story at 1440×900 and 390×844, light and dark,
  324 runs, axe-core WCAG 2.0/2.1 A/AA and best-practice): no horizontal
  overflow, no page errors and no violations other than the page-level
  `landmark-one-main`/`page-has-heading-one`/`region` rules that do not apply
  to isolated stories.

## Console UI kit — 18 November 2026

- `web/admin/src/ui` is the complete kit: the shared controls moved there
  (one barrel, no re-exports at old paths) and 27 components were added
  (primitives, Dialog, toasts, form controls, Stepper/Timeline, sidebar parts,
  ContextSwitcher, CommandPalette, DataTable, RoleMatrix). Storybook builds 78
  stories. Light status text colors now differ in lightness (L* 27/36/45 for
  danger/success/warning, ≥ 4.5:1 on their soft background, the surface and the
  canvas); StatusBadge adds a different mark shape per tone.
- Swaps in the console: all 25 hand-written error banners render through
  Alert; FormModal, ConfirmDialog, the agent access editor and the entity
  designer's editors (formerly a native `<dialog>`) are Dialog wrappers;
  Collection draws through DataTable; the state badges use StatusBadge; the
  form, people, platform-admin and agent permission checkboxes are the kit
  Checkbox; ConsoleShell's navigation is SidebarNav with a remembered collapse
  (rail only on desktop); the Toaster is mounted at the root.
- The first full `make e2e` run failed 1/35: closing an entity editor no longer
  returned focus to "Edit Website", because the editor's autofocused field took
  focus before `useDialogFocus` recorded the opener (the native `<dialog>` had
  hidden that). The hook now falls back to the element focused at first render;
  the rerun passed.

Executed checks:

- `npm run lint`, `npm run typecheck`, `npm run build`, `npm run build-storybook`
  (output deleted), `make lint` (0 issues), `python3 design/validate_schema.py
  --check`, `python3 scripts/verify_driver.py`, `git diff --check` **PASS**.
- `make e2e` **35/35 passed** (load average 10–12) after the focus fix; the
  entity, confirm-dialog and appearance specs also passed 7/7 on their own.
- A temporary Playwright probe of the static Storybook build ran every story at
  1440×900 and 390×844 in light and dark (312 runs) with axe-core (WCAG 2.0/2.1
  A/AA and best-practice rules): no horizontal overflow, no page errors and no
  color-contrast violation. Two findings remain. The shell's account menu
  `summary` is named "Account menu" while it shows the user's name (axe
  `label-content-name-mismatch`, desktop only). That markup predates the kit and
  the e2e helpers select it by that exact label, so it is left for the shell
  rewrite. The other is the first-load `favicon.ico` 404 of the probe's static
  server.
- Keyboard probe on the stories: Tabs (←/→ select with wrap and disabled tabs
  skipped, Home/End, roving tabindex, manual activation on Enter); Dialog
  (focus inside, Tab/Shift+Tab trapped, Escape closes and returns focus, busy
  dialog ignores Escape); CommandPalette (Ctrl+K opens, filter, ↓ skips
  disabled, Enter runs and closes, Escape closes); Checkbox (Space toggles,
  17×17 px, parent indeterminate then checked); Switch (Space/Enter);
  RadioGroup (arrows select); ContextSwitcher (↓ opens with the filter
  focused, typing filters, Enter chooses, focus returns, Escape closes);
  sidebar (collapse → 68 px rail, persisted across reload, rail items named,
  tooltip on keyboard focus, expand).

## Signal theme, local Storybook and UI library foundations — 18 November 2026

- Signal ("Ink & Signal") replaces the Neo, Studio and Evergreen presets as the
  only console theme, in light, dark and system modes. `themes.css` keeps the
  `--ui-*` names and adds heading/mono fonts, card radius, focus, sidebar
  hover/muted/border/surface tokens and soft status utilities. A stored
  `{"theme":"evergreen","mode":"dark"}` keeps dark mode and is rewritten as
  `{"mode":…}` on the next change (new e2e scenario).
- The production build first emitted small Fontsource subsets as `data:` URIs,
  which the console CSP blocked (Chrome console error in the e2e traces). Vite
  now always emits fonts as files; the built CSS has no `data:font`.
- Storybook 10.6.5 (local only) builds 22 stories. A Playwright probe of the
  static build at 1440×900 and 390×844, light and dark: no page errors, no
  horizontal overflow, no story text under 4.5:1; Select keyboard choice,
  RemoteSelect paging (4 → 9 active members, search, choose), Collection
  paging/empty/error, NewPasswordInput validity following the supplied policy
  (12–128 accepts an 18-character password, 20–64 rejects it), FormModal error
  and 390 px fit, generated password length 40, danger ConfirmDialog focusing
  Cancel. The Colors story found the light warning mark at 2.2:1; the marks now
  meet 3:1 with distinct lightness (light L* 37/52/59, dark 52/68/75 for
  danger/success/warning).
- A temporary, uncommitted Playwright probe on the disposable e2e stack (Tenants page and Create Tenant
  form, both modes, 1440 and 390): ink sidebar `rgb(14, 15, 18)`, white/
  `rgb(27, 28, 32)` cards with 14 px radius, Inter Tight headings, no overflow.
  It found the sidebar brand inheriting near-black text in light mode (1.02:1);
  `.sidebar` now restates its text color, and the rerun reports no text under
  4.5:1.

Executed checks:

- `npm run lint`, `npm run typecheck`, `npm run build`, `npm run build-storybook`
  (output deleted), `make lint` (0 issues), `python3 design/validate_schema.py
  --check`, `python3 scripts/verify_driver.py`, `git diff --check` **PASS**.
- `make e2e` **not green on this machine**, and not attributable to this change.
  The appearance spec passed 5/5 on a lightly loaded run (before the
  sidebar-color fix). Two later full runs, with load averages of 10–37, failed
  30/35 the same way: an element (mostly the post-login `Tenants` heading) did
  not appear within the 5 s expect timeout. The traces show slow server
  responses, for example 1.8 s for `/v1/config`. The same subset
  (`confirm-dialog.spec.ts`, `console-data.spec.ts`) fails 6/6 identically on
  `origin/main` (6edc683) under the same load. The suite needs a rerun on an
  idle machine before merge.

## Package-split findings: directory-scoped share search, agent reads, small DRY fixes — 17 November 2026

- Share-recipient search (`GET .../shares?view=options`) listed every active
  Tenant member (matching emails) and every team to any record manager. It now
  shares `identityOptions` with reference options: without live
  `membership.read`/`team.read` the caller finds only themselves and their own
  teams. `TestDataShareRecipientDirectory` **PASS** (a manager without directory
  access finds nothing by another member's email; after a live Tenant
  administrator grant the same App session finds them).
- Audit-trail and user-list reads now use `Runner.Authorize`. Grant scope and
  revocation were already enforced through `CheckAgent`. An explicit agent
  `op.Actor` on a context without a call skipped the tool-permission check.
  `TestAgentAuditAndUserReadsNarrowed` **FAIL** on the previous code ("audit
  read ignored the actor's agent call") and **PASS** now.
- `OpenGate` rejects a layout-hash rewrite of the version the same attempt
  already opened (409). `TestOpenGateFence` was extended and **PASS**es.
  `TestOwnershipMarkerFormat` pins the storage marker format.
- Goldens are unchanged (`internal/control/testdata` clean; no
  `-update-golden`). The `data_shapes` share-options case runs as a Tenant
  administrator.

Executed checks:

- `make lint` **PASS**.
- `go test -race -count=1 ./...` with `LASTAPP_TEST_CONFIG` and
  `LASTAPP_SECURITY_CONFIG` **PASS** for every package except two:
  - `TestMariaDBControlPlane` **FAIL**: "database status timeout" waiting for
    real provisioning. It needs the running development server's worker. The
    server was intentionally stopped, so this was not executed meaningfully.
  - `internal/session` **FAIL** because of a reordering in `ChangePassword`,
    which was then reverted; `internal/session` **PASS** under `-race` after
    that.
- `go test -count=2 -run Golden ./internal/control` **PASS**.
- The `make mcp-test` command, run with the main checkout's config path,
  **PASS**.
- `make e2e` **PASS** (34 tests, after the race suite).
- `python3 design/validate_schema.py --check` and
  `python3 scripts/verify_driver.py` **PASS**.

## Worker follow-ups: independent workspaces, own registry pool — 17 November 2026

- Workspaces within a Tenant are independent: `WorkspaceJob` tries the
  Tenant's preparing workspaces oldest first and stops at the first that
  progresses; one waiting for storage/publications or erroring no longer holds
  back the others. No order is needed between workspaces (own backing Space and
  storage, per-workspace state machine). Deployments keep strict per-Tenant
  order (base-hash check and shared Space configuration).
- The worker has its own registry pool, capped at 1 + 2 × steps (11): the
  leader lock and every service its steps call (second delivery service and
  publisher via `deliveryServices`, the provisioner) use it; the request pool
  (16) is never touched by background work. `Step.Run` now receives the pool
  instead of a pinned per-run connection, so a step waiting on a target or a
  pause holds no connection. Inside the worker jobs' transactions the only
  extra pool read is `Publisher.BusinessDB`'s resource lookup (checked:
  `QueueDefinition`, `candidates`, `projectPermissions`, `EnsureReleaseBuild`
  use the transaction or run outside one), hence two per step.
- Regression tests, run against the previous branch head with the same test
  file: `TestWorkerWorkspacesIndependent` on `4b5b764` **FAIL** ("newer
  workspace blocked behind the Tenant's waiting workspace", 15 s);
  `TestWorkerOwnPool` (request pool capped and fully checked out by the test)
  on `7c01127` **FAIL** ("worker stalled while the request pool was fully in
  use", 15 s). Both now **PASS** (< 1 s).
- Goldens unchanged (`internal/control/testdata` clean; no `-update-golden`).

Executed checks: `make lint` **PASS** (after a gofumpt fix to two test
helpers); `go test -race -count=1 ./...` with `LASTAPP_TEST_CONFIG` and
`LASTAPP_SECURITY_CONFIG` **PASS** (control 249.0 s, worker 8.8 s), then
`internal/worker` and the `TestWorker*` control tests again under `-race` after
the formatting fix **PASS**; `go test -count=2 -run Golden ./internal/control`
**PASS**; `make mcp-test` **PASS**; `make e2e` **PASS** (34 tests, after the
race suite); `python3 design/validate_schema.py --check`,
`python3 scripts/verify_driver.py` and `git diff --check` **PASS**.

## Non-blocking worker: Tenant fairness, step isolation, concurrent steps — 17 November 2026

Intended behaviour change (one leader, one `GET_LOCK`, recovery at term start
unchanged):

- Tenant fairness: `DeploymentJob`/`WorkspaceJob` take the oldest unfinished job
  of the first Tenant after a cursor and report whether it progressed;
  `worker.Fair` round-robins Tenants and skips a Tenant whose head job waits
  (deployment on publications, workspace on storage or publications), so a
  stalled Tenant no longer idles the whole queue. Within a Tenant, jobs keep
  strict creation order (`created_at`, then `id`).
- Error isolation: a step error is logged as `worker step failed` with the step
  name and backs off 1 s doubling to 60 s; other steps keep running and
  leadership is kept. Leadership ends only when the lock connection breaks or
  the lock is lost (1 s heartbeat) or on shutdown (`worker leader disconnected`).
  Deadlock 1213 / lock wait timeout 1205 (`op.Transient`) is retried after
  100 ms and no longer fails a deployment, workspace or publication.
- No cross-kind blocking: each step runs in its own goroutine with a pooled
  registry connection per run (never the lock connection); the term cancels and
  waits for all of them before releasing the lock. Publication now reports busy
  when it ran a job, so queued publications run back to back.
- Goldens unchanged (`git status internal/control/testdata` clean after the
  golden runs; no `-update-golden`).

Regression tests and their result on the P4 code (`0e6aefb`, test files copied
into a detached worktree unchanged):

- (a) `TestWorkerTenantFairness` (`internal/control`, real leader and steps):
  P4 **FAIL** — "Tenant B blocked behind Tenant A: deployment queued, workspace
  preparing" after 15 s; now **PASS** (~1 s). `TestFairSkipsWaitingTenants`
  (database-free) covers pass order, the all-waiting pass and a failing Tenant.
- (b) `TestLeaderIsolatesStepErrors`: P4 **FAIL** (lock released after the step
  error); now **PASS**.
- (c) `TestLeaderSlowStepDoesNotDelayOthers`: P4 **FAIL** ("fast step ran 0
  times while the slow step held its run"); now **PASS**.
- (d, e) `TestLeaderTermLifecycle` (kill the lock connection while a step is
  blocked; recovery once per term before steps; no term-1 run after term 2's
  recovery; nothing runs after stop): P4 **FAIL** (the blocked step never
  yields, so the sequential leader never completes a round); now **PASS** under
  `-race`. The existing control test that kills the leader connection during
  provisioning still recovers the resource to `ready`.

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 219.3 s, worker 8.8 s); `go test -count=2 -run Golden ./internal/control`
**PASS**; `make mcp-test` **PASS**; `make e2e` **PASS** (34 tests, run after the
race suite); `python3 design/validate_schema.py --check`,
`python3 scripts/verify_driver.py` and `git diff --check` **PASS**.

## Worker leader and steps extracted from control (P4) — 17 November 2026

- Pure move, no behaviour change: the background leader is `worker.Leader`
  (`internal/worker`). It holds `GET_LOCK(lastapp:<registry>,0)` on one pinned
  registry connection, runs its `Step`s in order on that connection, pauses a
  second when no step found work, and on any step error ends the term (lock
  released through `data.ReleasePinnedLock`, `provisioning leader disconnected`
  logged) and retries a second later. The step list is built once in `Open`
  in the previous order: `Cleanup`, deployments, workspaces, `Publication`,
  `databases.Provisioner`. `Server.RunWorker` is a one-line wrapper.
- Per-term state (cleanup timestamp, publication and provisioning Tenant
  cursors) moved from loop locals into the steps; an optional `Starter.Start`
  resets it on each new term, and the provisioning step's `Start` runs the
  former provisioning→pending reset. Only provisioning reports busy, so the
  idle pause is unchanged. A process-local mutex serializes terms so step state
  is never shared between two.
- Provisioning (`provision`, `nextJob`, `validateRuntime`, `checkGrants`,
  `provisionError`, the generated-name patterns) moved to
  `internal/databases/provisioner.go` with the target connector injected from
  control. A line-by-line diff against `ac4d569:internal/control/worker.go`
  (indentation stripped) shows only receivers, exported names, `return busy, e`
  forms, the injected `Warn`/`Connect`/`Access`/`Sealer` and doc comments: both
  transactions are still `READ COMMITTED` on the leader connection, the lock
  order is unchanged (`tenants … FOR UPDATE`, then `database_resources`; the
  per-resource `GET_LOCK` on the target), the agent recheck still runs before
  the claim, and audits still carry `op.AgentAuditContext`.
- Cleanup: agent token-session and OAuth-request deletes and `CleanupClients`
  (with `UnapprovedClientLifetime`/`ClientIdleLifetime`) moved to
  `internal/access/cleanup.go`; agent registration and OAuth use them from
  there, with no copy left in agent. Console-session and upload-ticket deletes
  stay in the worker's `Cleanup` step, in the same order.
- Layering: `worker` is layer 100 below control (now 110) and is scanned by
  `TestNoHTTPLoopback`; it imports only `access`, `data` and `registry`.
- New tests: `internal/worker` (lock-only fixture: start/step order, steps on the
  connection holding the lock, standing by while the lock is held, idle pause,
  leadership loss and a new term after a step error; adapters database-free);
  `internal/databases` (provisioning error mapping, generated names).
- `internal/control` production code: 2,709 → 2,337 lines.

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 263.8 s, worker 4.9 s); goldens with `-count=2` **PASS** and
`git diff ac4d569 -- internal/control/testdata` is empty; `make mcp-test`
**PASS** (includes `TestAgentRetry*` and `TestAgentDeniedDatabaseJobAudit`);
`make e2e` **PASS** (34 browser tests; a first run made concurrently with the
race suite timed out and was discarded); `python3 design/validate_schema.py --check`,
`python3 scripts/verify_driver.py` and `git diff --check` **PASS**.

## Agent OAuth and management MCP extracted from control (P6) — 17 November 2026

- Pure move, no behaviour change: the OAuth authorization server for MCP
  clients (discovery, registration/retention, consent, code/refresh exchange,
  revocation), the console agent-access endpoints, bearer authentication,
  admission slots/limiters and the MCP server with its tool catalog and typed
  adapters are `agent.Server` (`internal/agent`), wired in `Open` with the
  registry DB, `access.Checker`, config, the domain services, the shared
  `Limiter` and trusted proxies. Control mounts it with
  `Routes(mux, console)`; route paths are unchanged and consent receives the
  console-session wrapper instead of importing session. `agent_tools.go` was
  split mechanically by domain (`tools.go`, `mcp.go`, `writes.go`,
  `context.go`, `resources.go`, `entities.go`, `access.go`, `databases.go`,
  `development.go`); a declaration-by-declaration comparison against the old
  files (modulo renames) found only the route function and `mcpURL` changed.
- Tool input types, `Tools`, `InputSchema`, `Instructions` and the handlers the
  control integration tests drive are exported (types do not appear in the
  generated schemas; `mcp_tools` is unchanged). Control tests call tools
  through `Server.Tools`.
- Prep: the account list and audit-trail reads moved from control into
  `inventory.Inventory.Users` and `roles.Authority.Audits` (same SQL and
  authorization calls) so MCP reads need no control import.
- `registry.PKCEMatches` is now the single S256 check of agent OAuth code
  exchange and App launch exchange; both previously computed
  `BASE64URL(SHA256(verifier))` inline with a constant-time compare. Fixed
  vectors pin it against both former implementations.
- Layering: `agent` is layer 99 and a transport package; agent, session and
  dataapi are siblings that only control may import (new check in
  `TestPackageLayering`). `TestNoHTTPLoopback` scans agent. `make mcp-test`
  now also runs `./internal/agent`.
- New tests in `internal/agent` (database-free): tool table invariants and
  visibility, permission catalog and scope normalization, configured-client
  validation, plus the moved schema/redirect/SSRF, pre-database admission and
  response-bound tests.
- `internal/control` production code: 5,485 → 2,709 lines
  (`internal/agent` production: 3,159 with doc comments and the split).

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 186.2 s); goldens with `-count=2` **PASS** and
`git diff edde6f5 -- internal/control/testdata` is empty; `make mcp-test`
**PASS** (agent and control); `make e2e` **PASS** (34 browser tests, including agent access screens);
`python3 design/validate_schema.py --check`, `python3 scripts/verify_driver.py`
and `git diff --check` **PASS**.

## Console sessions and login extracted from control (P5) — 17 November 2026

- Pure move, no behaviour change: password login, OIDC login/callback with the
  sealed flow cookie, session issue and cookies, principal resolution, CSRF and
  Origin checks, logout and password change are `session.Manager`
  (`internal/session`), wired in `Open` with the registry DB, config, `Sealer`,
  `membership.Passwords` and the trusted proxies; it owns the login limiter.
  Route registration, `/v1/config`, `POST /admin/v1/users` (now in
  `tenants.go`) and the trusted platform bootstrap (`bootstrap.go`) stay in
  control; `registryTransport` stays in `transport.go`.
- Account preparation moved to membership, its true owner (policy, verifiers,
  `LocalAccount`): `membership.Passwords` is the single PBKDF2 work pool, and
  `Passwords.PrepareAccount` serves platform user creation, Tenant creation,
  memberships, Tenant administrators and bootstrap. `Members` and `Inventory`
  take the pool instead of an injected `PrepareAccount` function; session uses
  the same pool for login and password change.
- Small DRY consolidations inside the moved code, each equivalent: the OIDC
  callback issues its session through the same helper as password login (the
  `auth_method` column already defaulted to `oidc`) and sets cookies through
  the same helper; the two bootstrap paths share the platform-grant block; the
  console cookie names are `httpapi.SessionCookie`/`CSRFCookie` (also used by
  the data API console launch and `/v1/config`).
- Layering: `session` is layer 98 (directly below control) and a transport
  package, so only control may import it. Agent OAuth consent gets the console
  principal through `httpapi.Session` wired by control, so P6 can extract agent
  OAuth without importing session. `TestNoHTTPLoopback` scans session.
- New tests: `internal/session` (database-free: cookie attributes, CSRF/Origin,
  login limiter budgets, shared work bound, password policy on change, OIDC
  state/expiry/nonce against a stub token endpoint and key set) and
  `internal/membership` (account preparation, work bound moved from control).
- `internal/control` production code: 5,959 → 5,485 lines
  (`internal/session` production: 530).

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 192.8 s); `go test -race -count=1 ./internal/session
./internal/membership` **PASS**; goldens with `-count=2` **PASS** and
`git diff a1e06bb -- internal/control/testdata` is empty; `make e2e` **PASS**
(34 browser tests, including login, password policy and expired sessions);
`python3 design/validate_schema.py --check`, `python3 scripts/verify_driver.py`
and `git diff --check` **PASS**.

## Workspaces, releases and deployments extracted from control (P3) — 17 November 2026

- Pure move, no behaviour change: workspaces and snapshots, workspace
  preparation, build upload tickets and storage, releases, preview, deployment
  queueing and the deployment steps, starters, development launch codes and the
  development policy are `delivery.Service` (`internal/delivery`). `Open` wires
  it with the registry DB, `op.Runner`, `access.Checker`, config,
  `databases.Storage`, `publisher.Publisher`, the registry name and the shared
  warning logger. Console handlers, the upload endpoint (rate limit, slots,
  bearer/origin checks, body decoding), `serveAppHost` and the worker leader loop
  (`GET_LOCK`, step order) stay in control and call the same operations as the
  MCP tools; the upload was split into `AuthorizeUpload` (before the body is
  read) and `StoreBuild`, and static serving into `HostedApp`, `ReleaseEntry`
  and `HostedFile`, preserving the original check order.
- Publisher cycle: the publisher stays below delivery and receives
  `delivery.Service.PublishingDeploymentAuthority` and
  `delivery.DeploymentEntities`; delivery then receives the publisher. Entity
  authoring's `Queue` is `delivery.Service.QueueDefinition`.
- `LaunchLifetime` moved to `registry.LaunchLifetime` (shared by dataapi console
  launches and delivery development launches).
- Layering: `delivery` is layer 96 (below dataapi, above publisher), not a
  transport package; it imports neither `net/http` nor `httpapi`, and
  `TestNoHTTPLoopback` now scans it as an operation package.
- `internal/delivery` has its own tests: database-free (snapshot remapping,
  contract hashes, deployment plans, pinned versions, starter pins, policy,
  hosted origins, artifact directory) and a disposable registry built from the
  migrations (deployment candidate entities and publishing recheck state,
  production App quota, capture readiness). HTTP/MCP flows and goldens stay in
  control.
- `internal/control` production code: 8,019 → 5,959 lines
  (`internal/delivery` production: 2,194).

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 193.3 s, delivery 5.0 s); the six goldens with `-count=2` **PASS** and
`internal/control/testdata` is unchanged since the P2 commit; targeted
`TestDevelopmentWorkspaceReleaseFlow`, `TestRelease*` (including the concurrent
preview test) and `TestAgentDevelopmentIsolation` **PASS**;
`python3 design/validate_schema.py --check`, `python3 scripts/verify_driver.py`
and `git diff --check` **PASS**; `make e2e` **PASS** (34 browser tests on the
disposable stack).

## App data API extracted from control (P2) — 17 November 2026

- Pure move, no behaviour change: `/data/v1` dispatch, actor/definition
  loading, record reads and mutations, capabilities, reference options, record
  shares, the definition cache, Tenant fairness and App sessions (console
  launch and launch info, PKCE exchange, bearer sessions and the shared
  browser/delegated parent check) are `dataapi.Runtime` (`internal/dataapi`).
  `Open` wires it with the registry DB, `access.Checker`, `entities.Authoring`,
  `publisher.Publisher`, the `Sealer`, config and the console's shared
  `Limiter` and trusted proxies; control keeps one `/data/v1/` mount and the two
  console launch routes. Development launch codes (`Server.Launch`) stay in
  control and use `dataapi.LaunchLifetime`. Inside the package the former
  `entity_data.go` is split mechanically into `runtime.go`, `records.go` and the
  SQL builders in `sql.go`.
- Token hashing and the PKCE verifier/challenge formats moved to
  `registry.TokenHash`, `registry.PKCEVerifier` and `registry.PKCEChallenge`
  (one source for control and dataapi) instead of being copied.
- Layering: `dataapi` is layer 97 (between control and publisher) and a
  transport package, so it may import `httpapi` (its decode/send, error writer,
  limiter and client IP) and only control imports it. `TestNoHTTPLoopback`
  scans all dataapi files for the removed loopback and keeps its non-HTTP files
  (`sql.go`, `cache.go`, `fairness.go`) free of `net/http`.
- `internal/dataapi` has database-free tests (cursor, share and team-recipient
  SQL, App gates, definition cache LRU and hash check, Tenant fairness, the
  latter moved from control); the `/data/v1` HTTP tests and goldens stay in
  control.
- `internal/control` production code: 9,545 → 8,019 lines
  (`internal/dataapi` production: 1,574).

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 189.1 s; only the CI-only `TestPrepareFixture` skips); the six goldens
with `-count=2` **PASS** and `internal/control/testdata` is unchanged since the
P1 commit; `python3 design/validate_schema.py --check`,
`python3 scripts/verify_driver.py` and `git diff --check` **PASS**; `make e2e`
**PASS** (34 browser tests on the disposable stack).

## Entity publisher extracted from control (P1) — 17 November 2026

- Pure move, no behaviour change: the publication job, `publish`, `Activate`,
  `RestoreBase`, `OpenGate`, permission projection, storage preparation and
  reference/column checks, publication candidates, `BusinessDB`/`RuntimeDB`
  and the runtime pool cache are `publisher.Publisher` (`internal/publisher`).
  `Open` wires it; control supplies the deployment authority recheck (same
  `publishing` deployment query and `deploymentPrincipal`), the deployment
  snapshot entities, the database connector and the warning logger. The worker
  loop, its `GET_LOCK`, job order, READ COMMITTED tenant lock, receipts, audit
  attribution and the physical version fence are unchanged.
- Shared helpers moved below control instead of being copied:
  `data.ReleasePinnedLock`, `databases.OwnershipConflict`,
  `entities.BusinessRecord`.
- `internal/publisher` is in the layering table (layer 95, below control) and
  in `TestNoHTTPLoopback`; it has its own disposable business-database fixture
  and tests (support-table ownership, the attempt fence of the storage gate,
  reference constraints, generated column checks). HTTP/MCP tests stay in
  control.
- `internal/control` production code: 10,265 → 9,545 lines
  (`internal/publisher/publisher.go`: 735).

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 190.0 s, publisher 2.6 s); the six goldens with `-count=2` **PASS** and
`internal/control/testdata` is unchanged since the P0b commit;
`python3 design/validate_schema.py --check` and `python3 scripts/verify_driver.py`
**PASS**; `make e2e` **PASS** (34 browser tests on the disposable stack).

## Shared-helper extraction before the control split (P0b) — 17 November 2026

- Pure moves, no behaviour change: `internal/httpapi` now holds the HTTP
  plumbing every surface shares (decode/send/reply, `WriteError` over
  `op.Classify`, the console `Session` wrapper, page shims, request logging and
  `Diagnostic`, `Limiter`, trusted-proxy `ClientIP`, `AllowedOrigin`,
  `CookieName`); `internal/sealer` holds credential sealing as a `Sealer` built
  in `Open` from the same key and key ID (same envelope; a fixed vector sealed by
  the previous code is decrypted in `TestDecryptsFixedVector`); the App row read
  is `inventory.AppRow` (also used by `entities`); the entity actor context is
  `entities.Authoring.ActorContext`, removing its injected func. Unit tests of
  the moved helpers moved with them.
- `TestPackageLayering` (`internal/layering_test.go`) runs `go list` and fails
  on any import against the documented direction, naming the import; checked to
  fail on a deliberately added upward import.
- Console: `Item` is `Record<string, unknown>` with narrowing at use sites and
  named release/agent-access shapes; the last inline ESLint disable is gone.
  Narrowing differs only for values the backend never sends (documented in the
  commit review: e.g. a missing `created_at` or a string revision).
- `internal/control` production code: 10,825 → 10,265 lines.

Executed checks: `make lint` **PASS**; `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config **PASS**
(control 196.6 s); the six goldens with `-count=2` **PASS** and
`internal/control/testdata` is unchanged since the P0a commit; `npm run lint`,
`npm run typecheck`, `npm run build` in `web/admin` **PASS**; `make e2e`
**PASS** (34 browser tests on the disposable stack).

## Behaviour-pinning goldens before the control split (P0a) — 17 November 2026

- Tests only; no production code changed. The response-shape recorder gained
  `raw` requests (bearer, form, cookie, binary) that also record normalised
  headers: cookie names and attributes (never values), redirect path and query
  keys, `WWW-Authenticate` parameter names, `Cache-Control`, `Retry-After`,
  media type and CORS presence. OAuth `error`/`error_description` strings and
  UUIDs inside recorded messages are normalised like console error messages.
- New goldens: `delivery_shapes` (75 entries: console workspaces, starters,
  development launch, upload tickets/uploads, builds, releases, preview, deploy,
  deployment status after the synchronous worker, and the MCP workspace/release
  tools with stale, missing-scope, sibling-grant and not-found failures),
  `data_shapes` (49: PKCE exchange and replay, CORS preflight, schema, records,
  bulk, query, capabilities, reference options, shares/share options for an
  owner and a denied member), `session_shapes` (54: config, sessions, CSRF and
  origin failures, password login/change/logout, rate limiting, OIDC
  login/callback errors against a stub token endpoint, agent OAuth discovery,
  registration, authorize, consent, token/refresh/replay, revoke, grant update)
  and `audit_attribution` (12 rows in 10 phases: console, MCP with request-key
  replays and a denied call, and publication-worker rows with agent attribution).
- `businessDatabaseFixture` (real registered database with runtime and schema
  accounts, dropped on cleanup) is now shared by `TestEntityRuntime` and the
  new goldens. `response_shapes` and `mcp_tools` are byte-identical.

Executed checks: the new goldens with `LASTAPP_TEST_CONFIG` and `-count=2`
**PASS** (deterministic); `go test -race -count=1 ./...` with
`LASTAPP_TEST_CONFIG` and `LASTAPP_SECURITY_CONFIG` on the local config
**PASS** (control package 201.3 s); `make lint` **PASS**. `make e2e` was not
run (no browser-facing change).

## Package split — 16 November 2026

- `internal/control` is split by mechanical moves (one commit per package,
  type-checked rewrite of references; names exported where needed, otherwise
  unchanged): `internal/registry` (Queryer, Row, APIError, typed readers,
  plans/policy/quotas/locks, workspace and App records), `internal/access`
  (`Checker` with authorize/scopes/member, delegated grant checks, production
  and workspace guards; Principal, AgentCall, grants), `internal/op` (Actor,
  Ref, Error/Classify, Runner, audit, receipts, paging, request deadlines),
  `internal/config`, and the domains `databases`, `roles`, `membership`,
  `inventory`, `entities`. Control non-test Go 15,951 → 10,825 lines.
- Two preparatory non-move commits: the authorization methods got their own
  `Checker` receiver (embedded in `Server`; it held only the MCP audience) and
  the runner holds that `Checker`. Six local variables were renamed where a new
  package name would be shadowed. Console request parsing moved back to control.
- Workspaces/releases/deployments/starters, the MCP adapters (`agent_*`) and
  `/data/v1` with App sessions stay in control: they are `Server` methods sharing
  transport state (slots, limiters, cookies, encryption key, pools, mux).
- `TestNoHTTPLoopback` also scans the operation packages and fails if one
  imports `net/http` or `internal/control`. Both golden files are unchanged.

Executed checks after each commit: `go build ./...`, `go vet ./...` **PASS**;
golangci-lint v2.15.0 **0 issues**; schema `--check` and driver verification
(42 files) **PASS**. `go test -race -count=1 ./...` with the local config
**PASS** after the op, inventory and final commits (control package 177.2 s,
68 top-level tests); the `make mcp-test` command **PASS** (37 tests/subtests);
`make e2e` (`scripts/e2e.py`, disposable MariaDB) **PASS**, 34 Playwright tests.
`git diff --check` clean.

## Typed registry review follow-ups — 16 November 2026

- `TestAccountDeadlineRenewsOnlyAfterDerivation`: choosing an existing account
  keeps the request deadline; only a derived password renews it
  (`preparedAccount.deadline`, used by membership, administrator and Tenant
  creation).
- `TestEntityRuntime` now makes the publisher's workspace read fail with a
  database error: the job returns the error and the version stays `queued`.
  With the previous "any error means no workspace" code it fails.
- Rebased on the updated `refactor/operations`. `go build`, `go vet`,
  golangci-lint v2.15.0 (0 issues), `validate_schema.py --check`,
  `verify_driver.py` passed; `go test -race -count=1 ./internal/...` on the
  development MariaDB: 5 packages, 82 top-level tests passed, 1 skipped
  (`TestPrepareFixture`); golden snapshots unchanged. `make e2e`: **34 of 34**
  Playwright scenarios passed in 2.6 minutes; no e2e listener remained.

## Typed registry reads — 16 November 2026

- The PBKDF2 key derivation honours the request deadline (separate commit).
- Registry reads scan into typed structs domain by domain (plans/policy,
  micro-apps, inventory, memberships/teams/users, roles/assignments, database
  targets/resources, entities and publications, runtime metadata, App records,
  workspaces, releases/deployments/builds, agent grants/OAuth, App sessions,
  audit trail/overview). Untyped `row()` calls 107 → 4, `rows()` 51 → 7,
  `pageQuery` 23 → 0 (kept only as the test reference); the remaining calls read
  dynamic business records and record ACLs.
- Both golden files stay byte-identical. `TestRegistryRecordsMatchUntypedRows`
  compares every converted read with the former `SELECT` scanned as `Row`;
  `TestRegistryRecordKeysAreSorted` checks that struct JSON keys stay in map
  order.

Executed checks after each commit: `go build ./...`, `go vet ./...` **PASS**;
golangci-lint **0 issues**; schema `--check` and driver verification (42 files)
**PASS**; `git diff --check` clean; `go test -race -count=1 ./internal/...`
with the local config **PASS** (only `TestPrepareFixture` skipped); `make
mcp-test` **PASS**. Browser e2e was not run: the shared dev stack on 8787 serves
another branch from the main checkout.

## Operation review follow-ups — 16 November 2026

- `TestAgentOperationResponseLimit` replaces the coverage removed with the MCP
  loopback: an over-limit read returns `response_limit` without data; an
  over-limit result after a committed mutation returns the compact `committed`
  + `resource` reply (an uncommitted one returns the error). Removing the
  `Committed` condition, or the committed branch, makes it fail.
- `TestActorIsTheAgentSource`: an Actor whose agent call differs from (or omits)
  the one already in its context fails `mutate`, `authorize` and `scopes` with
  `errActorAgentMismatch`; the context is no longer a fallback.
- Rebased on origin/main 87a8ecd. `go build`, `go vet`, golangci-lint v2.15.0
  (0 issues), `validate_schema.py --check`, `verify_driver.py` passed;
  `go test -race -count=1 ./internal/...` on the development MariaDB: 5
  packages, 78 top-level tests passed, 1 skipped (`TestPrepareFixture`, e2e
  only); golden snapshots unchanged.

## Operation extraction, steps 4–5 (loopback removed) — 16 November 2026

- Rebased onto `fix/go-minor` (runtime-grants subcommand); full race suite
  **PASS** before further changes (139 passing).
- Roles/assignments/recipients (`authority`), database targets/resources
  (`storage`), entity authoring, App contracts and App data access
  (`authoring`), and workspaces, releases, deployments, builds, starters, audit
  and user reads (`Server` operations) take an explicit `Actor`. Every MCP tool
  calls them directly. `executeManagement`, the recorded-response writer and the
  synthesized `If-Match` are deleted; `TestNoHTTPLoopback` (replacing the
  ratchet, which went 25 → 20 → 10 → 0) fails if they return.
- Both golden files stay byte-identical (`TestAgentToolSchemaGolden`,
  `TestResponseShapeGolden`).
- New direct operation tests as console and agent actors:
  `TestAuthorityOperations`, `TestStorageOperations`, `TestAuthoringOperations`
  (428/412 revisions, protected roles, retirement guard, target/TLS validation,
  denied agent scopes and targets, request-ID/grant audit attribution). Tests
  that inspected loopback-decoded values now decode the tool result as an MCP
  client does (`wire`).

Executed checks after each commit: `go build ./...`, `go vet ./...` **PASS**;
golangci-lint **0 issues**; schema `--check` and driver verification (42 files)
**PASS**; `git diff --check` clean; `go test -race -count=1 ./internal/...`
with the local config **PASS** (final run: 141 passing tests/subtests, control
package 153.2 s; only `TestPrepareFixture` skipped); the `make mcp-test` command
(`-run TestAgent`, race) **PASS** (36). Browser e2e for this branch was not run:
the shared dev stack on 8787 serves another branch from the main checkout.

## Operation extraction, steps 0–3 — 16 November 2026

- Golden contracts (`internal/control/testdata`) were generated against the
  unmodified code in a separate worktree: the 16 generated MCP tool schemas plus
  instructions, and the JSON key/type shape (with error codes and messages) of
  71 console and MCP responses, including receipts, replays, stale/missing
  revisions, missing delegated scopes and unsupported update fields. Every
  later commit leaves both files byte-identical (`TestAgentToolSchemaGolden`,
  `TestResponseShapeGolden`).
- `TestLoopbackRatchet` counts `executeManagement` references: 26 before, 25
  after (shared create/update/lifecycle/list/get call sites remain until their
  other resources migrate).
- Direct operation tests as console and agent actors: `TestInventoryOperations`,
  `TestTenantInventoryOperations`, `TestMembershipOperations` (428/412 revision
  rules, team revision on member add, denied scopes incl. `needs_authorization`
  scopes, receipt replay and conflicting request keys, request-ID and
  grant/client audit attribution, including audits written by not-yet-migrated
  helpers when the context does not carry the call).

Executed checks after each commit: `go build ./...`, `go vet ./...` **PASS**;
golangci-lint **0 issues** (web half of `make lint` not run: no `web/admin`
node_modules in the worktree, no console source changed); schema validator
`--check` and driver verification (42 files) **PASS**; `git diff --check`
clean; `LASTAPP_TEST_CONFIG`/`LASTAPP_SECURITY_CONFIG=.local/config.json go test -race -count=1 ./internal/...`
**PASS** (final run: 139 passing tests/subtests, control package 144.6 s; only
`TestPrepareFixture` skipped). Browser e2e (`make integration`) was not run: the
shared dev stack belongs to another session and no console source changed.

## Console data-layer tails and dependency audit — 16 November 2026

- RemoteSelect, Agent access/consent and the console config provider no
  longer fetch from effects. RemoteSelect uses an infinite query per debounced
  search (Load more appends a page; the AbortSignal cancels superseded and
  abandoned requests); the rest use `useApiQuery()`. Revoking agent access
  runs through `useAction()`.
- ESLint suppressions: 20 → 0, none added; `eslint-suppressions.json` removed.
  Hooks/constants moved out of component files (`console-config.ts`,
  `appearance.ts`); the release spec types the window client instead of `any`.
- The ErrorBoundary's "Try again" now resets inactive cached reads and retries
  failed lazy chunks (`retryable-lazy.ts`; React.lazy caches a rejection).
- `tests/console-data.spec.ts` adds two browser tests: a render failure
  (malformed Agent access response via route interception) shows the fallback,
  the shell navigates, and Try again recovers once the response is valid; an
  aborted designer chunk shows the fallback, returning to the tab keeps the
  failure without a new request, and Try again downloads it. Both tests failed
  as expected with the reset and the lazy retry temporarily disabled.

Executed checks in `web/admin`: `npm run lint`, `npm run typecheck`,
`npm run build` **PASS** at each commit. Browser run: `make e2e` (disposable
stack from `test/e2e-isolated-db`, merged into a throwaway branch only):
**34/34** passed.

`npm audit`: 7 moderate, all one advisory (GHSA-g3hv-75wg-m249, minimatch
`<11.0.1` ReDoS-style globparts amplification) through `typescript-eslint`
8.72.2 → `@typescript-eslint/typescript-estree` (`minimatch ^10.2.2`). Dev-only
lint tooling with repository-controlled globs; `npm audit --omit=dev` reports 0
and nothing reaches the console bundle. `npm audit fix` (no `--force`) changes
nothing: no fixed version is in range. `--force` would move `typescript-eslint`
to 8.36.0, outside the declared range, so it was not applied.

## Browser regressions on a disposable database — 16 November 2026

- `make e2e` (`scripts/e2e.py`) started a fresh tmpfs MariaDB (`mariadb-e2e`
  Compose service, profile `e2e`, port 33318), prepared it with `scripts/ci_config.py` and
  `TestPrepareFixture`, built the console and binaries, and ran the API on 8797,
  the development issuer on 8798 and Vite on 5183: **29 of 29** Playwright
  scenarios passed in 2.2 minutes, including `app-runtime.spec.ts` (the data App
  is now pointed at the e2e API origin). Afterwards no e2e container or listener
  remained.
- The development registry was not used: tenants with the e2e `ui-` slug stayed
  at 177 (newest created before the run) and `ui_*` schemas at 17. The total
  tenant count moved 240 → 242 only because of concurrently running Go
  integration tests (`test-`/`starter-` slugs).
- `npm run lint`, `npm run typecheck`, `go build ./...`, golangci-lint (0 issues)
  and `python3 -m py_compile` on the touched scripts passed. The CI workflow
  change (browser step via `scripts/e2e.py`) was not executed on GitHub here.

## Console data layer and component split — 16 November 2026

- `@tanstack/react-query` 5.107.1 (pinned) is the console read layer:
  `useApiQuery()` wraps the typed `api()` helper with the query AbortSignal.
  `useAction()` remains the only mutation mechanism and invalidates cached reads.
  FormModal, the entity designer and App releases now use it, so they all get
  the double-submit guard. Logging out or an expired session clears the cache.
- Polling: the Tenant workspace (5 s) and the designer publication poll
  (1.5 s, stops when the pending version clears) use `refetchInterval`. The
  deployment status poll keeps its 1.5-10 s backoff and stops at a final state.
- TenantWorkspace (1233 → 449 lines), PlatformWorkspace (767 → 199) and
  designer.tsx (1674 → 379) are split into page/section files. The markup was
  moved unchanged. The designer is a lazy chunk. An ErrorBoundary keyed by
  route wraps the routed pages of both consoles.
- Expression arguments and App data-access gates use stable generated keys
  (`useListKeys`). Validation diagnostics are read-only and keep index keys.
- ESLint suppressions: 40 → 20, none added.

Executed checks in `web/admin`: `npm run lint`, `npm run typecheck`,
`npm run build` **PASS** at each commit. The stack was rebuilt and restarted
with `scripts/dev.py` before each browser run. `npm run test:e2e`: **29/29**
passed after each data-layer and split commit; **32/32** with the new
`tests/console-data.spec.ts`. That spec checks:
- Tenant polling: 2-3 overview reads in 11 s, never overlapping; none while a
  form is open; polling resumes after the form closes.
- Expression argument identity after a removal. This assertion failed as
  expected when the index key was put back temporarily.
- No error and no horizontal overflow at 1280 and 390 px on 4 Platform and
  7 Tenant pages, including the lazy-loaded designer.

The ErrorBoundary and chunk-failure gaps noted here are covered by the
entry above.

## Minor Go/docs review fixes — 16 November 2026

- `CheckSchema` derives the required registry version from the newest embedded
  migration (`TestRequiredSchemaVersionFollowsMigrations` compares it with
  goose's own collection).
- `/v1/me` and `/v1/my-tenants` share one membership query; Go-side state
  comparisons and written values use named constants and one `enabledState`.
- Runtime UPDATE,DELETE grants cover every migrated table except the explicit
  append-only set, derived once in Go (`RuntimeGrantStatements`). The CI fixture
  applies them and checks the migration-derived table list against
  `information_schema`; `scripts/dev.py` applies the output of the new
  `lastapp runtime-grants` subcommand instead of its own Python table list.
  `TestRuntimeGrantStatements` **PASS**; the subcommand's 27 tables equal the
  removed Python list, and dev.py's grant step was dry-run against a scratch
  binary with a stub SQL runner (the dev stack was not restarted).
  `TestPrepareFixture` itself only runs in CI (opt-in disposable server) and was
  not executed locally.
- Flaky `TestTenantAdministratorTask` under `-race`: one 600k PBKDF2 derivation
  measured about 2.8 s with the race detector on an idle 10-core machine, and an
  earlier loaded run took 50 s for the test. The 15 s login deadline spanned
  that uncancellable derivation, so the session transaction could start with an
  expired context (500 `deadline_exceeded`). The deadline now bounds each
  database phase. `TestLoginDeadlineExcludesPasswordDerivation` sets the
  database deadline to half a measured derivation: it failed with the old
  structure (500) and passes now.
- `devoidc` logs through `slog`; the Makefile uses node/npm from PATH.

Executed checks: `go build ./...`, `go vet ./...` **PASS**; golangci-lint
**0 issues** (the web half of `make lint` was not run: no `web/admin`
node_modules in this worktree and no console source changed); schema validator
`--check` and driver verification (42 files) **PASS**;
`LASTAPP_TEST_CONFIG`/`LASTAPP_SECURITY_CONFIG=.local/config.json go test -race -count=1 ./internal/...`
**PASS** (control package 146.8 s; only `TestPrepareFixture` skipped).

## GitHub ruleset on main — 16 November 2026

- Repository ruleset `main` (id 27382055, active, created 16 November 2026)
  targets the default branch: pull request required (no approving review
  required), required status check `test` (the `Checks` workflow job) with the
  branch up to date with `main`, and force-push (`non_fast_forward`) and deletion
  blocked. Repository administrators keep a bypass. Read back with
  `gh api repos/getmarina/lastapp/rulesets/27382055`; PRs #2–#8 were merged
  after it took effect.

## Error wrapping and structured request logging — 16 November 2026

- Underlying errors are wrapped with `%w` (registry ping, cipher setup, schema
  check, envelope decryption, trusted proxies, OIDC metadata); cipher
  construction errors are no longer ignored. Sentinel checks use `errors.Is`.
  The registry DSN parse error is deliberately not wrapped.
- `diagnostic()` adds the wrap-chain types and context state, still without
  messages. Regressions: `TestRedactedDiagnostics` (wrapped secret absent, SQL
  number and chain present) and `TestWrappedErrorResponses` (wrapped SQL/API
  errors keep their 409/400 mapping; an unmapped wrapped error is a generic 500).
- `slog` is configured in `main` (text in development, JSON otherwise). One
  access line per request with request ID, method, route, status, bytes and
  duration. `TestAccessLog` sends a secret in query, cookie and Authorization
  and verifies it never reaches the log and that a client `X-Request-ID` is replaced.

Executed checks: `go build ./...`, `go vet ./...`, `go test ./...` **PASS**;
golangci-lint **0 issues**; schema validator and driver verification **PASS**;
control integration suite with the local fixture configuration
(`go test -race -count=1 ./internal/control`) **PASS**, 51 tests, 132.7 seconds.
Restarted with `scripts/dev.py`: `/readyz` 200, access lines in
`.local/lastapp.log`, and a probe with a secret in query/cookie/Authorization
left no password/token/cookie/Authorization text in the new log lines.

## Console useAction and ConfirmDialog — 16 November 2026

- Screen mutations share `web/admin/src/use-action.ts` (double-submit guard,
  stale-route/unmounted guard, one `errorMessage()` helper). A browser check
  double-clicked "Recheck connection" on a Tenant database resource with the
  request delayed 400 ms and counted exactly **one** POST.
- `window.confirm()` is replaced by `ConfirmDialog` (`confirm.ts`,
  `confirm-dialog.tsx`, focus handling in `dialog.ts`). `tests/confirm-dialog.spec.ts`
  checks at 1280×800 and 390×844 that the dialog fits the viewport, focuses Cancel
  for destructive actions, traps Tab, cancels on Escape without a request,
  returns focus to the trigger and proceeds on confirm. A one-off designer check
  confirmed remove-field, back-to-list and guarded console navigation (URL
  unchanged while asking, unchanged on Cancel, navigates on confirm).
- `npm run lint`, `npm run typecheck` and `npm run build` passed. Full Playwright
  suite: 27 of 29 passed. `workspace-regressions.spec.ts` still used the removed
  native dev-SSO select; its selector was updated and the test passed on rerun.
  `app-runtime.spec.ts` still fails as it does on main (popup on port 8789 times
  out).

## Lint baseline — 16 November 2026 (PR #1)

- `.golangci.yml` (golangci-lint v2.15.0: govet, errcheck, ineffassign,
  staticcheck SA/S1, unused; gofumpt and gci formatters; `third_party` excluded)
  and `make lint` / `make fmt` (c90fbf1); Go sources reformatted by `make fmt`
  (4c8f06d). errcheck findings handled or discarded with a reason, and tests fail
  on unchecked Exec/Scan/Unmarshal (6d45bb1); two lookup errors hidden by
  shadowing/unchecked Scan now return (0c14507). CI runs both linters (1ae5e25).
- Console: ESLint (typescript-eslint, react-hooks with exhaustive-deps as an
  error, react-refresh) plus `noUnusedLocals`/`noUnusedParameters`/
  `noFallthroughCasesInSwitch`; 73 existing findings went into
  `eslint-suppressions.json` (3698834), removed later in PR #5 (53e8175).
- Native `<select>` and number fields replaced by `Select`/`NumberInput` on a
  shared listbox (b648a2e, then the entity designer and agent access form);
  console URLs built and parsed from one route table, `console-url.ts` (c4ff3cc).
- `make dev` starts MariaDB from `docker-compose.yaml` when no local `mariadbd`
  exists or `LASTAPP_MARIADB=docker` (623e571).
- Executed: the `test` check of the `Checks` workflow passed on the PR (final head
  ab7f4f4, two runs, 16 November 2026). No other result for these commits is
  recorded here.

## Follow-up workspace/release review fixes — 14 November 2026

All four findings from the next review are fixed:

- Undeployed release inspection uses the shared workspace ownership/delegation
  guard. Another human manager or another grant for the same human cannot read
  private snapshots through the API/MCP. Only successful deployments expose a
  snapshot as authorized production history, which remains inspectable after
  workspace archive without opening its private backing Space.
- Preview activation validates the settled Space snapshot and identity mappings
  inside the same locked mutation transaction that selects the client release.
  The regression observes the actual preview request waiting on the Tenant lock,
  commits a concurrent role change and verifies rejection without client changes.
- Startup rejects console hostnames equal to or below the App base hostname,
  including case/trailing-dot/port variations and UUID-shaped console names.
  Startup and static routing use one shared hostname namespace definition.
- Workspace App creation/restoration share one quota check. Development uses
  its metadata cap; production creation/restoration and deployment retain the
  subscription quota. The console explains this distinction without copying
  backend limits. Current workspace/MCP guides and runtime agent context agree.

Executed checks:

- Full backend race/security suite using both local fixture configurations:
  `go test -race -count=1 ./...` **PASS**, control package **160.511 seconds**.
- Focused new regressions with race detection: **PASS**, **10.005 seconds**.
  Coverage includes private API/MCP reads, completed versus queued/failed release
  history, archived workspaces, real concurrent preview mutation, separate quota
  limits, restoration at the workspace cap and deployment quota rejection.
- Full Chrome final rerun: **28/28 PASS (1.9 minutes)**. The initial run passed
  27/28; its expired-session
  test cleared cookies while initial collection requests were still starting.
  The UI correctly returned to login before the test clicked Sign out. The
  isolated unchanged test then passed. Its setup now waits for the collection
  and opens the menu before expiry, and asserts the actual logout 401 response;
  that check passes in the complete final rerun.
- Console TypeScript/Vite build, `go vet ./...`, canonical schema parity,
  `git diff --check`, **42 pinned driver files** and **23 current-guide local
  links**: **PASS**. Desktop/mobile workspace-create screenshots were inspected;
  the new browser flow verifies successful development App creation at full
  production capacity with unchanged production inventory and no mobile overflow.
- Local service rebuilt/restarted, registry remains at migration 12; `/readyz`
  returns **ready**. Existing local data and keys are preserved. No commit/push.

Local MariaDB and installed Chrome were used; no remote SaaS deployment or native
Codex/Claude connection was exercised. Logs are retained in ignored
`.local/review-pass2/`, including the initial browser failure and its reruns.

## Workspace/release review fixes — 14 November 2026

All five reported findings are fixed. Release storage publications now stop at
`prepared`; role states, entity versions/ACL projections, App policies and client
selection commit together. Physical version checks deny old runtime snapshots
across activation. Failure rolls back live registry authority, clears pending
versions and restores only verified compatible gates. Gate opening is idempotent
across a process loss before registry commit.

Published ACL references to subsequently disabled users/teams copy into a
workspace without authorizing those recipients; ordinary authoring still rejects
new inactive recipients. App host routing canonicalizes default ports and keeps
the static hostname namespace separate from management for alternate port forms.
The shared App deployment-status operation restores UI/MCP context after a lost
response or reload; transient UI polling errors retry automatically.

Executed against the final implementation:

- Complete backend race/security suite with both local fixture configurations:
  `go test -race -count=1 ./...` **PASS**; control package **164.072 seconds**.
  This includes role-reactivation/removal ordering, inactive user/team baselines,
  lost-build failure, activation-transaction failure, old API restoration,
  interrupted gate opening/restart, status authorization and cross-agent denial.
- Full Chrome suite: **27 passed (1.8 minutes)**. A real local React build is
  uploaded, previewed against isolated data and deployed to production. Status
  survives reload; an injected 503 is retried without user action; failed jobs
  remain visible after reload. Desktop/mobile screenshots were inspected and
  the mobile viewport has no horizontal overflow.
- Console TypeScript/Vite build, `go vet ./...`, canonical schema parity,
  `git diff --check` and all **42 pinned driver files**: **PASS**.
- Local service rebuilt/restarted with registry migration 12; `/readyz`: **ready**.
  Existing local data and keys preserved. Changes remain uncommitted.

The initial full backend pass also succeeded before the additional restart
regression identified MariaDB's zero-changed-row retry behavior; the idempotence
fix and the final full rerun above supersede that receipt. No alternate MariaDB
version, remote SaaS deployment or native remote-agent session was tested.
Compatible DDL/draft metadata can remain after deployment failure, and changed
entity APIs can pause during preparation; no destructive rollback is provided.


## Agent onboarding documentation and final review — 14 November 2026

AGENTS.md now directs new agents to current architecture, source contracts, API
boundaries, security rules and verification commands. CLAUDE.md imports those
same instructions. DEVELOPMENT.md maps the implementation and separates shipped
features from future client hosting, design packages, runners and connectors.
MCP.md documents connection/consent, all management tools, context discovery,
entity authoring, delegation ceilings, replay/revisions and worker guardrails.
README links to these guides; foundation/entity guides and historical-plan status
notes were reconciled with the implemented runtime and MCP.

The canonical SQL document was missing the MCP additions. It now derives from
all executable migrations. The existing schema utility supports offline `--check`
and `--write`; CI checks parity alongside driver integrity. Its default database
validation behavior remains intact.

Executed checks:

- **120 local Markdown link targets valid**; the MCP guide matches all **14** tool
  names from the Go catalog; CLAUDE.md imports AGENTS.md.
- Offline schema parity and generation: **PASS**. An isolated temporary fixture
  confirms stale SQL is rejected, only Up migrations are generated, regenerated
  SQL passes parity and mutually exclusive modes are rejected without database
  access.
- Default schema validator plus the real local foundation SQL/API test: **PASS**,
  control package **2.611 seconds**.
- Final MCP review rerun before these documentation-only/runtime-neutral changes:
  `make mcp-test` **PASS with race detection**, **48.934 seconds**. No new actionable
  findings in that pass. The preceding full backend/browser results remain below.
- `git diff --check` and pinned driver verification: **PASS**, **42/42 unchanged**.

These checks use the existing local MariaDB; no new engine/version matrix or
benchmark ran. The CI parity step is locally verified, not a claim of a completed
remote workflow run.

## Second MCP review fixes — 14 November 2026

All six findings from the follow-up review are fixed:

- Tenant targets require a valid bound Tenant ID before entering the shared management API. An empty Tenant target cannot be interpreted as platform scope and bypass `platform.manage`. Target normalization uses a private copy, preserving request-key inputs.
- Each MCP POST accepts one bounded JSON-RPC object. Legacy batches, including requests without a protocol header, are rejected before SDK dispatch; one admitted request cannot fan out into many tool executions or responses.
- App entity inventories apply authorization, search and paging before response serialization. The shared paging helper validates selectors and reports completeness; hidden entities and fields remain filtered by the App ceiling.
- Entity checks use the shared validator's diagnostics-only response. Large valid definitions can complete checks without embedding generated schema and TypeScript. Results retain candidate identity and synthetic-case diagnostics; agents can inspect generated contracts separately with the existing get tool.
- Queued database and entity-publication jobs retain the delegating user, grant and OAuth client in terminal audit events, including work rejected after revocation. Audit metadata is separate from execution authority and local to each job.
- Creating a Space that reuses an existing shared database requires resource-management authority. Actual allocation still passes the shared provisioning permission, target, mode and quota checks.

Permanent regressions cover the platform escalation through a real MCP request, legacy batch admission, oversized requests, large entity checks, authorized App inventory pages, shared database reuse versus allocation, and successful/revoked worker audit attribution. The Chrome paging test also reuses the searchable fixture helper rather than assuming its administrator is on the first user page.

Validation on the existing local MariaDB and pinned driver:

- Full backend integration/race/security suite: **PASS**, control package **108.213 seconds**, including live provisioning/publication and all new regressions. Only the explicitly opt-in fixture-preparation helper skips locally.
- Complete installed-Chrome suite after the fixture correction: **25/25 PASS (1.4 minutes)**.
- `go test ./...`, `go vet ./...`, TypeScript/Vite production build and `git diff --check`: **PASS**.
- Driver verifier: **42/42 pinned files unchanged**. Registry schema remains **8**.
- Backend and embedded console rebuilt and restarted; `/readyz` returns `ready`. Existing local data and keys are preserved.

Earlier attempts are retained in ignored `.local/mcp-review2/`: a host-interrupted MCP run was followed by a successful focused rerun of both affected cases (**9.331 seconds**). The first complete browser run passed 24/25 and exposed the fixture's first-page assumption; the corrected full browser rerun passed all 25. The backend pass is in `fixes-integration.log` and the final browser pass in `fixes-browser-rerun.log`.

## MCP review fixes — 14 November 2026

All seven findings from the MCP implementation review are fixed:

- Managed allocation and retry share the same grant target/mode/quota checks. Claiming an existing resource consumes a slot; retrying the same resource does not consume another. Requeued work records the current agent/user and stops after revocation. Explicit human retries clear the previous agent binding.
- Access-token validation loads its exact grant revision and permissions in one database statement. Subsequent checks reject stale snapshots; an old access token cannot acquire an expanded grant without refresh/reconnection.
- Anonymous registration/metadata setup, token endpoints and MCP token validation have bounded admission budgets. Bootstrap and validation rate maps are separate from ordinary authenticated request limits. Bearer admission runs before database lookups, and anonymous bootstrap cannot occupy authenticated MCP execution slots.
- The in-process response adapter retains writer and JSON decoding errors. Oversized reads report `response_limit`; oversized responses after a committed mutation return the compact resource receipt and explicit response error. Replaying the same request key remains safe. Entity inspection supports separate draft/published definitions, individual generated sections and explicitly marked single-field contracts. App inspection continues to omit hidden fields and policy expressions; App inventory uses concise metadata.
- The existing list tool supports typed user/team recipient discovery through the shared authorized endpoint. Space/App context includes both recipient inventories with selector-preserving continuation guidance.
- Unapproved registrations have a separate bounded quota and expire after ten minutes. Invalid authorization attempts cannot renew their lifetime. Approved clients do not consume anonymous quota; cleanup preserves live grants and in-progress consent/code exchanges. An index supports pending-client checks (registry schema 8).
- Clipboard and revoke failures leave the loaded connection list and its actions usable. Failed loads are tracked separately from action errors.

Validation on the existing local MariaDB and pinned driver:

- Full backend `make integration` race/security run: **PASS**, control package **85.380 seconds**, including the new regressions and real provisioning/publication checks. Only the explicitly opt-in fixture-preparation helper skips locally.
- Complete installed-Chrome suite: **25/25 PASS (1.3 minutes)**, including denied clipboard access and a rejected stale revoke followed by refresh/recovery. Desktop connection and mobile edit screenshots inspected.
- `go test ./...`, `go vet ./...`, TypeScript/Vite production build and `git diff --check`: **PASS**.
- Driver verifier: **42/42 pinned files unchanged**. No database-version matrix or benchmark run.
- Backend and embedded console rebuilt; `/readyz` returns `ready` at the existing local service. Existing local data and keys are preserved.

## Single-source password policy — current, 12 November 2026

DRY is recorded as the main foundational engineering principle in `AGENTS.md`, README and the foundation contract. Password limits are defined once in `internal/control/passwords.go`. Validation, error text and public `GET /v1/config.password_policy` derive from that source. The running server publishes 8–128 characters with `Cache-Control: no-store`; the embedded console/index is also served without caching.

All initial-password fields share one field definition. All new-password inputs share one policy-driven control, including first-login replacement and ordinary account settings. There are no frontend password-limit constants or hardcoded fallback policy. Login/current-password fields remain independent of new-password requirements. Password generation respects the server bounds. Unicode code-point validation matches Go rather than native HTML UTF-16 limits. Public settings refresh after sign-in and on focus/page restoration, updating open forms without clearing drafts. Missing initial settings show an explicit retry instead of guessing policy.

New browser regressions supply a different policy (9–12 characters), verify all creation/change forms and generation, update an open form to 8–64 without losing its draft, verify missing-policy retry and complete real eight-character local onboarding after a stricter policy was loaded on the login page. The latter captures `artifacts/password-policy-onboarding.png`. Backend tests verify the unauthenticated public contract, no-store header and Unicode boundaries against the authoritative validator.

Full backend race suite with existing local integration/security configs: **PASS**, control package **47.932 seconds**; only the opt-in CI fixture-preparation helper skips locally. Complete installed-Chrome suite: **19/19 PASS in 1.1 minutes**, including all three new password-policy regressions and real eight-character account/password lifecycle checks. `go vet ./...`, TypeScript/Vite production build, `git diff --check` and driver verification (**42/42 unchanged**) pass. API and embedded console were rebuilt and restarted while preserving local data and credentials. Detailed logs are in ignored `.local/password-policy-dry/`.

## Earlier password length adjustment — 12 November 2026

Local account creation/bootstrap and password changes now accept **8–128 characters**. The API counts Unicode code points. Shared browser form settings show the requirement before submission and apply it to initial account passwords and replacement passwords; external database credentials keep their separate behavior. Existing passwords and hashing/session safeguards are preserved.

Focused verification: password-boundary cases (including Unicode), local bootstrap/login guards and the Tenant/account/password lifecycle all pass with `-race` on the existing local MariaDB (**25.989 seconds**). The integration fixtures use exactly eight-character initial and replacement passwords. `go vet ./...`, the TypeScript/Vite production build and `git diff --check` pass. Both targeted Chrome scenarios pass (**2/2 in 9.3 seconds**), covering Tenant/new-administrator creation, global account creation, login and initial-password replacement with eight-character passwords. The updated API and embedded console are running.

## Console review corrections — 12 November 2026

All five findings from the UI-simplification code review are fixed in the running console:

- Existing platform/Tenant/team roles can be revoked while the account, membership or team is disabled. New assignments still require active recipients and last-administrator guards remain enforced. Chrome checks revoke platform authority from a disabled account, Tenant authority from a disabled membership and team roles across disabled team membership/team states without restoring access.
- Platform **Users → Add to Tenant** selects an existing local or SSO identity and creates ordinary membership through the existing API. No automatic administrator grant, credential creation or email-based OIDC linking occurs. Tenant administrators still cannot read the global identity directory.
- Space/App Access collections use the parent refresh counter for manual/background updates. Open role editors keep their drafts and captured revisions; a concurrent update remains a 412 instead of being overwritten.
- Tenant administrator and user-recipient searches include email; team-recipient queries use their own columns. Database tests check the user/team query variants and scoped authority restrictions; Chrome selects a role recipient by email.
- App compute preferences are displayed and edited under Advanced using the creation form's shared field definition. Chrome verifies retaining an existing dedicated preference after plan support is removed, changing to shared and preventing a new dedicated selection.

Validation on the existing local MariaDB and pinned driver:

- Full `go test -race -count=1 -v ./...` with integration/security configs: **PASS**, control package **63.007 seconds**. Only the opt-in CI fixture-preparation helper skips locally.
- `go vet ./...`, TypeScript/Vite production build and `git diff --check`: **PASS**.
- Complete installed-Chrome suite: **16/16 PASS in 1.1 minutes**, including all three new review-regression scenarios. The focused regressions also passed independently (**3/3 in 19.0 seconds**). The ordinary-membership form was visually inspected in `artifacts/review-add-to-tenant.png`.
- Driver verifier: **42/42 pinned files unchanged**. Registry schema and database bindings are preserved. The rebuilt API/embedded console is running and ready at `http://127.0.0.1:8787/console/`.

## Console simplification — 12 November 2026

The console now uses task-based navigation and shared shell/forms/collections/pickers. Platform navigation is Tenants, Users, Activity and Settings; Tenant navigation starts at Apps. Space/App detail screens open on their actual collections. Members and team roles appear together; database servers/plans and technical form overrides are secondary settings. Browser URLs preserve direct links, reloads and Back/Forward. The account menu groups appearance, password and sign-out actions.

Tenant administrator management is one atomic platform operation: **Tenants → Open → Administrators → Add administrator**, using either an existing identity or new email/password credentials. Tests verify account/membership/grant rollback, idempotence, membership recovery, credential preservation, secret omission, privilege separation and scoped recipient lookup. Name-only Space creation uses eligible managed targets, shared-resource reuse and effective plan defaults; an unconfigured target fails cleanly without partial allocation. Registry schema is unchanged.

Validation uses the existing local MariaDB and pinned driver:

- Full `go test -race -count=1 -v ./...` with the local integration/security configs: **PASS**, control package **48.272 seconds**. Only the opt-in CI fixture-preparation helper skips locally.
- `go vet ./...`, TypeScript/Vite production build and `git diff --check`: **PASS**.
- Driver verifier: **42/42 pinned files unchanged**. No driver changes, alternate server-version testing or benchmarks.
- Complete installed-Chrome suite against the final embedded console: **13/13 PASS in 46.7 seconds**. It covers new/existing administrator creation, required initial-password rotation, name-only Space/App/micro-app creation, shared database provisioning and existing registration with an unchanged business-table marker, inline Tenant/team role management, stale revisions, delayed old-Tenant requests, direct URLs/reloads/Back/Forward, server search/paging, picker Load more, Vite authentication/writes, App-only visibility, session recovery, themes, mobile layout/focus and disabled browser storage.
- Final desktop, administrator, team, dark-form and mobile screenshots were inspected in `artifacts/simple-*.png`. Chrome verification caught a hidden desktop sidebar control; the final build corrects it. Selector failures during the test rewrite were corrected to target the custom picker listbox rather than hidden native select options. The onboarding/mobile scenario also passed after adding assertions that wait for the responsive sidebar transition to settle (**1/1 in 6.3 seconds**); the final mobile capture has a hidden sidebar and full-width content.
- Rebuilt API and embedded console restarted successfully; final readiness: **ready** at `http://127.0.0.1:8787/console/`. Existing local data, keys and database bindings are preserved.

## Tenant workspace review fixes — current, 12 November 2026

All four findings from the Tenant-workspace review are fixed in the running API and console:

- Global email/password account creation requires platform administration. Tenant membership creation rejects attempts by Tenant administrators to claim a new email or replace an existing password. Platform **Users → Create account** provides the authorized creation flow without granting automatic roles/memberships; Tenant **People** adds existing accounts by email or user ID. Authority is checked again inside the mutation transaction. Verified self-service invitations remain future work.
- Optional `trusted_proxies` identifies exact ingress IPs/CIDRs. Login resolves `X-Forwarded-For` from the direct peer towards the first untrusted hop. Direct header spoofing, injected chain prefixes, malformed/oversized chains and IPv4/IPv6 normalization are covered. Distinct proxy clients have independent budgets, while attacks against one email retain a shared limit.
- **Home → View all Apps** opens a scoped, paginated/searchable directory. A real Chrome scenario creates 101 Apps for an App-only manager and a private sibling App: the manager has no parent-Space access, reaches the oldest App through paging/search, opens its micro-app controls, and cannot discover the sibling.
- Session expiry/revocation clears the workspace and returns to login; signing out an expired session also succeeds locally. Wrong current-password credentials stay on the form. A real old-session request held across password rotation returns 401 after revocation but cannot close the newly authenticated session.

Final checks on the existing private MariaDB 12.3.3 and pinned driver:

- Full `go test -race -count=1 -v ./...` with integration/security configs: **PASS**, control package **41.186 seconds**. Only the opt-in CI fixture-preparation helper skips locally. The first run exposed a timing assumption in the new rate-budget test under the race detector; its final burst assertions use the fast invalid-length path after rate charging, avoiding password-hash timing dependence.
- Complete installed-Chrome suite: **13/13 PASS in 55.0 seconds**. The expanded account/password-rotation scenario subsequently passed again (**1/1 in 4.1 seconds**).
- `go vet ./...`, TypeScript/Vite production build and `git diff --check`: **PASS**.
- Driver verifier: **42/42 pinned files unchanged**. No driver changes, database-version matrix or benchmark run.
- Registry schema is unchanged by these four fixes. Existing data, keys and database bindings are preserved.

## Earlier Tenant workspace and local accounts implementation, 12 November 2026

The running local console has distinct Platform and Tenant workspaces. The implemented contract and review steps are in [FOUNDATION.md](FOUNDATION.md) and [README.md](README.md).

Final validation uses the existing private MariaDB 12.3.3 and the pinned Go driver; no database-version matrix or performance benchmark was run:

- Full `go test -race -count=1 -v ./...` with integration/security configs: **PASS**, control package **37.493 seconds**. This includes existing provisioning/authorization/security checks plus local bootstrap without OIDC, email/password login, initial-password restrictions, session rotation/revocation, existing-account reuse without credential replacement, rollback, secret omission, origin/throttle/hash-concurrency guards and strict platform/Tenant administrative separation. Only the explicitly enabled CI fixture-preparation helper skips locally.
- `go vet ./...`, `git diff --check` and TypeScript/Vite production build: **PASS**.
- Complete installed-Chrome suite: **10/10 PASS in 42.1 seconds**. It provisions a local administrator through the UI, changes the initial password, builds Space → App → micro-app, verifies real managed provisioning and existing registration without business-table modification, exercises teams/roles, stale editor protection, server paging/search, Tenant switching, Vite login, themes, mobile navigation and disabled browser storage.
- The Tenant-switch regression deliberately delays a successful old-Tenant form request, switches Tenants, then verifies that its completed response cannot open an old-Tenant dialog in the new workspace.
- Canonical 18-table DDL parity and the executable foundation SQL/API validator: **PASS** on a disposable registry on the same server.
- Driver snapshot: **42/42 pinned files unchanged**.
- Rebuilt API/embedded console running; readiness: **ready**.

Chrome verification caught and corrected an accessible password-field label and Space/App metadata refreshes remounting access editors. Final screenshot review also caught long Tenant names overflowing the sidebar selector; the selector is now constrained to the sidebar. Both affected registration/access and server-paging Chrome scenarios passed again (**2/2 in 19.3 seconds**) with selector-boundary assertions after the layout correction. Final desktop, dark-theme, Space/App access and mobile screenshots were inspected in `artifacts/tenant-*.png`; mobile tables scroll inside their containers.

Existing local data, encryption keys and database bindings are preserved. Local credentials add one global table and a session authentication-method column. Email recovery delivery, MFA, business entities/ACL execution, integrations and App execution remain outside this stage; the UI exposes working functionality only.

## Review fixes — 12 November 2026

Both review findings are fixed in the running local API and embedded console:

- Tenant request budgets are charged only after active membership or platform authority is established. Per-user throttling remains first. The regression covers a 120-request outsider burst, nonexistent/arbitrary Tenant IDs, disabled membership, legitimate member traffic and platform access without membership.
- Team editors submit their captured revision even when background queries refresh current metadata. Editing is disabled until the selected team is loaded. A real Chrome/API regression opens a draft, concurrently renames and disables the team, triggers a background refresh, and verifies that the stale save returns 412 without undoing the disable. Reopening and saving against the new revision succeeds.

The new tests failed against the original implementation before the fixes. Final validation uses the same existing private MariaDB instance and pinned driver:

- Full `go test -race -count=1 -v ./...` with local integration/security configs: **PASS**, control package **14.810 seconds**; only the explicit CI fixture-preparation helper skips locally.
- `go vet ./...` and `git diff --check`: **PASS**.
- TypeScript/Vite production build: **PASS**.
- Complete installed-Chrome suite: **10/10 PASS in 36.3 seconds**.
- Rebuilt API and embedded console restarted successfully; local readiness reports **ready**.

## Earlier identity roles and scopes foundation

12 November 2026, macOS/arm64, using the existing private MariaDB 12.3.3, the pinned `aantich/go-mariadb` driver, Go 1.27.4, NVM Node 24.20.0 and installed Chrome. The API and console are running against `lastapp_foundation`; earlier local databases remain intact.

Final results:

- `go test -race -count=1 -v ./...` with both local integration/security configs: **PASS**. The control package completed in **14.491 seconds**, including the new foundation, live provisioning and all 15 security regression subtests. The CI-only fixture preparation helper is not invoked in this local run.
- `go vet ./...`: **PASS**.
- TypeScript/Vite production build: **PASS**.
- Complete Chrome suite: **9/9 PASS in 33.7 seconds**. This covers the updated scoped-role UI, real provisioning/registration, fixed catalogs, team membership disable/enable, custom Space/App roles, team recipients, pagination/search, Vite login and all theme/accessibility scenarios.
- `python3 scripts/verify_driver.py`: **PASS**, 42 pinned files unchanged.
- `python3 design/validate_schema.py`: **PASS**. Canonical DDL matches executable bootstrap SQL, then the current Go foundation SQL/API checks pass on a disposable registry on the same local server.
- Local readiness: **ready**. The Northstar demo’s CRM database is **ready**, with its two registered Apps. Space/App access screenshots were inspected for layout and shared styling.

Foundation assertions cover true team scopes; fixed administrative catalogs; business/control separation; manager assignment authority; cross-Tenant/Space/App rejection; team-recipient inheritance; user/membership/team/role disable and re-enable; removal/rejoin semantics; stale membership revisions; paged caller roles; SQL domain/null-scope constraints; and concurrent last-Tenant-admin removal.

Browser verification caught a create-team form hidden before a scope was selected; that flow was corrected and passes. Running the final checks during development startup exposed a brief audit-write privilege grant in the startup script. Bootstrap now grants schema-level SELECT/INSERT only and individual table UPDATE/DELETE permissions, excluding audit history. The final Go run after corrected startup confirms that runtime credentials cannot modify/delete audit rows.

The implemented API/scope is [FOUNDATION.md](FOUNDATION.md). Business-entity ACL evaluation, authorized SQL compilation, App credentials and the App SDK remain deferred. These results do not claim production load or deployment validation.

## Earlier provisioning stage verification

12 November 2026, local macOS/arm64. Actual execution against the private MariaDB instance; no mock database.

## Environment

- Go 1.27.4; module baseline Go 1.25.
- Node 24.20.0 from the working NVM installation, React 19, Vite 7, Tailwind 4.
- MariaDB 12.3.3, through the dedicated `aantich/go-mariadb` driver snapshot.
- Installed Google Chrome, driven by Playwright.

Final result: **all Go tests passed with the race detector; `go vet ./...` passed; all 3 Chrome scenarios passed** (final browser run: 19.5 seconds). Frontend production build passed.

## Executed checks

`go test ./...` and `go vet ./...` cover compilation and static checks. `go test -race -v -count=1 ./internal/control` with `LASTAPP_TEST_CONFIG` enables the real MariaDB integration scenario in addition to unit tests.

The integration scenario checks:

- Encrypted credential round trips, tampering and wrong owner/Tenant rejection; malformed request validation.
- OIDC-backed identities cannot create Tenants without platform grants.
- Last direct Tenant administrator protection; last platform administrator protection is also exercised through the browser API.
- Concurrent shared Space creation produces one shared physical database; duplicate Space creation does not allocate an orphan resource.
- Managed provisioning reaches ready; verification retry preserves credentials and database ownership.
- Dedicated database bindings cannot be reused by a second Space.
- App-scoped collection reads and writes, scoped pagination, sibling-write denials and stale revisions.
- Group membership, inherited grants and immediate revocation when disabling a group.
- Tenant custom roles cannot include platform permissions.
- Existing database registration preserves a business-table marker and validates schema CRUD grants.
- Bad registered credentials fail cleanly and replacement credentials recover the same resource.
- Unknown database ownership and pre-existing account collisions are refused; unrelated account passwords remain valid.
- Killing the worker leader's pinned registry connection during target work recovers to ready safely.
- Plan layout restrictions, pending-resource quotas, cross-Tenant references and suspended Tenant allocation rules.
- API/audit responses omit connection secrets. The registry runtime account is denied UPDATE/DELETE on audit history at the database level.

`make e2e` (formerly `npm run test:e2e` against the development stack) uses real OIDC redirects and Chrome to check:

1. Tenant → shared Space/ready database → App → micro-app, using console forms.
2. Missing CSRF rejection, stale revisions, last-superadmin protection and unauthorized outside-user access.
3. Existing database registration with unchanged business data; membership, custom role, group and grant management; platform access, plans, targets and activity screens; desktop and mobile screenshots.

Screenshots are saved locally in `artifacts/`: `admin-overview.png`, `admin-microapps.png`, `admin-databases.png`, `admin-access.png`, `admin-mobile.png`. Playwright retains traces and screenshots for failed runs in ignored local folders. Browser scenarios assert no uncaught page errors.

The earlier standalone DDL validation report, [design/VALIDATION.md](design/VALIDATION.md), records 65 structural checks. It is separate from the Go/API/browser tests.

## Boundaries

These checks verify the provisioning stage, not production load, multi-region operation, every external OIDC provider, or business-entity APIs. MariaDB 10.11 remains a compatibility-matrix task; the executed server version was 12.3.3. Automatic key rotation, large-directory console paging, production packaging/deployment and App scheduling are not claimed as implemented.

Test-owned database resources, including intentional failures, are retained locally for inspection. Tests create isolated IDs/names and do not drop existing databases or user accounts. `make stop` stops services without deleting data.

## Admin design and themes — 12 November 2026

The Paces Neo-inspired redesign passes the frontend TypeScript/Vite production
build and Go compilation through `make dev`. The final complete Chrome run
passes **7/7 scenarios in 35.0 seconds**, using the existing local API, OIDC
issuer and MariaDB. The original three provisioning/access scenarios still pass.
The four additional scenarios verify:

- Sidebar collapse/expand; actual light/dark surface changes; preset persistence
  across reloads, tables and edit forms; reset; Escape and focus restoration.
- System color-mode changes, preference synchronization between tabs and
  recovery from invalid stored values.
- Mobile workspace selection and navigation, theme controls, focus containment,
  Enter-key navigation search, and no horizontal page overflow at 390px.
- Console navigation and theme switching when browser storage throws.

The authenticated appearance scenario also checks for browser console and
uncaught page errors. The initial signed-out `/v1/me` probe intentionally returns
401 and is outside its console-error assertion. Earlier checks caught and led
to fixes for the radio hit area and Enter-key search reopening; screenshot and
color assertions now account for finite CSS transitions.

Desktop Neo, Studio dark forms and the mobile Evergreen dark appearance drawer
were visually inspected. Additional local captures are in
`artifacts/admin-neo-overview.png`, `artifacts/admin-studio-dark-form.png` and
`artifacts/admin-mobile-appearance.png`. Design observations, raw local reference
locations and theme extension instructions are recorded in
[the design reference](design/references/paces-neo.md).

This design pass changes the frontend and its documentation/tests. It does not
rerun or relabel the earlier Go race/integration results above.


## Security corrections — 12 November 2026

All ten findings in the initial security review are corrected; see [the remediation and verification record](review/security-2026-11-12/FIXES.md). The existing registry is on migration 2. The full Go/API suite passes with `-race`; `go vet` passes; the TypeScript/Vite build passes; and the final Chrome run passes **9/9 scenarios in 34.2 seconds**, including server paging/search and Vite login plus authenticated writes. `govulncheck` reports no vulnerabilities after the go-jose patch, and all 42 driver snapshot hashes match.

This supersedes the earlier limitations about unpaged collection loading and the latest-150-only audit view. Security regression fixtures are disposable and clean up only their own verified resources; the original integration/browser inspection fixtures retain their existing behavior. The new CI workflow is configured but has not been run remotely. Business-data policy enforcement and generated-app execution remain outside the provisioning phase; their required boundaries are recorded in the provisioning plan.

## Business entities, ACLs and rules — 12 November 2026

The entity implementation supersedes the preceding provisioning-only boundary.
It includes the React designer, generated MariaDB storage, durable publication,
secured data APIs, App-bound authentication, TypeScript SDK and optional record
sharing. General generated-App builds/runners remain outside this stage. Usage
and supported changes are in [the entity guide](ENTITY_RUNTIME.md).

Final checks ran against the existing local MariaDB and the rebuilt Go/React
services, using the unchanged pinned driver:

| Check | Result |
| --- | --- |
| `LASTAPP_TEST_CONFIG="$PWD/.local/config.json" LASTAPP_SECURITY_CONFIG="$PWD/.local/config.json" go test -race -count=1 ./...` with local `GOCACHE` | All packages passed; control integration suite 56.094 seconds |
| `npm run test:e2e --prefix web/admin` with the configured Node | 21/21 Chrome scenarios passed in 1.2 minutes |
| `make client` | SDK TypeScript build and static example generation passed |
| `make dev` | React type/build checks, Go binaries and local startup passed; registry version 4 |
| `go vet ./...` | Passed |
| `python3 scripts/verify_driver.py` | All 42 snapshot hashes matched; driver unchanged |
| Normalized design DDL versus all four migration Up sections | Identical |
| Plan/guide JSON examples, local links and `git diff --check` | Passed |
| HTTP `/healthz`, `/console/`, `/v1/config` | All returned 200 after verification |

The real-database entity regression covers owner ACLs, inaccessible-record errors,
authorized revision conflicts, atomic bulk rollback, count/cursor pagination,
wide-projection page limits, exact decimal and null/text SQL/evaluator parity,
synthetic tests without mutations, interrupted activation recovery and rejection
of unsupported storage changes while preserving the active version. Two Apps
reuse the same entity and records; another Space in the same physical database
cannot cross their namespace or reference boundary. Sharing checks include exact
permission delegation, label versus security changes, grantor revocation and
parent-session revocation. Separate regressions check Tenant concurrency limits
and prove a stalled database open does not block a healthy cached resource.

Browser checks retain the complete provisioning, administration, roles, password,
appearance and workspace regressions. New flows author/test/publish an entity on
managed storage, preserve a dirty editor across a competing draft update, and
connect a static App on a separate host through the actual approval/PKCE/token
flow to create and read real records. Registered storage is covered by database
and integration fixtures. The designer's desktop and 390px screenshots were
visually inspected, including responsive layout and the existing shared theme.

Final local logs are `.local/entities-go-final-all.log`,
`.local/entities-browser-final-all.log` and `.local/entities-dev-final.log`.
Screenshots are `web/admin/test-results/entity-designer-desktop.png` and
`web/admin/test-results/entity-designer-mobile.png`. These generated artifacts
remain ignored. Earlier verification runs exposed and corrected ambiguous test
selectors and an expiration-test race where a pending request could return the
UI to login before Refresh was clicked; the final complete run passed.

Existing development data and pre-existing working-tree changes were preserved.
No database-version matrix, benchmark, driver modification or remote deployment
was performed. The local console remains available at
<http://127.0.0.1:8787/console/>.

## Entity designer layout correction — 12 November 2026

The Fields screen now aligns the name input and Add field button, uses a compact
list with shared edit/remove actions, and groups ownership, default fields,
sharing and lifecycle controls into a properly padded settings disclosure.
Desktop ownership controls share a row and stack on mobile. The same row
component serves Access and Rules. Entity dialogs have one padding layer,
consistent form spacing and grouped review actions. All colors continue to use
the shared appearance tokens.

`make dev` passed the TypeScript/Vite build and rebuilt the running local console.
The complete browser run passed its 20 existing non-entity scenarios; the new
entity layout check initially used an exact label selector that did not match
the select's implicit label. After switching to its accessible combobox role,
the focused entity scenario passed, including real create/test/publication,
desktop input/button and ownership alignment, 390px overflow, mobile dialog
bounds and focus restoration. The final visual run also captures Studio dark
and wide desktop layouts; its final run passed in 8.2 seconds. All four final
captures were visually inspected. Logs are `.local/entity-layout-browser.log`,
`.local/entity-layout-focused.log` and `.local/entity-layout-visual-final.log`.
The screenshot helper resets scroll before full-page captures so fixed headers
are not captured at their previous scrolled offset.

Screenshots are in `web/admin/test-results/entity-designer-{desktop,dark,mobile}.png`
and `web/admin/test-results/entity-editor-mobile.png`. This correction changes
frontend layout/components and their browser verification; backend business
logic and the pinned MariaDB driver are unchanged.

## Complete screen and modal layout review — 12 November 2026

Reviewed the current console screen families and their create/edit/disclosure
variants through real local sessions. The final capture matrix contains **136
states at desktop and mobile sizes (272 captures)**, with no document horizontal
overflow, off-screen dialogs or extra Advanced-section indentation in its final
checks. Captures/contact sheets were visually inspected, with full-size checks
of the dense forms, settings, credentials, onboarding and expression dialogs.

| Area | Reviewed surfaces |
| --- | --- |
| Authentication/account | Sign-in, first password, ordinary password change, SSO account, account before Tenant assignment, App connection approval/error |
| Platform | Tenants, Tenant overview/administrators/settings, users, activity, database servers, plans; their create/edit/account/administrator dialogs and Advanced sections |
| Tenant workspace | Apps, Spaces, Space/App tabs and settings, micro-apps, people, teams and team members, activity, Tenant settings; create/edit/member dialogs |
| Access | Space/App assignments, custom role lists and create/edit dialogs, user/team access assignment, inherited and explicit App ceilings/restrictions |
| Databases | Managed creation, existing registration, resource list, replacement runtime credentials and schema credentials |
| Entity authoring | Library, fields/settings, all eleven field types and relevant options, access subjects/operations/conditions, validation/allowed-change dialogs, all four Test operations and publication review |
| Shared surfaces | Search/paging tables, empty/loading states, appearance controls, mobile navigation and the isolated static data example |

Corrections use shared components and CSS tokens:

- One form spacing/padding/control-height contract; Advanced fields no longer
  acquire a second inset. Single-field forms and initial passwords use the
  available width. Account selection and credential pairs have coherent rows.
- Table action headings align with their actions; long actions wrap. Table text
  is more readable and existing scroll containers retain mobile containment.
- Settings have consistent heading/action groups and spacing. App data access
  is a sibling section rather than a panel nested inside another settings card;
  its developer-only local setup instructions remain in documentation.
- Password onboarding uses a centered single-column grid. The account password
  form has an appropriate section heading and no inherited oversized title or
  excess hint margins. Mobile navigation no longer animates the desktop content
  offset across the layout breakpoint.
- The isolated example aligns its entity selector/actions and uses consistent
  field/button spacing. Colors and appearance presets remain shared in the
  console; no authorization rules or backend behavior changed.

`make dev` passed the final TypeScript/Vite and Go build and left the local UI
running. The complete Chrome suite passed **23/23 scenarios in 1.3 minutes**,
including two new regressions for advanced-field alignment, credential pairing,
responsive dialog bounds, centered onboarding and account use without Tenant
assignment. Existing provisioning, permission, password-policy, theme and App
data flows passed. All 42 pinned-driver hashes matched; `git diff --check` passed.

Local evidence: `.local/ui-audit/final-console/manifest.json` (164 captures),
`.local/ui-audit/final-authoring/manifest.json` (108 captures),
`.local/ui-review-build.log`, `.local/ui-review-browser-final.log` and
`.local/ui-review-example-final.log`. The first audit iteration exposed a bad
audit endpoint and transient measurements during responsive animation; these
were investigated and corrected before the final matrix. It also exposed an
audit navigation race during password replacement; the final audit waits for
the actual authenticated screen, and the permanent regression verifies that
flow. These audit issues are not counted as application failures or passing
checks. Screenshot artifacts remain ignored; prior data and uncommitted work
were preserved. No commit, push or remote deployment was performed.

## Delegated management MCP — 14 November 2026

Implemented the management-only MCP endpoint in the existing Go service using
the official Go SDK and Streamable HTTP. Its 14 typed tools reuse the existing
management handlers, authorization, mutations and workers. Context includes
paginated inventories, full authorized entity definitions, published App
contracts, revisions, current grant limits and explicit unavailable features.
Canonical guides are shared between the describe tool and MCP resources.
Micro-app registration does not yet build or host source; business-record
access and robot identities are outside this release.

OAuth authorization-code/S256 PKCE consent delegates an existing user's current
authority within an explicit Tenant, team, Space, App or platform ceiling.
Audience-bound, hashed credentials, rotating refresh tokens, grant revision
checks and revocation apply to requests and queued work. Provisioning has
approved-target and allocation limits. Mutation receipts make unchanged retries
safe, and shared audit entries retain the human and agent identity. Agents
cannot expand grants, retrieve credentials or execute arbitrary SQL/code.
Native clients require registration; public HTTPS Client ID Metadata Documents
are also supported. The URL-only connection follow-up below adds automatic
dynamic client registration to this initial implementation.

The console now provides consent plus My account → Agent access, with editing,
expiry and revocation; Tenant administrators can inspect and revoke Tenant
connections in Settings. Permissions, presets, eligibility and limits come from
the backend contract. Desktop consent and mobile consent/edit screens were
visually inspected; browser checks cover control sizes, overflow, modal bounds,
SSO return, approval and denial.

Final validation passed:

- `make integration`: the full backend race/integration suite passed in
  63.240 seconds, followed by **25/25 browser scenarios in 1.4 minutes**.
- `go test ./...`, `go vet ./...` and `git diff --check` passed. All 42 pinned
  MariaDB-driver file hashes matched; the driver was not modified.
- The official MCP client exercised real OAuth, schemas/tool discovery,
  resource/role/membership management, managed database provisioning and entity
  publication. Security cases include sibling-scope denial, narrowed context,
  App contract filtering, stale edits, idempotency conflicts, invalid PKCE,
  refresh replay, grant changes/revocation, token audience isolation across
  endpoints sharing one registry, unsafe origins and provisioning ceilings.
- A deterministic regression verifies that publication queued before database
  provisioning waits for storage readiness. Recursive expression schemas are
  checked against actual default/calculated-field and App-gate expressions.
- The rebuilt local server reports ready and advertises the exact
  `http://127.0.0.1:8787/mcp` OAuth resource. The registry is at schema version 6.

Final log: `.local/mcp-release-validation.log`. Screenshots remain ignored in
`web/admin/test-results/mcp-{consent-desktop,consent-mobile,edit-mobile,connections-desktop}.png`.
The console remains running at <http://127.0.0.1:8787/console/>. Existing local
data was preserved. No database-version matrix, commit, push or remote deployment
was performed.

### URL-only agent connection follow-up — 14 November 2026

The user-facing flow now starts in the agent client with the MCP URL, followed
by Lastapp sign-in and consent. Static client registration, secrets, tokens and
manual OAuth scope configuration are unnecessary. Discovery supports both Client
ID Metadata Documents and an advertised RFC 7591 registration endpoint. A
registration stores bounded public metadata only; it cannot grant authority.
S256 PKCE, exact registered host/path matching, dynamic native loopback ports
(including Claude's localhost callback), explicit consent and audience checks
still apply. Registration is rate/concurrency limited and serialized at its
registry cap; cleanup retains clients with live grants.

Discovery offers the supported capabilities so users can choose App/Entity
author permissions on the first connection. Consent still initially selects
inspection only, and issued tokens contain only the selected permissions.
Explicitly requested narrower scopes remain narrow. The console shows the
return address and offers Copy MCP URL with a short connection explanation.
Policy limits and scope lists continue to derive from the backend contract.

Native-client smoke checks used the installed Codex CLI
`0.162.0-alpha.17.2` (default CIMD and explicit DCR) and Claude Code `2.1.263`
(default CIMD). Each discovered OAuth using only the server URL. Following each
real client-generated authorization URL returned HTTP 302 to Lastapp consent;
both vendors' real metadata documents and native callback formats were accepted.
These checks deliberately stopped before approval, so they did not authorize a
connection or save access tokens. Headless commands ended at their callback
input prompt. Claude used an isolated disposable profile; normal client
configuration was not changed.

Two new backend integration scenarios exercise automatic discovery/registration,
mandatory PKCE, redirect pinning, explicit approval, token exchange and the
official MCP client, plus rejection of unsafe/oversized registration metadata.
They verify that registration creates no grant, offered capabilities do not
become authority, and cleanup preserves a live connection. The browser consent
scenario now registers its own public client with no configured scope list and
checks the exact approved token scopes. Consent/account screenshots were
visually inspected. The desktop account capture now opens the account at the
desktop viewport, verifies panel/sidebar alignment and disables transient
animations while taking its image.

The full backend race/integration run passed in 68.647 seconds and all **25
browser scenarios passed in 1.4 minutes**. The final two-scenario MCP browser
confirmation, including the settled desktop capture, passed in 4.1 seconds.
`go test ./...`, `go vet ./...`, driver integrity (42 pinned files) and
`git diff --check` passed. The local console was rebuilt and remains running.

Evidence: `.local/mcp-auto-focused.log`, `.local/mcp-auto-final-validation.log`
and `.local/mcp-native-{codex,codex-dcr,claude}.log`. Schema version 7 adds the
public client metadata table. These ignored local logs contain transient OAuth
request state and should not be published. No native-client approval, commit,
push or remote deployment was performed.

## 14 November 2026 — isolated workspaces and static App releases

Implemented the agreed remote-management/local-authoring flow: owned per-agent
Space copies with empty managed databases, typed identity mapping, development
PKCE sessions, authoritative React/SDK starter files and ZIP download, bounded
immutable filesystem uploads, pinned App release manifests and durable explicit
production preparation/activation. The console exposes Space Development and
App Releases. Current contracts and deployment prerequisites are in WORKSPACES.md;
AGENTS.md and CLAUDE.md reference that guide.

Final validation:

- `LASTAPP_TEST_CONFIG=.local/config.json LASTAPP_SECURITY_CONFIG=.local/config.json go test -race -count=1 ./...` passed; control integration/security package: **143.667 seconds**.
- All **26 Chrome scenarios passed in 1.8 minutes**. The new scenario writes the
  returned starter files to a disposable local directory, compiles React using
  the console's identical pinned dependency tree, uploads through the UI,
  authenticates with real App PKCE and performs workspace CRUD. The same compiled
  files then run on the production App host with its production App ID and an
  empty production dataset. Desktop viewport and mobile captures were visually
  inspected; geometry assertions cover sidebar/content alignment and overflow.
- Integration coverage includes production-record preservation during additive
  schema promotion, cyclic-reference baseline copying, immutable release bases,
  workspace rebasing after success, cross-agent denial for the same user,
  development-only authority, grant-revision session revocation, artifact path
  confinement, upload replay and corrupt-build rejection.
- `go vet ./...`, UI TypeScript/Vite build, generated SQL parity, driver integrity
  (**42 unchanged pinned files**) and `git diff --check` passed. Registry migrations
  9–11 are applied locally; the development service was rebuilt and restarted.

Expanded validation found and corrected a missing build-list pagination contract
and a reference-column ALTER operation that combined incompatible foreign-key
and online-column steps. The existing entity browser scenario also read a draft
revision before its asynchronous save had completed; it now waits for the actual
successful save response. Focused reruns and the final complete suites passed.

Evidence stays ignored locally: `.local/workspaces-final-go.log`,
`.local/workspaces-final-browser.log`, `.local/workspace-focused.log` and
`web/admin/test-results/`. These files can contain transient test authorization
state and must not be published. Native-client remote SaaS setup, wildcard TLS,
shared-volume replica operation, production load and object storage were not
exercised by these local checks. No commit, push or remote deployment was made.
