# Starter business packages

The package catalog installs tenant-owned configuration into an existing production Space. Create a Space with the ordinary Spaces flow when an installation needs its own data boundary. A Tenant administrator or authorized Space manager can review and stage a package. The worker publishes the reviewed changes after explicit production activation.

## Publish a package

A platform operator publishes through the server CLI with a registry and artifact directory shared with the running service. Use the package's reviewed CI output for the build and source arguments.

Tenant publishing and platform review are [future marketplace work](CONCEPT.md#tenant-publishing-and-a-platform-marketplace).

```sh
lastapp publish-package -config /path/to/server.json -manifest manifest.json
lastapp publish-package -config /path/to/server.json -manifest manifest.json \
  -build dist -source source.zip
```

The manifest has `format: 1`, a `key`, a `version`, a `name`, optional `description` and `requires`, and the `entities` and `roles` arrays. Use canonical entity definition fields. A reference field names an entity by key. A permission's role list names Space business roles by key. The optional `dependencies` map pins an already installed prerequisite to an exact version. A reference to its entity uses `package_key/entity_key`.

An optional `app` declares its `key`, `name`, `microapps`, optional App `roles`, and explicit `access` ceilings. A ceiling or restriction names a permission as `entity_key/permission_key`. App role gates use `app/role_key`. Each micro-app has a key, name, and exact version. `launch_roles` controls discovery in **My Apps**. Its default is the package's declared role set. Actual records follow the App access policy and the caller's live role assignments.

Supported capability identifiers are `entity-v1`, `app-sdk-v1`, and `static-release-v1`. Choose one immutable version per released manifest, source, and build combination. An identical republish returns its existing digest. A changed package needs a new version. Use `lastapp retire-package -package KEY -version VERSION` to remove it from new installation choices while retaining the source and build for existing installations. The reader verifies the pinned digest. The platform CLI is the catalog publishing boundary.

For a full package, build the App in CI from the exact source archive and lockfile. The runtime verifies the build file manifest and source digest. Include `package.json` and `package-lock.json` in the ZIP root. Keep credentials, tenant settings, `node_modules`, and `lastapp.local.json` outside that archive. A browser build obtains its destination App ID and API origin from `/_lastapp/config.json`.

## Install and customize

1. Open **Packages**, choose a version and Space, and review either **Business objects** or **Business objects and App**. The review includes resources and any existing Apps that inherit Space permissions.
2. Stage the reviewed plan. Wait for workspace readiness, then prepare and optionally open the App preview. Preview runs against the isolated workspace and its own App session. Publish production changes with **Activate**.
3. Assign starter roles to local users or teams under **Set up access**. A member with a discovery role can find the App in **My Apps**. Assignments and users stay under Tenant control.
4. Use **Add App** on an objects-only installation to attach the supplied client to the current Space contracts. An incompatible field or permission path appears as a planning conflict.
5. Use **Customize** on the installed App to open the Space's development workspaces. In the workspace's App, download the effective source and the workspace configuration. Build and test locally. In Releases, choose **Attach source ZIP**, then upload the build folder. An upload ticket accepts the source ZIP alongside the static files. The starter upload command takes it after the build folder and upload URL.

```sh
node scripts/upload.mjs dist "$LASTAPP_UPLOAD_URL" source.zip
```

Set the short-lived `LASTAPP_UPLOAD_TOKEN` in the shell before running the command. The upload JSON's `source_zip` is base64. Capture, preview, and deploy the resulting build through the ordinary release flow. Authorized managers can download the source attached to the selected release. Workspace source is available under the corresponding workspace and delegation boundary. A valid build-only release reports source unavailable.

Packages add selected resources. A key or slug with unrelated ownership is a conflict. An existing prerequisite is resolved in the destination Space. Installation provides neither role assignments nor records. After a stale review, archive and discard an unused stage, then plan from the current production state. A manager with production deployment authority can retry a failed installation, including after the original workspace is archived. If the recorded base changed or is unavailable, review the retry against the current Space before activating again. The review checks the original resource reservations. The operation's status retains the failed step and latest attempt.

## Management MCP

Use `lastapp_package_read` for `catalog`, `inspect`, `plan`, `stages`, `status`, `retry_plan`, or `source`. Use `lastapp_package` for `stage`, `preview`, `activate`, `retry`, or `discard`. The plan returns a `plan_hash` for the stage input. The stage ID is its `workspace_id`. Production activation uses the usual `production.deploy` grant. Source download URLs require a valid MCP bearer token and recheck the live release access on each request. Use the normal workspace and entity tools to customize a clone. Business records remain behind the App Data API.

## Implementation

The package service owns the catalog and plan. Delivery owns workspace staging, attachments, and activation. The existing worker shares the publication queue and uses the same storage gates for ordinary releases and package operations. A deployment kind and constrained optional client references support objects-only commits. Existing releases preserve their IDs and status API.

Package operations have fixed manifest, dependency, item and upload limits. Each stage uses an isolated managed database under the Space's separate package preview capacity. Planning checks its capacity and target eligibility. Archiving a package preview schedules deletion of its test database and generated account, and returns its preview capacity after cleanup succeeds. Production resources and release history are retained. Package install identity is unique per Space and package key. Source and build bytes are pinned by digest. The server exposes no unauthenticated source directory.

Operators can run `lastapp prune-package-artifacts -config /path/to/server.json`
to remove staging files and unreferenced artifacts older than 24 hours. The
operation rechecks catalog, build, and release references under each artifact's
lock. All processes sharing an artifact directory must use the same registry.

Stop workers before applying registry migrations 16–17 and restart with the new
binary. Migration 17 classifies existing package preview databases. A previously
archived preview is scheduled for cleanup when explicitly discarded or archived again.
The managed target credentials need permission to remove the generated preview
databases and accounts. Cleanup waits while the target is disabled.
