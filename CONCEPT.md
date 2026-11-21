# Lastapp Platform Concept

**Product direction and decision history — current status updated 14 November 2026.**

For implemented behavior, start with [DEVELOPMENT.md](DEVELOPMENT.md), [FOUNDATION.md](FOUNDATION.md), [ENTITY_RUNTIME.md](ENTITY_RUNTIME.md) and [MCP.md](MCP.md). Provisioning, identity/scoped roles, the entity/rules/ACL runtime, App sessions/SDK and delegated management MCP are implemented. Owned development workspaces, local React starter/build workflows, immutable filesystem-backed static releases and explicit production deployment are also implemented; see [WORKSPACES.md](WORKSPACES.md). Hosted source/Git, remote builds, a shared package registry, object storage, browser Node/WASM runners, external connectors and App container scheduling remain future work. Requirements and architecture diagrams below include that broader direction; they are not claims that those features run today or instructions to implement them without an agreed scope.

The detailed [business entity, ACL and rules implementation plan](ENTITY_DESIGN_IMPLEMENTATION_PLAN.md) is implemented for the initial entity stage; see [the runtime guide](ENTITY_RUNTIME.md). It specifies entity-only authoring with generated storage and a single authoritative document, refining the earlier working entity-language proposal below.

The [Space and authorization plan](SPACE_AUTHORIZATION_PLAN.md) specifies the ownership model. The global identity/roles/scopes foundation and matching console are implemented in [FOUNDATION.md](FOUNDATION.md); entity ACLs, the data runtime and the small App SDK are implemented in the subsequent [entity stage](ENTITY_RUNTIME.md). The provisioning plan records the earlier foundation, whose Tenant custom-role model is superseded.

Lastapp is a framework for creating arbitrary business applications from a shared business entity backend and generated client code. A company organizes its data in Spaces, which own reusable business-object APIs, rules and permissions. Apps inherit this backend and compose client micro-apps. Many client micro-apps reuse those App APIs. The client software runs in the browser and can be distributed as files through static hosting and a CDN.

The central design principle is **a stable backend contract with freely programmable clients**. App authors and agents can generate executable UI, navigation, actions and computations without rebuilding the backend for every application. The framework should stay small enough that this flexibility does not require a proprietary UI language or a large application runtime.

## Requirements established in the discussion

- A company is a Tenant. A Tenant contains Spaces; a Space contains Apps; an App contains client micro-apps.
- A Space supplies a database binding and owns shared business objects, invariants, permissions and roles. Apps normally inherit these and expose client experiences; App extensions are advanced and bounded by Space authority.
- Go is the selected backend language. The initial backend runs trusted platform code enforcing authentication, permissions and business rules; arbitrary generated backend code is outside the initial scope.
- Apps are the logical units of backend deployment and scaling. Small Apps can share runtime containers; larger Apps can have dedicated containers and multiple replicas.
- Database layout is configurable: Spaces may share a database within their Tenant or use separate databases, subject to configuration and subscription policy. Both managed database creation and registration of existing databases must be supported.
- Initially, platform superadmins create Tenants; tenant administrators manage their own Spaces and Apps.
- Identity and authorization are shared platform capabilities. Global users, Tenant memberships, teams, fixed Tenant/team role definitions, Space/App custom roles and scoped assignments live in shared system tables; each App runtime uses the same backend authorization implementation. Tenant creation adds rows, not another identity/permission schema.
- The framework will be open source.
- Apps can address arbitrary business needs. CRM examples illustrate the model; they do not define its limits.
- Client code is real executable code, with React and CSS as the initial UI direction. React/Tailwind is the implemented console stack and starting design direction for generated clients; the micro-app runner remains deferred.
- UI flows, navigation and client computations run in the browser. Backend rules and permissions remain authoritative for API operations.
- Agents generate and modify code that is stored in a library, published and delivered to users.
- Apps reuse components, styles and API tooling so a company can maintain a consistent experience across many independently generated micro-apps.
- Both trusted authors and arbitrary third-party authors must be supported.
- Authors can request arbitrary npm dependencies. The execution model must accommodate both ordinary browser apps and complete Node projects running in WebContainers.
- External systems such as HubSpot and ERPs must be accessible through managed connections, including OAuth where supported.
- Keep the architecture lightweight. Vendors and detailed specifications below remain review decisions where no choice is recorded.

