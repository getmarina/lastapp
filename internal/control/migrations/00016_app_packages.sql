-- +goose Up
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
-- +goose Down
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Automatic destructive rollback is disabled';
