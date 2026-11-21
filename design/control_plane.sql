-- Lastapp provisioning schema: final phase-one design, MariaDB / InnoDB.
-- Design DDL for an empty database, not an upgrade migration from the earlier draft.
-- Apply to an EMPTY, explicitly selected control-plane database.
-- No databases/accounts are created and no tenant business data is modified.
-- All service connections and migration sessions use UTC.
SET time_zone = '+00:00';

CREATE TABLE users (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    oidc_issuer VARCHAR(255) COLLATE utf8mb4_nopad_bin NOT NULL,
    oidc_subject VARCHAR(255) COLLATE utf8mb4_nopad_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    email VARCHAR(320) NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_identity (oidc_issuer, oidc_subject),
    CONSTRAINT ck_users_state CHECK (state IN ('active', 'disabled')),
    CONSTRAINT ck_users_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE sessions (
    token_hash BINARY(32) NOT NULL,
    user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    csrf_hash BINARY(32) NOT NULL,
    purpose VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'console',
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY (token_hash),
    KEY ix_sessions_user (user_id),
    KEY ix_sessions_expiry (expires_at),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT ck_sessions_purpose CHECK (purpose = 'console' AND state = 'active'),
    CONSTRAINT ck_sessions_expiry CHECK (expires_at > created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE plans (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    allow_shared_database TINYINT UNSIGNED NOT NULL,
    allow_dedicated_database TINYINT UNSIGNED NOT NULL,
    allow_managed_database TINYINT UNSIGNED NOT NULL,
    allow_registered_database TINYINT UNSIGNED NOT NULL,
    allow_dedicated_compute TINYINT UNSIGNED NOT NULL DEFAULT 0,
    default_layout VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    max_spaces INT UNSIGNED NULL,
    max_apps INT UNSIGNED NULL,
    max_databases INT UNSIGNED NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_plans_code (code),
    CONSTRAINT ck_plan_revision CHECK (revision > 0),
    CONSTRAINT ck_plan_flags CHECK (
        allow_shared_database IN (0, 1)
        AND allow_dedicated_database IN (0, 1) AND allow_managed_database IN (0, 1)
        AND allow_registered_database IN (0, 1) AND allow_dedicated_compute IN (0, 1)
    ),
    CONSTRAINT ck_plan_layouts CHECK (allow_shared_database = 1 OR allow_dedicated_database = 1),
    CONSTRAINT ck_plan_methods CHECK (allow_managed_database = 1 OR allow_registered_database = 1),
    CONSTRAINT ck_plan_default CHECK (
        (default_layout = 'shared' AND allow_shared_database = 1)
        OR (default_layout = 'dedicated' AND allow_dedicated_database = 1)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE tenants (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    plan_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    default_layout VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NULL,
    default_shared_database_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    default_shared_scope VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'shared',
    created_by CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_tenants_slug (slug),
    KEY ix_tenants_plan (plan_id),
    CONSTRAINT fk_tenants_plan FOREIGN KEY (plan_id) REFERENCES plans (id),
    CONSTRAINT fk_tenants_creator FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT ck_tenants_state CHECK (state IN ('active', 'suspended', 'archived')),
    CONSTRAINT ck_tenants_default CHECK (default_layout IS NULL OR default_layout IN ('shared', 'dedicated')),
    CONSTRAINT ck_tenants_shared_scope CHECK (default_shared_scope = 'shared'),
    CONSTRAINT ck_tenants_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE tenant_memberships (
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (tenant_id, user_id),
    KEY ix_memberships_user (user_id, state, tenant_id),
    CONSTRAINT fk_memberships_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_memberships_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT ck_memberships_state CHECK (state IN ('active', 'disabled')),
    CONSTRAINT ck_memberships_revision CHECK (revision>0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE database_targets (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    canonical_host VARCHAR(253) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    port SMALLINT UNSIGNED NOT NULL DEFAULT 3306,
    tls_mode VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'verify_identity',
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    managed_enabled TINYINT UNSIGNED NOT NULL DEFAULT 0,
    provisioning_credentials_enc BLOB NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_targets_endpoint (canonical_host, port),
    CONSTRAINT ck_targets_port CHECK (port > 0),
    CONSTRAINT ck_targets_tls CHECK (tls_mode IN ('verify_identity', 'disabled')),
    CONSTRAINT ck_targets_state CHECK (state IN ('active', 'disabled')),
    CONSTRAINT ck_targets_managed CHECK (
        managed_enabled IN (0, 1) AND (managed_enabled = 0 OR provisioning_credentials_enc IS NOT NULL)
    ),
    CONSTRAINT ck_targets_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE database_resources (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    target_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    database_name VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
    database_name_key VARCHAR(64) COLLATE utf8mb4_bin
        GENERATED ALWAYS AS (LOWER(database_name)) PERSISTENT,
    management_mode VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    layout VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
    runtime_credentials_enc BLOB NOT NULL,
    ownership_token BINARY(16) NULL,
    attempts INT UNSIGNED NOT NULL DEFAULT 0,
    last_error_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    last_error_message VARCHAR(500) NULL,
    last_verified_at DATETIME(6) NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_databases_tenant_scope (tenant_id, id, layout),
    UNIQUE KEY uq_databases_tenant (tenant_id, id),
    UNIQUE KEY uq_databases_physical (target_id, database_name_key),
    KEY ix_databases_inventory (tenant_id, state, id),
    KEY ix_databases_work (state, created_at, id),
    CONSTRAINT fk_databases_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_databases_target FOREIGN KEY (target_id) REFERENCES database_targets (id),
    CONSTRAINT ck_databases_mode CHECK (management_mode IN ('managed', 'registered')),
    CONSTRAINT ck_databases_layout CHECK (layout IN ('shared', 'dedicated')),
    CONSTRAINT ck_databases_state CHECK (state IN ('pending', 'provisioning', 'ready', 'failed', 'retired')),
    CONSTRAINT ck_databases_name CHECK (CHAR_LENGTH(database_name) > 0),
    CONSTRAINT ck_databases_ownership CHECK (
        (management_mode = 'managed' AND ownership_token IS NOT NULL)
        OR (management_mode = 'registered' AND ownership_token IS NULL)
    ),
    CONSTRAINT ck_databases_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE tenants ADD CONSTRAINT fk_tenants_shared_database
    FOREIGN KEY (id, default_shared_database_id, default_shared_scope)
    REFERENCES database_resources (tenant_id, id, layout);

CREATE TABLE spaces (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    database_resource_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    database_layout VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    dedicated_database_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin
        GENERATED ALWAYS AS (
            CASE WHEN database_layout = 'dedicated' THEN RTRIM(database_resource_id) ELSE NULL END
        ) PERSISTENT,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_spaces_tenant (tenant_id, id),
    UNIQUE KEY uq_spaces_slug (tenant_id, slug),
    UNIQUE KEY uq_spaces_dedicated_database (dedicated_database_id),
    KEY ix_spaces_database (tenant_id, database_resource_id, database_layout),
    CONSTRAINT fk_spaces_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_spaces_database FOREIGN KEY (tenant_id, database_resource_id, database_layout)
        REFERENCES database_resources (tenant_id, id, layout),
    CONSTRAINT ck_spaces_layout CHECK (database_layout IN ('shared', 'dedicated')),
    CONSTRAINT ck_spaces_state CHECK (state IN ('active', 'suspended', 'archived')),
    CONSTRAINT ck_spaces_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE apps (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    compute_mode VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'shared',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_apps_tenant (tenant_id, id),
    UNIQUE KEY uq_apps_space (tenant_id, space_id, id),
    UNIQUE KEY uq_apps_slug (space_id, slug),
    KEY ix_apps_inventory (tenant_id, state, id),
    CONSTRAINT fk_apps_space FOREIGN KEY (tenant_id, space_id) REFERENCES spaces (tenant_id, id),
    CONSTRAINT ck_apps_state CHECK (state IN ('active', 'suspended', 'archived')),
    CONSTRAINT ck_apps_compute CHECK (compute_mode IN ('shared', 'dedicated')),
    CONSTRAINT ck_apps_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE microapp_installations (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    artifact_ref VARCHAR(1024) COLLATE utf8mb4_bin NULL,
    navigation_order INT NOT NULL DEFAULT 0,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_microapps_slug (app_id, slug),
    KEY ix_microapps_navigation (tenant_id, app_id, navigation_order, id),
    CONSTRAINT fk_microapps_app FOREIGN KEY (tenant_id, app_id) REFERENCES apps (tenant_id, id),
    CONSTRAINT ck_microapps_state CHECK (state IN ('active', 'suspended', 'archived')),
    CONSTRAINT ck_microapps_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE audit_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    actor_user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    action VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    resource_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    resource_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    details_json JSON NOT NULL DEFAULT '{}',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY ix_audit_tenant (tenant_id, id),
    KEY ix_audit_resource (resource_type, resource_id, id),
    CONSTRAINT fk_audit_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_audit_actor FOREIGN KEY (actor_user_id) REFERENCES users (id),
    CONSTRAINT ck_audit_details CHECK (JSON_TYPE(details_json) = 'OBJECT')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Shared authorization catalog. New Tenants add rows, never new auth tables.
CREATE TABLE groups (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_groups_tenant (tenant_id, id),
    UNIQUE KEY uq_groups_slug (tenant_id, slug),
    CONSTRAINT fk_groups_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT ck_groups_state CHECK (state IN ('active', 'disabled')),
    CONSTRAINT ck_groups_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE group_memberships (
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    CONSTRAINT ck_group_membership_state CHECK (state IN ('active','disabled')),
    PRIMARY KEY (tenant_id, group_id, user_id),
    KEY ix_group_memberships_user (tenant_id, user_id, group_id),
    CONSTRAINT fk_group_memberships_group FOREIGN KEY (tenant_id, group_id)
        REFERENCES groups (tenant_id, id),
    CONSTRAINT fk_group_memberships_member FOREIGN KEY (tenant_id, user_id)
        REFERENCES tenant_memberships (tenant_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE permissions (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    code VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    domain VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'control',
    description VARCHAR(160) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_permissions_code (code),
    UNIQUE KEY uq_permissions_domain (id, domain),
    CONSTRAINT ck_permissions_domain CHECK (domain='control')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE roles (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    owner_key VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    scope_kind VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    domain VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_roles_code (owner_key,code),
    UNIQUE KEY uq_roles_domain (id,domain),
    UNIQUE KEY uq_roles_binding (owner_key,id,scope_kind,domain),
    CONSTRAINT fk_roles_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_roles_space FOREIGN KEY (tenant_id,space_id) REFERENCES spaces(tenant_id,id),
    CONSTRAINT fk_roles_app FOREIGN KEY (tenant_id,space_id,app_id) REFERENCES apps(tenant_id,space_id,id),
    CONSTRAINT ck_roles_scope CHECK (scope_kind IN ('platform','tenant','team','space','app')),
    CONSTRAINT ck_roles_domain CHECK (domain IN ('control','data')),
    CONSTRAINT ck_roles_owner CHECK (
      (domain='control' AND owner_key='builtin' AND tenant_id IS NULL AND space_id IS NULL AND app_id IS NULL)
      OR (domain='data' AND tenant_id IS NOT NULL AND space_id IS NOT NULL AND
          ((scope_kind='space' AND app_id IS NULL AND owner_key=CONCAT('s:',RTRIM(space_id)))
           OR (scope_kind='app' AND app_id IS NOT NULL AND owner_key=CONCAT('a:',RTRIM(app_id)))))
    ),
    CONSTRAINT ck_roles_state CHECK (state IN ('active','disabled')),
    CONSTRAINT ck_roles_revision CHECK (revision>0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE role_permissions (
    role_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    permission_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    domain VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    PRIMARY KEY (role_id,permission_id),
    CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id,domain) REFERENCES roles(id,domain),
    CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id,domain) REFERENCES permissions(id,domain)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One recipient, one role, one typed assignment scope. No direct permissions.
CREATE TABLE access_grants (
    id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    scope_kind VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    domain VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    group_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    scope_team_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
    role_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    role_owner_key VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    equivalence_key VARCHAR(400) CHARACTER SET ascii COLLATE ascii_bin
      GENERATED ALWAYS AS (CONCAT_WS('|',scope_kind,COALESCE(RTRIM(tenant_id),''),COALESCE(RTRIM(user_id),''),COALESCE(RTRIM(group_id),''),COALESCE(RTRIM(scope_team_id),''),COALESCE(RTRIM(space_id),''),COALESCE(RTRIM(app_id),''),RTRIM(role_id))) PERSISTENT,
    PRIMARY KEY (id),
    UNIQUE KEY uq_grant_equivalence (equivalence_key),
    KEY ix_grants_user (user_id,tenant_id),
    KEY ix_grants_group (tenant_id,group_id),
    KEY ix_grants_scope (tenant_id,scope_kind,scope_team_id,space_id,app_id),
    CONSTRAINT fk_grants_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_grants_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_grants_member FOREIGN KEY (tenant_id,user_id) REFERENCES tenant_memberships(tenant_id,user_id),
    CONSTRAINT fk_grants_group FOREIGN KEY (tenant_id,group_id) REFERENCES groups(tenant_id,id),
    CONSTRAINT fk_grants_team_member FOREIGN KEY (tenant_id,scope_team_id,user_id) REFERENCES group_memberships(tenant_id,group_id,user_id),
    CONSTRAINT fk_grants_space FOREIGN KEY (tenant_id,space_id) REFERENCES spaces(tenant_id,id),
    CONSTRAINT fk_grants_app FOREIGN KEY (tenant_id,app_id) REFERENCES apps(tenant_id,id),
    CONSTRAINT fk_grants_role FOREIGN KEY (role_owner_key,role_id,scope_kind,domain) REFERENCES roles(owner_key,id,scope_kind,domain),
    CONSTRAINT ck_grants_subject CHECK ((user_id IS NOT NULL AND group_id IS NULL) OR (user_id IS NULL AND group_id IS NOT NULL)),
    CONSTRAINT ck_grants_control_subject CHECK (domain='data' OR (domain='control' AND user_id IS NOT NULL AND group_id IS NULL)),
    CONSTRAINT ck_grants_scope CHECK (
      (scope_kind='platform' AND tenant_id IS NULL AND user_id IS NOT NULL AND group_id IS NULL AND scope_team_id IS NULL AND space_id IS NULL AND app_id IS NULL)
      OR (scope_kind='tenant' AND tenant_id IS NOT NULL AND scope_team_id IS NULL AND space_id IS NULL AND app_id IS NULL)
      OR (scope_kind='team' AND tenant_id IS NOT NULL AND user_id IS NOT NULL AND group_id IS NULL AND scope_team_id IS NOT NULL AND space_id IS NULL AND app_id IS NULL)
      OR (scope_kind='space' AND tenant_id IS NOT NULL AND scope_team_id IS NULL AND space_id IS NOT NULL AND app_id IS NULL)
      OR (scope_kind='app' AND tenant_id IS NOT NULL AND scope_team_id IS NULL AND space_id IS NULL AND app_id IS NOT NULL)
    ),
    CONSTRAINT ck_grants_owner CHECK (
      (domain='control' AND role_owner_key='builtin')
      OR (domain='data' AND ((scope_kind='space' AND role_owner_key=CONCAT('s:',RTRIM(space_id))) OR (scope_kind='app' AND role_owner_key=CONCAT('a:',RTRIM(app_id)))))
    ),
    CONSTRAINT ck_grants_revision CHECK (revision>0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The fixed control catalog is installed from internal/authz/catalog.go by the
-- trusted bootstrap command. Custom data roles cannot reference control actions.

CREATE TABLE local_accounts (
    user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    login_email VARCHAR(320) COLLATE utf8mb4_nopad_bin NOT NULL,
    password_hash BINARY(32) NOT NULL,
    password_salt BINARY(16) NOT NULL,
    password_iterations INT UNSIGNED NOT NULL,
    must_change_password TINYINT UNSIGNED NOT NULL DEFAULT 1,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id),
    UNIQUE KEY uq_local_login_email (login_email),
    CONSTRAINT fk_local_account_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT ck_local_password_iterations CHECK (password_iterations=600000),
    CONSTRAINT ck_local_password_change CHECK (must_change_password IN (0,1)),
    CONSTRAINT ck_local_revision CHECK (revision>0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE sessions ADD COLUMN auth_method VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'oidc',
    ADD CONSTRAINT ck_session_auth_method CHECK (auth_method IN ('oidc','password'));

CREATE TABLE entity_definitions (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 entity_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 label VARCHAR(160) NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'active',
 draft_json JSON NOT NULL,
 revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
 active_version_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 pending_version_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 UNIQUE KEY uq_entity_key(space_id,entity_key),
 UNIQUE KEY uq_entity_owner(tenant_id,space_id,id),
 CONSTRAINT fk_entity_space FOREIGN KEY(tenant_id,space_id) REFERENCES spaces(tenant_id,id),
 CONSTRAINT ck_entity_state CHECK(state IN ('active','archived')),
 CONSTRAINT ck_entity_revision CHECK(revision>0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE entity_versions (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 entity_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 base_version_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 definition_json JSON NOT NULL,
 definition_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'queued',
 attempt_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 error_message VARCHAR(500) NULL,
 created_by CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 finished_at DATETIME(6) NULL,
 KEY ix_publication_work(state,created_at),
 UNIQUE KEY uq_version_owner(entity_id,id),
 CONSTRAINT fk_version_entity FOREIGN KEY(tenant_id,space_id,entity_id) REFERENCES entity_definitions(tenant_id,space_id,id),
 CONSTRAINT fk_version_author FOREIGN KEY(created_by) REFERENCES users(id),
 CONSTRAINT ck_version_state CHECK(state IN ('queued','publishing','published','failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE entity_definitions
 ADD CONSTRAINT fk_entity_active FOREIGN KEY(id,active_version_id) REFERENCES entity_versions(entity_id,id),
 ADD CONSTRAINT fk_entity_pending FOREIGN KEY(id,pending_version_id) REFERENCES entity_versions(entity_id,id);
ALTER TABLE permissions DROP CONSTRAINT ck_permissions_domain,
 DROP INDEX uq_permissions_code,
 ADD COLUMN owner_key VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'builtin',
 ADD COLUMN tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN entity_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN version_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN security_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN security_revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
 ADD COLUMN state VARCHAR(16) NOT NULL DEFAULT 'active',
 ADD UNIQUE KEY uq_permission_owner(owner_key,code),
 ADD CONSTRAINT fk_permission_entity FOREIGN KEY(tenant_id,space_id,entity_id) REFERENCES entity_definitions(tenant_id,space_id,id),
 ADD CONSTRAINT fk_permission_version FOREIGN KEY(entity_id,version_id) REFERENCES entity_versions(entity_id,id),
 ADD CONSTRAINT ck_permissions_domain CHECK((domain='control' AND owner_key='builtin' AND entity_id IS NULL AND tenant_id IS NULL AND space_id IS NULL AND version_id IS NULL) OR (domain='data' AND entity_id IS NOT NULL AND tenant_id IS NOT NULL AND space_id IS NOT NULL AND version_id IS NOT NULL AND owner_key=CONCAT('e:',RTRIM(entity_id)))),
 ADD CONSTRAINT ck_permission_state CHECK(state IN ('active','disabled'));
ALTER TABLE apps ADD COLUMN data_access_json JSON NOT NULL DEFAULT '{"origin":"","inherit":false,"ceiling":[],"restricted":false,"restrictions":[]}',
 ADD COLUMN origin_generation BIGINT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE database_resources ADD COLUMN schema_credentials_enc BLOB NULL,
 ADD COLUMN credential_generation BIGINT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE sessions DROP CONSTRAINT ck_sessions_purpose,
 ADD COLUMN parent_hash BINARY(32) NULL,
 ADD COLUMN tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN origin VARCHAR(500) NULL,
 ADD COLUMN origin_generation BIGINT UNSIGNED NULL,
 ADD COLUMN challenge VARCHAR(128) NULL,
 ADD COLUMN nonce VARCHAR(128) NULL,
 ADD KEY ix_session_parent(parent_hash),
 ADD CONSTRAINT fk_session_app FOREIGN KEY(tenant_id,space_id,app_id) REFERENCES apps(tenant_id,space_id,id),
 ADD CONSTRAINT ck_sessions_purpose CHECK((purpose='console' AND state='active' AND parent_hash IS NULL AND app_id IS NULL) OR (purpose IN ('launch','app') AND state IN ('active','consumed') AND parent_hash IS NOT NULL AND app_id IS NOT NULL AND origin IS NOT NULL AND origin_generation IS NOT NULL));

ALTER TABLE entity_versions ADD COLUMN compiler_version INT UNSIGNED NOT NULL DEFAULT 1,
 ADD COLUMN storage_plan_json JSON NULL,
 ADD COLUMN storage_plan_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL;
ALTER TABLE apps ADD COLUMN approved_origin VARCHAR(500) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN origin_host VARCHAR(253) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD UNIQUE KEY uq_app_origin_host(origin_host),
 ADD KEY ix_app_origin(approved_origin);

CREATE TABLE agent_grants (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 client_id VARCHAR(500) COLLATE utf8mb4_bin NOT NULL,
 client_name VARCHAR(160) NOT NULL,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 scope_kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 scope_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 permissions_json JSON NOT NULL,
 limits_json JSON NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
 revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 expires_at DATETIME(6) NOT NULL,
 last_used_at DATETIME(6) NULL,
 KEY ix_agent_owner(user_id,state), KEY ix_agent_tenant(tenant_id,state),
 FOREIGN KEY(user_id) REFERENCES users(id), FOREIGN KEY(tenant_id) REFERENCES tenants(id),
 CHECK(scope_kind IN ('platform','tenant','team','space','app')),
 CHECK((scope_kind='platform' AND tenant_id IS NULL AND scope_id IS NULL) OR
       (scope_kind='tenant' AND tenant_id IS NOT NULL AND scope_id IS NULL) OR
       (scope_kind IN ('team','space','app') AND tenant_id IS NOT NULL AND scope_id IS NOT NULL)),
 CHECK(state IN ('active','revoked') AND revision>0 AND expires_at>created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE agent_oauth_requests (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 client_id VARCHAR(500) COLLATE utf8mb4_bin NOT NULL,
 client_name VARCHAR(160) NOT NULL,
 redirect_uri VARCHAR(1500) COLLATE utf8mb4_bin NOT NULL,
 challenge VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 state_value VARCHAR(1024) COLLATE utf8mb4_bin NOT NULL,
 scopes VARCHAR(500) NOT NULL,
 grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 code_hash BINARY(32) NULL UNIQUE,
 consumed TINYINT UNSIGNED NOT NULL DEFAULT 0,
 expires_at DATETIME(6) NOT NULL,
 FOREIGN KEY(grant_id) REFERENCES agent_grants(id),
 KEY ix_agent_request_expiry(expires_at), CHECK(consumed IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE agent_token_sessions (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 grant_revision BIGINT UNSIGNED NOT NULL,
 family_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 access_hash BINARY(32) NOT NULL UNIQUE,
 refresh_hash BINARY(32) NOT NULL UNIQUE,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'active',
 access_expires_at DATETIME(6) NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 FOREIGN KEY(grant_id) REFERENCES agent_grants(id),
 KEY ix_agent_tokens_grant(grant_id,state), KEY ix_agent_tokens_family(family_id),
 CHECK(state IN ('active','rotated','revoked'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE agent_operation_receipts (
 grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_key CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 input_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 operation VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_json JSON NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 PRIMARY KEY(grant_id,request_key), FOREIGN KEY(grant_id) REFERENCES agent_grants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE entity_versions ADD COLUMN agent_grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD CONSTRAINT fk_entity_agent FOREIGN KEY(agent_grant_id) REFERENCES agent_grants(id);
ALTER TABLE database_resources ADD COLUMN agent_grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN agent_user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD CONSTRAINT fk_database_agent FOREIGN KEY(agent_grant_id) REFERENCES agent_grants(id);

-- Earlier development grants have no audience and must be re-approved. Never
-- infer an OAuth resource from a client-controlled request or backfill a host.
ALTER TABLE agent_grants ADD COLUMN audience_uri VARCHAR(500) COLLATE utf8mb4_bin NOT NULL DEFAULT '';
ALTER TABLE agent_oauth_requests ADD COLUMN resource_uri VARCHAR(500) COLLATE utf8mb4_bin NOT NULL DEFAULT '';
UPDATE agent_grants SET state='revoked',revision=revision+1 WHERE audience_uri='' AND state='active';

-- Public client registration is an identity/redirect record, never a grant.
CREATE TABLE agent_oauth_clients (
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 metadata_json JSON NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 last_used_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 KEY ix_agent_client_idle(last_used_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE INDEX ix_agent_client_active ON agent_grants(client_id,state,expires_at);

-- Bounded cleanup must preserve clients with an in-progress consent/code flow.
CREATE INDEX ix_agent_request_client ON agent_oauth_requests(client_id,consumed,expires_at);

CREATE TABLE development_workspaces (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 production_space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 backing_space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 owner_user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 owner_agent_grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 display_name VARCHAR(160) NOT NULL,
 base_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 base_json JSON NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'preparing',
 error_message VARCHAR(500) NULL,
 revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 KEY ix_workspace_owner(owner_user_id,state),
 FOREIGN KEY(tenant_id,production_space_id) REFERENCES spaces(tenant_id,id),
 FOREIGN KEY(tenant_id,backing_space_id) REFERENCES spaces(tenant_id,id),
 FOREIGN KEY(owner_user_id) REFERENCES users(id),
 FOREIGN KEY(owner_agent_grant_id) REFERENCES agent_grants(id),
 CHECK(state IN ('preparing','ready','failed','archived') AND revision>0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE spaces ADD COLUMN workspace_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD CONSTRAINT fk_space_workspace FOREIGN KEY(workspace_id) REFERENCES development_workspaces(id);
CREATE TABLE workspace_items (
 workspace_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 development_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 production_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 PRIMARY KEY(workspace_id,kind,development_id),
 UNIQUE KEY uq_workspace_production(workspace_id,kind,production_id),
 FOREIGN KEY(workspace_id) REFERENCES development_workspaces(id),
 CHECK(kind IN ('entity','role','app','microapp'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE sessions DROP CONSTRAINT ck_sessions_purpose,
 ADD COLUMN agent_grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN agent_grant_revision BIGINT UNSIGNED NULL,
 ADD CONSTRAINT fk_session_agent FOREIGN KEY(agent_grant_id) REFERENCES agent_grants(id),
 ADD CONSTRAINT ck_sessions_purpose CHECK(
 (purpose='console' AND state='active' AND parent_hash IS NULL AND app_id IS NULL AND agent_grant_id IS NULL) OR
 (purpose IN ('launch','app') AND state IN ('active','consumed') AND app_id IS NOT NULL AND origin IS NOT NULL AND origin_generation IS NOT NULL AND
 ((parent_hash IS NOT NULL AND agent_grant_id IS NULL) OR (parent_hash IS NULL AND agent_grant_id IS NOT NULL AND agent_grant_revision IS NOT NULL))));

CREATE TABLE app_builds (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 content_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 files_json JSON NOT NULL,
 entrypoint VARCHAR(240) NOT NULL,
 size_bytes BIGINT UNSIGNED NOT NULL,
 created_by CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 UNIQUE KEY uq_build_app(app_id,id),
 FOREIGN KEY(app_id) REFERENCES apps(id), FOREIGN KEY(created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE artifact_uploads (
 token_hash BINARY(32) PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 agent_grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 agent_grant_revision BIGINT UNSIGNED NULL,
 build_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 expires_at DATETIME(6) NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 FOREIGN KEY(app_id) REFERENCES apps(id), FOREIGN KEY(user_id) REFERENCES users(id),
 FOREIGN KEY(agent_grant_id) REFERENCES agent_grants(id), FOREIGN KEY(build_id) REFERENCES app_builds(id),
 KEY ix_upload_expiry(expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE app_releases (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 build_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 workspace_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 display_name VARCHAR(160) NOT NULL,
 manifest_json JSON NOT NULL,
 snapshot_json JSON NOT NULL,
 snapshot_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_by CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 UNIQUE KEY uq_release_app(app_id,id),
 FOREIGN KEY(app_id,build_id) REFERENCES app_builds(app_id,id),
 FOREIGN KEY(workspace_id) REFERENCES development_workspaces(id), FOREIGN KEY(created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE apps ADD COLUMN active_release_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD CONSTRAINT fk_app_release FOREIGN KEY(active_release_id) REFERENCES app_releases(id);
CREATE TABLE release_deployments (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 release_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 target_app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_by CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 agent_grant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 agent_grant_revision BIGINT UNSIGNED NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'queued',
 error_message VARCHAR(500) NULL,
 plan_json JSON NOT NULL,
 revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 finished_at DATETIME(6) NULL,
 KEY ix_deployment_jobs(state,created_at),
 FOREIGN KEY(tenant_id,space_id) REFERENCES spaces(tenant_id,id),
 FOREIGN KEY(release_id) REFERENCES app_releases(id), FOREIGN KEY(target_app_id) REFERENCES apps(id),
 FOREIGN KEY(created_by) REFERENCES users(id), FOREIGN KEY(agent_grant_id) REFERENCES agent_grants(id),
 CHECK(state IN ('queued','preparing','publishing','active','failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE entity_versions ADD COLUMN deployment_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD CONSTRAINT fk_publication_deployment FOREIGN KEY(deployment_id) REFERENCES release_deployments(id);

ALTER TABLE app_releases ADD COLUMN base_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '';
UPDATE app_releases r JOIN development_workspaces w ON w.id=r.workspace_id SET r.base_hash=w.base_hash;

ALTER TABLE entity_versions DROP CONSTRAINT ck_version_state,
 ADD CONSTRAINT ck_version_state CHECK(state IN ('queued','publishing','prepared','published','failed'));

-- The step a failed deployment had reached, so its status can name it.
ALTER TABLE release_deployments ADD COLUMN failed_state VARCHAR(16) NULL AFTER state,
 ADD CONSTRAINT ck_deployment_failed_state CHECK(failed_state IS NULL OR (state='failed' AND failed_state IN ('queued','preparing','publishing')));

-- The step a preparing workspace has reached (delivery.WorkspaceSteps); a
-- failed workspace keeps the step it failed at, a ready one has none.
ALTER TABLE development_workspaces ADD COLUMN preparation_step VARCHAR(16) NULL AFTER state,
 ADD CONSTRAINT ck_workspace_preparation_step CHECK(preparation_step IS NULL OR preparation_step IN ('storage','tables','publishing'));
UPDATE development_workspaces SET preparation_step='storage' WHERE state IN ('preparing','failed');
-- Retrying a failed deployment re-queues the same record: attempts counts its
-- runs, queued_at orders the Tenant's queue (a retry joins its end),
-- retried_by is the person whose live authority the latest attempt runs under,
-- and failed_base_hash is the production Space configuration left when it
-- failed, so a retry is refused once anything else has changed it.
ALTER TABLE release_deployments ADD COLUMN attempts INT UNSIGNED NOT NULL DEFAULT 1 AFTER failed_state,
 ADD COLUMN failed_base_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER attempts,
 ADD COLUMN retried_by CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER created_by,
 ADD COLUMN queued_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) AFTER created_at,
 ADD KEY ix_deployment_queue(state,queued_at),
 ADD CONSTRAINT fk_deployment_retried_by FOREIGN KEY(retried_by) REFERENCES users(id),
 ADD CONSTRAINT ck_deployment_attempts CHECK(attempts>0 AND (failed_base_hash IS NULL OR state='failed'));
UPDATE release_deployments SET queued_at=created_at;

-- Each entity numbers its versions 1, 2, ... in creation order. Every created
-- version gets one, whatever its state; a failed version keeps its number, so the
-- published sequence can have gaps. Versions are never deleted, so a number is
-- never reused. Existing versions are numbered by created_at (then id).
ALTER TABLE entity_versions ADD COLUMN version_number INT UNSIGNED NULL AFTER entity_id;
UPDATE entity_versions v JOIN (SELECT id,ROW_NUMBER() OVER (PARTITION BY entity_id ORDER BY created_at,id) AS n FROM entity_versions) o ON o.id=v.id SET v.version_number=o.n;
ALTER TABLE entity_versions MODIFY version_number INT UNSIGNED NOT NULL,
 ADD UNIQUE KEY uq_version_number(entity_id,version_number);

ALTER TABLE release_deployments
 MODIFY release_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 MODIFY target_app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN kind VARCHAR(16) NOT NULL DEFAULT 'release',
 ADD CONSTRAINT ck_deployment_kind CHECK (
   (kind='objects' AND release_id IS NULL AND target_app_id IS NULL) OR
   (kind IN ('release','package','attach') AND release_id IS NOT NULL AND target_app_id IS NOT NULL)),
 ADD KEY ix_deployment_space_state(space_id,state);
CREATE TABLE catalog_packages (
 package_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_json JSON NOT NULL,
 build_json JSON NULL,
 build_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 source_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 retired_at DATETIME(6) NULL,
 published_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 PRIMARY KEY(package_key,version),
 KEY ix_catalog_build(build_hash),
 KEY ix_catalog_source(source_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE package_installations (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 space_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 package_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 UNIQUE KEY uq_package_install_space(space_id,package_key),
 KEY ix_package_install_tenant(tenant_id,space_id),
 FOREIGN KEY (tenant_id,space_id) REFERENCES spaces(tenant_id,id),
 FOREIGN KEY (package_key,version) REFERENCES catalog_packages(package_key,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE package_stages (
 workspace_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 installation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 mode VARCHAR(16) NOT NULL,
 plan_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 plan_json JSON NOT NULL,
 deployment_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 release_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 UNIQUE KEY uq_package_install_mode(installation_id,mode),
 UNIQUE KEY uq_package_stage_operation(deployment_id),
 FOREIGN KEY (workspace_id) REFERENCES development_workspaces(id),
 FOREIGN KEY (installation_id) REFERENCES package_installations(id),
 FOREIGN KEY (deployment_id) REFERENCES release_deployments(id),
 FOREIGN KEY (release_id) REFERENCES app_releases(id),
 CHECK (mode IN ('objects','package','attach'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE release_sources (
 release_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 content_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 KEY ix_release_source_hash(content_hash),
 FOREIGN KEY (release_id) REFERENCES app_releases(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE build_sources (
 build_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 content_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 KEY ix_build_source_hash(content_hash),
 FOREIGN KEY (build_id) REFERENCES app_builds(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE app_builds ADD KEY ix_app_build_content_hash(content_hash);
CREATE TABLE package_app_audience (
 app_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 role_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 PRIMARY KEY(app_id,role_id),
 FOREIGN KEY (app_id) REFERENCES apps(id),
 FOREIGN KEY (role_id) REFERENCES roles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE database_resources
 ADD COLUMN package_preview TINYINT UNSIGNED NOT NULL DEFAULT 0,
 ADD COLUMN preview_cleanup_at DATETIME(6) NULL,
 ADD KEY ix_preview_quota(tenant_id,package_preview,state),
 ADD KEY ix_preview_cleanup(package_preview,preview_cleanup_at,tenant_id),
 ADD CONSTRAINT ck_database_preview CHECK (
   (package_preview=0 AND preview_cleanup_at IS NULL) OR
   (package_preview=1 AND management_mode='managed'));
UPDATE database_resources d
 JOIN spaces sp ON sp.database_resource_id=d.id AND sp.tenant_id=d.tenant_id
 JOIN development_workspaces w ON w.backing_space_id=sp.id AND w.id=sp.workspace_id
 JOIN package_stages x ON x.workspace_id=w.id
 SET d.package_preview=1
 WHERE d.management_mode='managed' AND d.database_name=CONCAT('la_',REPLACE(d.id,'-',''));