## Product hierarchy

```text
Tenant — a company
└── Spaces
    └── Space
        ├── Database binding, entities, business rules and shared permissions
        ├── Space roles and authoritative access boundary
        ├── Reusable external connections — proposed default scope
        └── Apps
            └── App
                ├── Inherited Space APIs and optional access restrictions
                ├── Optional Space-authorized extensions and connector bindings
                ├── Shared or dedicated backend placement
                ├── Navigation and micro-app composition
                ├── Shared design and component versions
                └── Micro-apps
                    └── Pages, components, actions and client flows
```

| Concept | Responsibility |
|---|---|
| Tenant | Company boundary, users, groups and administration |
| Space | Database binding, shared entities, invariants, permissions, roles and access boundary |
| App | Reuses the Space backend; owns client composition, connector bindings, serving placement and optional bounded extensions |
| Micro-app | A focused client experience containing one or more pages, arbitrary code and any UI flow it needs |
| Library | Reusable entity templates, component packages and versioned app or micro-app code |

A team or group is a Tenant structure that can receive Space/App role assignments and can itself be the scope of a user’s fixed team role. It is not another mandatory level between Space and App. Users may belong to several groups.

Fixed platform, Tenant and team roles and Space/App custom business roles use one global authorization model. Role assignments have one scope. Space permissions and invariants apply across Apps; optional App permissions narrow the Space authority. Team roles can be referenced by business ACLs without copying role assignments into every App. See the proposed [Space and authorization plan](SPACE_AUTHORIZATION_PLAN.md).

For example, a company creates a Sales Space with Company, Contact, Lead and Deal entities and shared permissions. Sales Operations and Forecasting Apps both consume these same definitions, using focused micro-apps. Changes to the shared backend do not require duplicate entity/policy edits in each App.

An App is one logical application with backend configuration and a collection of client micro-apps. This refinement does not introduce a second App hierarchy.

Reusing a library artifact reuses code or definitions. Its data access comes from the installed App’s Space binding; it does not carry another tenant’s database or credentials with it.

## Architecture and responsibilities

```text
Library and publishing
        │
        ▼
Static files and runtime artifacts on CDN
        │
        ▼
Browser App host
  ├── Navigation, shared design and platform context
  └── Isolated micro-app execution
        ├── Browser runner
        └── WebContainer runner
                  │
                  ▼
         Platform API and SDK
          ├── Business entities, rules and permissions → Space database
          └── Authorized integration requests → Connection service → External APIs
```

These are responsibilities, not a requirement for separate services. Go is the backend implementation language. A common runtime loads versioned Space definitions and App bindings as data and can serve many Apps in a shared pool or one App in a dedicated pool. Dedicated pools can add replicas without changing the App API or its Space database binding.

A management service records App definitions and runtime placement. Containers serving business APIs do not need infrastructure provisioning privileges. The first implementation phase is the control plane described in the [provisioning plan](PROVISIONING_PLAN.md).

The host provides the same context and API contract to either execution mode. The backend derives authority from authenticated users and verified App and Space bindings. IDs supplied by generated code do not grant access.

## Business entity backend

### Entity definitions

A Space's backend definition declares entities, field types, validation, relationships, identifiers, business rules and access policies over its database binding. Apps reuse these definitions by default; advanced App-owned entities are still registered and governed through the Space. Reusable entity templates help tenants begin with common objects while allowing their own extensions. Rules that must hold for data shared across Apps require enforcement across every writing path.

**Settled for the implemented entity stage:** use one typed entity document defined by the Go model, including fields, rules and ACLs. SQL/storage plans, permission projections, JSON Schema and TypeScript contracts are generated from it. JSON Schema is an output contract, not an independent source for storage mappings, policies or business operations. See [ENTITY_RUNTIME.md](ENTITY_RUNTIME.md) and `internal/entity`.

Database independence applies to the public entity contract. Adapters implement that contract using the selected SQL or document database. Types, missing values versus nulls, dates, decimals and relationship behavior need explicit semantics so an adapter change does not quietly change application behavior.

Entity evolution also needs a migration contract. Existing data and published clients must remain compatible, or receive an explicit migration or version change. Choosing JSON Schema does not solve this automatically.

