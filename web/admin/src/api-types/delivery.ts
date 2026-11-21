import type { Item } from "./loose";

// internal/control/artifacts.go: upload ticket; internal/artifact/store.go: Limits.
export type UploadTicket = {
  upload_url: string;
  upload_token: string;
  expires_in: number;
  limits: Record<
    | "max_bytes"
    | "max_file_bytes"
    | "max_files"
    | "max_body_bytes"
    | "max_source_bytes",
    number
  >;
};

// internal/control/artifacts.go: Build.
export type Build = {
  id: string;
  content_hash: string;
  size_bytes: number;
  created_at: string;
  entrypoint: string;
};

// internal/inventory/inventory.go: DeploymentStatus, an App list item's
// latest deployment (present only where the reader may manage its
// releases). failed_state is the step a failed deployment had reached.
export type DeploymentSummary = {
  id: string;
  state: string;
  failed_state: string | null;
  error_message: string | null;
  release_id: string;
  release_name: string;
  deployed_by: string;
  created_at: string;
  finished_at: string | null;
};

// internal/delivery/releases.go: Deployment. deployed_by is the person who
// started it (an agent acts for them); release_name the release's name;
// attempts counts its runs and retried_by names who last retried it.
export type Deployment = DeploymentSummary & {
  url: string;
  revision: number;
  attempts: number;
  retried_by: string | null;
};

// internal/delivery/workspaces.go: WorkspaceItem (GET
// /v1/spaces/{id}/workspaces). preparation_step is the WorkspaceSteps step a
// preparing workspace has reached (or a failed one failed at);
// agent_client_name names the agent client that owns an agent workspace.
export type WorkspaceItem = {
  id: string;
  display_name: string;
  backing_space_id: string;
  production_space_id: string;
  owner_agent_grant_id: string | null;
  agent_client_name: string | null;
  preparation_step: string | null;
  error_message: string | null;
  state: string;
  revision: number;
  created_at: string;
};

// internal/delivery/releases.go: ReleaseItem (GET /v1/apps/{id}/releases):
// who captured it, and the agent client that owns its workspace, if any.
export type ReleaseItem = {
  id: string;
  display_name: string;
  build_id: string;
  created_at: string;
  created_by_name: string;
  agent_client_name: string | null;
  snapshot_hash: string;
};

// internal/control/releases.go: Release (GET /v1/releases/{id}). The manifest
// pins micro-app and package versions; the snapshot is
// internal/control/development_workspaces.go: spaceSnapshot, whose entity
// definitions are internal/entity/model.go: Definition (fields read loosely).
export type ReleaseManifest = {
  microapps?: Record<string, string>;
  dependencies?: Record<string, string>;
};
export type Release = {
  id: string;
  display_name: string;
  build_id: string;
  created_at: string;
  production_app_id: string;
  manifest_json: ReleaseManifest | null;
  snapshot_json: {
    entities: {
      id: string;
      definition: { label: string; fields: Item[]; permissions: unknown[] };
    }[];
    roles: unknown[];
    apps: unknown[];
  };
};
