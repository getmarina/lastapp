-- +goose Up
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
-- +goose Down
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Automatic destructive rollback is disabled';