### Operations and queries

The implemented API supports single-object and bounded atomic bulk mutations, plus authorized field selection, structured filtering, sorting, cursor pagination and count. Same-Space references are supported. Arbitrary joins, aggregates, full-text/vector search and cross-Space references remain deferred; the exact type/operator/limit catalog is published by the entity capability API.

The implemented compiler validates a small structured query AST, applies authorization and compiles parameterized MariaDB SQL. Rules share the typed expression semantics used by the Go evaluator. Another adapter is a possible later extension, not a current MongoDB implementation or a reason to add a universal storage abstraction now.

This compiler handles a defined language. Natural-language requests or agent-generated code must first produce that language and pass validation. An LLM is not needed in the normal execution path of an already defined query.

Query compilation and database optimization have different responsibilities. Correct compilation makes a request executable; indexes, data distribution and the database optimizer determine much of its performance. Compilation cannot guarantee an optimal plan for arbitrary future workloads.

Portable operations should have consistent behavior across adapters. Additional engine capabilities can be advertised explicitly. The contract should define which requests are supported and return clear errors for unsupported ones.

Bulk operations need declared limits, per-object permission enforcement and a defined failure model. Whether a request is atomic or returns individual successes and failures must be visible to the caller.

### Filtering and search

Structured entity queries and text search need distinct semantics. Filtering Deal records by owner, stage and value is different from searching notes by relevance. Cross-entity search and forecasting aggregates may need dedicated indexes or read models.

Search belongs behind the App API so clients can remain independent of the storage engine. Permission checks must cover search results, counts and aggregates as well as ordinary reads. The initial search implementation and indexing policy remain open.

### Permissions and backend rules

ACL-based access extends the implemented local foundation; see the current [entity runtime guide](ENTITY_RUNTIME.md) and the historical [Space and authorization design](SPACE_AUTHORIZATION_PLAN.md). Tenant/team roles are platform-defined; Space/App business roles can be authored within their scope. Space policies reference ownership, team roles and explicit role assignments. Entity/record/field checks and optional App ceilings compile into the data operation; default deny and additive grants apply beneath mandatory guards. Record shares live beside business records while identities and policy definitions remain centrally managed.

The backend enforces every operation, including bulk requests. A hidden button or a client-side flow cannot authorize a mutation. Where Apps receive restricted capabilities, effective access is the intersection of the user’s permissions and the installed App’s grants.

Backend business rules enforce the conditions already required by the entity model. They are independent of the UI’s navigation and workflow implementation.

## Databases and scaling

Databases remain separated between Tenants. A Space owns a database binding, which its Apps use. Creating an App or micro-app does not inherently require another database.

**Settled requirement:** support both a shared database for multiple Spaces within a Tenant and a separate database per Space. Configuration and subscription policy determine permitted choices and defaults. A Tenant can mix these layouts where permitted. Logical databases may share a physical server or cluster.

**Settled requirement:** support managed database creation and registration of existing databases. Resource ownership is recorded explicitly so attaching an existing database does not imply permission to delete it.

MariaDB is the current database integration and provisioning target. Database independence remains a goal for the entity API; additional adapters can follow the first implementation.

Scale comes from static client delivery, API instances that can scale horizontally, controlled connection pools and placement of databases across infrastructure. Per-tenant query limits and resource accounting prevent one tenant’s workload from consuming all shared capacity.

The ambition is to scale with demand. Database count, connection limits, indexing, data size and request complexity remain capacity constraints; no engine or deployment is assumed to provide unlimited performance.

## Apps and micro-apps

### Executable UI and flows

A micro-app contains real pages and components, CSS, actions calling the platform API and arbitrary compatible browser code. React is the initial direction, with a popular component and styling ecosystem rather than a custom UI description language.

Navigation, form progress, temporary state and multi-page flows can be ordinary generated code. A shared router or small state-machine helper can be used where it makes a flow clearer. There is no mandatory declarative workflow engine or backend workflow model for these client experiences.

The App composes micro-apps and provides navigation between them. A micro-app can have its own internal pages and flow. Both use a documented host API instead of depending on the host's internal implementation.

Persistent business data goes through the App API. Local drafts and transient UI state can live in the browser. A browser flow runs while its execution context remains available; unattended jobs and scheduled work would require a separately defined execution capability.

