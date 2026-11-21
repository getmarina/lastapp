package delivery

import (
	"context"

	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/registry"
)

// DeploymentSteps are the states a production deployment passes through, in
// worker order (DeploymentJob: queued → preparing → publishing → active);
// DeploymentFailed ends one at any step. Together they are the
// release_deployments.state CHECK constraint (TestDeploymentStatesMatchSchema).
// The console derives its deployment timeline and polling from Policy.
var DeploymentSteps = []string{"queued", "preparing", "publishing", registry.StateActive}

const DeploymentFailed = "failed"

// WorkspaceSteps are the steps a preparing development workspace passes
// through, in worker order (prepareWorkspace): its database is provisioned,
// its entity tables are prepared, then its entity publications finish, and
// the workspace is ready. They are the development_workspaces.preparation_step
// CHECK constraint (TestWorkspaceStepsMatchSchema); the console derives its
// preparation progress from Policy.
var WorkspaceSteps = []string{"storage", "tables", "publishing"}

func Policy() registry.Row {
	return registry.Row{"deployment_states": registry.Row{"steps": DeploymentSteps, "failed": DeploymentFailed}, "workspace_steps": WorkspaceSteps, "max_workspace_apps": registry.MaxWorkspaceApps, "max_release_microapps": registry.MaxReleaseMicroapps, "max_release_dependencies": registry.MaxReleaseDependencies, "max_snapshot_bytes": registry.MaxSpaceSnapshotBytes, "max_app_builds": registry.MaxAppBuilds, "max_app_releases": registry.MaxAppReleases, "max_owned_workspaces": registry.MaxOwnedWorkspaces, "max_space_workspaces": registry.MaxSpaceWorkspaces, "upload": artifact.Limits}
}

func (s *Service) snapshotAppQuota(ctx context.Context, q registry.Queryer, tenant string, snap SpaceConfiguration) error {
	policy, e := registry.PolicyFor(ctx, q, tenant)
	if e != nil {
		return e
	}
	if policy.MaxApps == nil {
		return nil
	}
	var count int
	if e = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM apps a JOIN spaces sp ON sp.id=a.space_id WHERE a.tenant_id=? AND sp.workspace_id IS NULL AND a.state<>'archived'", tenant).Scan(&count); e != nil {
		return e
	}
	for _, app := range snap.Apps {
		var n int
		if e = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM apps WHERE id=? AND state<>'archived'", app.ID).Scan(&n); e != nil {
			return e
		}
		if app.State != registry.StateArchived && n == 0 {
			count++
		}
		if app.State == registry.StateArchived && n == 1 {
			count--
		}
	}
	if int64(count) > *policy.MaxApps {
		return registry.Conflict("This release exceeds the Tenant's production App quota.")
	}
	return nil
}