### Shared components and design

**Working proposal:** publish a versioned React component package, a theme contract and a platform SDK. Generated apps import these packages and can add their own components and CSS. Generation instructions and examples encourage consistent use of the shared library without restricting arbitrary app functionality.

An App pins the component and SDK versions used by its micro-apps. Theme defaults can be shared across a Tenant or Space and selected by the App. The exact inheritance rules remain a design decision.

Each isolated frame loads the applicable component assets and theme. Host CSS and React context do not automatically propagate into another document; the host must supply theme configuration through the integration contract. Shared files can be cached, but a separate frame still has its own execution context.

Shared packages provide consistency and reuse. Arbitrary generated CSS can override them, so visual consistency also depends on generation guidance and review. Component upgrades should be deliberate rather than silently changing every published App.

## Arbitrary packages and execution modes

This is a planned execution design, not a shipped runner. The intended authoring flow accepts arbitrary npm dependency declarations and evaluates them against the requested execution environment. Compatibility is a property of the complete dependency tree and runtime behavior, not just the package name. External runtime requirements and the linked vendor documentation must be revalidated when this feature is designed/implemented.

| Mode | Published artifact | Execution |
|---|---|---|
| Browser | Built HTML, JavaScript, CSS, WASM and other assets | The browser runner loads the UI in an isolated frame |
| WebContainer | Complete Node project with prepared dependencies, lockfile and startup command | The runner starts the project and loads the resulting UI URL in an isolated frame |

The WebContainer mode accepts a complete project: filesystem layout, package scripts, processes and a local HTTP server where required. It is not limited to a Node function called by a separately generated React UI. WebContainers expose an in-browser Node runtime with filesystem and process APIs. [WebContainer API](https://webcontainers.io/api)

Both runners implement the same host lifecycle: start the micro-app, obtain its UI URL, supply the allowed platform context, surface failures and stop execution when appropriate. Runtime selection belongs in the artifact manifest. Browser bundles should be prepared at publishing time; Node runtime files should also be prepared and cached where possible. [Vite production builds](https://vite.dev/guide/build)

Build-time Node requirements do not force WebContainers into the published app. Runtime Node requirements can. WebContainers therefore serve both authoring and execution use cases, depending on the artifact.

Arbitrary package selection cannot mean guaranteed execution of every npm package. WebContainers do not execute ordinary native addons or provide a general-purpose operating system. Compatible JavaScript or WASM implementations may be needed. Publishing must report unsupported dependencies and runtime failures clearly. [WebContainer limitations](https://webcontainers.io/guides/troubleshooting#cannot-load-native-addon-error)

WebContainer boot cost, memory use, cross-origin isolation and browser differences need a focused prototype. The documented API permits only one concurrently booted instance in its context. Concurrent Node micro-apps and the boundary between mutually untrusted projects therefore remain explicit design questions; separate directories in a shared filesystem do not isolate hostile code. [WebContainer API](https://webcontainers.io/api#boot), [browser requirements](https://webcontainers.io/guides/browser-support)

### WASM extension

Code compiled from other languages can be distributed as WASM where its compiler, runtime imports and host interfaces are compatible with the browser. A small host interface can expose computation inputs and outputs and explicitly permitted platform calls.

This is an extension path, not a promise that arbitrary existing programs can be compiled and executed unchanged. WASI specifies host interfaces; the selected browser runtime must actually implement the interfaces a program needs. The compilation toolchain and module contract remain open. [WASI overview](https://wasi.dev/)

## Library and publishing

The library stores source, manifests, dependency versions and published artifacts. Publishing creates an immutable version that can be reused, inspected and replaced by a later version. Installing it binds the code to the target App and Space.

A proposed publication flow is:

1. An author or agent generates or edits source using the relevant entity contract, SDK and component library.
2. The publisher resolves and pins dependencies in an isolated build environment.
3. Build and runtime checks validate the requested execution mode and declared platform capabilities.
4. Source and prepared artifacts are stored as a version in the library.
5. The version is installed into an App with its Space binding and integration grants.
6. The browser host loads that version from static hosting or prepared runtime artifacts.

Generated code should remain editable. A published version can be rolled back without regenerating it. Publication checks help identify defects and incompatibilities; they do not prove arbitrary code is trustworthy.

## Open source distribution

The framework will be open source. The Go backend can be distributed as a versioned Go module for developers, compiled release binaries for operators, and a container image for deployment. Go modules are published through repository version tags and consumed through the Go toolchain. A command can be installed using go install with a version suffix. These mechanisms do not require operating a custom package registry. [Go module publishing](https://go.dev/doc/modules/publishing), [Go command installation](https://go.dev/ref/mod#go-install)

The TypeScript SDK and React components can continue to use npm. The backend language does not determine the distribution format of client libraries. Start with one backend module and release artifact; additional separately versioned modules should follow a concrete reuse requirement. The public module path and open-source license remain to be selected.

Discovery needs its own plan. pkg.go.dev supports search over package names, paths, summaries and READMEs; curated channels such as Awesome Go and Go Weekly offer additional exposure to Go developers. For Lastapp's client-app authors, publish useful SDK and UI packages on npm and direct their documentation to the complete framework. A Go backend can therefore participate in both ecosystems. These are proposed promotion channels, not established acquisition results. [Go package search](https://pkg.go.dev/search-help), [Awesome Go](https://awesome-go.com/), [Go Weekly](https://golangweekly.com/), [npm package discovery](https://docs.npmjs.com/searching-for-and-choosing-packages-to-download/)

## Trust and client isolation

Both reviewed code and third-party code are supported. **Working proposal:** use an isolated frame as the common loading boundary for both, avoiding an initial split between direct host imports and isolated execution.

The browser host exposes a limited API bridge or narrowly scoped credentials. The backend verifies the resulting user, App and Space authority. Cross-frame messages require origin and sender validation and must be bound to the actual installed instance. Integration tokens stay on the server.

The exact sandbox, origin allocation, network policy and WebContainer embedding arrangement need validation together. An iframe alone is not a complete isolation design. Arbitrary code can read and potentially disclose data it has legitimately received, so App grants must limit what it can access. Untrusted projects must not share privileged execution state or build credentials.

The same rule applies to WebContainers and WASM: running on the user’s device does not make code a trusted backend or give it additional business permissions.

## Login and external integrations

### Platform identity

Users authenticate to the platform once. The host then provides authenticated access to installed Apps and micro-apps. Lastapp owns internal user IDs, Tenant and Space membership, and application permissions.

**Working proposal:** use direct Google and Microsoft OpenID Connect login through a mature server library, with platform sessions managed by our backend. External identities map to internal users using stable provider identifiers rather than email alone. Both providers support OIDC. [Google OIDC](https://developers.google.com/identity/openid-connect/openid-connect), [Microsoft OIDC](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc)

Auth0 remains an alternative if enterprise identity requirements justify a managed identity service. No identity vendor is selected.

### Connections to external systems

**Working proposal:** a backend integration gateway uses a connection service such as Nango for provider authorization, credential storage and refresh. A Space owns connections by default; App owners bind the connections their Apps need, subject to administrative permissions. Personal user connections can be supported where the external account is individual.

Several Apps can reuse a Space connection without each browser or micro-app performing its own OAuth setup. Multiple connections to the same provider are valid: for example, two ERP accounts or separate shared and personal mailboxes.

The backend checks the calling user, installed App, Space and permitted provider operations before using a connection. It resolves the actual credentials server-side. A connection ID is a reference, not an authorization credential. Provider scopes are an additional limit and do not replace Lastapp permissions.

The proxy can preserve provider-native APIs to support broad integration functionality. OAuth standardizes authorization; it does not normalize HubSpot and ERP data models. Grants, operation constraints and any required filtering of external results belong in the integration contract. Entity API permissions do not automatically filter a raw third-party response.

Nango’s documented model provides connections and managed credentials that the application associates with its own users, organizations or projects. It does not replace the platform’s login-session and membership model. Nango plus direct OIDC would avoid adding a separate managed login vendor. Auth0 plus Token Vault is another option, with different connection and proxy responsibilities. [Nango auth guide](https://nango.dev/docs/guides/auth/auth-guide), [Auth0 Token Vault](https://auth0.com/docs/secure/call-apis-on-users-behalf/token-vault)

## Decision status and remaining design work

The hierarchy and client/backend separation are settled. The table distinguishes the delivered backend from future client-hosting and integration decisions; current API guides take precedence over earlier proposals.

| Decision | Current proposal or unresolved point |
|---|---|
| Backend language | Settled: Go, with trusted platform code and declarative App definitions |
| Database placement | Settled: both Tenant-shared and Space-dedicated databases, controlled by configuration and subscription policy |
| Provisioning methods | Settled: managed creation and registration of existing databases |
| Administration | Settled: superadmins create Tenants; tenant administrators manage their Spaces and Apps |
| First database adapter | Implemented: MariaDB through the reviewed, pinned `aantich/go-mariadb` snapshot |
| Entity specification | Implemented: typed Go entity document; generated JSON Schema, TypeScript, storage and permission projections |
| Query contract | Implemented bounded AST, field selection, sorting, cursor paging/count and bulk writes; joins, aggregates and full-text/vector search deferred |
| Permission model | Implemented global identities/roles/scopes, entity/record/field ACLs, record shares and Space-bounded App access |
| Agent management | Implemented existing-user OAuth delegation, contextual typed MCP tools and console approval/revocation; robot accounts and record tools deferred |
| Shared design | Versioned components and themes; decide Tenant, Space and App overrides and update behavior |
| Client execution | Browser and WebContainer modes; validate isolation, supported browsers and concurrent Node projects |
| Publication policy | Define review expectations and capability grants for trusted and third-party code |
| Authentication and integrations | Direct OIDC plus Nango is a working option; vendor selection remains open |
| WASM modules | Preserve the extension path; choose a host interface and toolchain when specifying this capability |
| Open source release | Public repository/module: `github.com/aantich/lastapp`; license and release/public package packaging still need an explicit decision |

## Current foundation and next product work

The Go control plane and system database already record Tenants, Spaces, databases, Apps and micro-app installations. Global authorization, entity publication, the secured data API, a small browser SDK and management MCP are implemented. See the [maintainer guide](DEVELOPMENT.md) and [verification](VERIFICATION.md).

Next product directions are a versioned source/library/build/static publishing flow, shared micro-app components/design, an isolated browser host, optional Node/WASM execution, integration connections and App placement. Their boundaries still need design/approval; they do not require splitting the current service in advance. Shared/dedicated App runtime pools are a target, not implemented scheduling.

Prove that different Apps in the same Space can reuse stored data with coordinated shared invariants, that shared design survives independently generated code, and that both execution modes use the same host and API contract. Resolve runtime isolation and compatibility before treating WebContainer support as complete. Additional services and abstractions should follow demonstrated needs.

### Tenant publishing and a platform marketplace

Roadmap idea added 21 November 2026. The platform superadmin could enable package publishing per Tenant. A Tenant administrator or delegated publisher could package selected Space entities, rules and role definitions, with an optional App from a captured source and build. Submissions stay private until a platform superadmin approves the exact version and digest for the system-wide marketplace. Other Tenants then clone the approved version through the existing installer.

- **Export from a Tenant.** Convert selected configuration into portable references, resolve dependencies, and review what will become public. Export configuration and chosen App artifacts. Exclude records, assignments, credentials, and Tenant-specific identities and origins. [Package format](PACKAGES.md#publish-a-package)
- **Publisher identity and review.** Add a Tenant publishing entitlement and scoped publisher role. Give each publisher a stable namespace. Track immutable submissions, review feedback, approval and withdrawal with audit history. A superadmin reviews the submitted package and isolated preview through platform tools. Tenant membership and records stay under Tenant authority.
- **Review tenant-supplied source and builds.** Validate public metadata and bundled assets, scan for secrets and risky dependencies, review the pairing between build and source, and define safe preview and install boundaries for browser code. Review the installed App's permissions and outbound access. Approve dependencies explicitly and support emergency suspension independently from ordinary withdrawal.
- **Reuse installation and provenance.** Send approved versions to the existing catalog and installer. Pin installed versions, show publisher attribution, and preserve each Tenant's local edits when a listing changes. Ordinary withdrawal stops new installs. Emergency suspension of an already installed build needs its own policy.

Initial estimate is **2–3 engineer-weeks** for a curated objects-only pilot and **5–8 engineer-weeks total** to include Apps, source, and review tooling. This assumes a working package foundation, free listings, manual review, and builds supplied by publishers. Payments, hosted builds, and automatic upgrades are separate work.
