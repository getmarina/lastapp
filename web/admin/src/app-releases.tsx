import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { apiKey, queryClient, useApiQuery } from "./query";
import { useConsoleConfig } from "./console-config";
import { deployModal, retryModal, useDeploymentStates } from "./deployment";
import { Alert, FormModal, PageStack, type Modal } from "./ui";
import {
  num,
  optText,
  type Build,
  type Deployment,
  type Item,
  type Page,
  type Release,
} from "./api-types";
import { useAction } from "./use-action";
import {
  listedReleases,
  useReleaseHistory,
} from "./app-releases/use-release-history";
import { uploadBuild } from "./app-releases/build-upload";
import { releaseModal } from "./app-releases/release-modal";
import { ShipCard } from "./app-releases/ship-card";
import { DeploymentCard } from "./app-releases/deployment-card";
import { ReleaseList } from "./app-releases/release-list";
import { ReleaseReview } from "./app-releases/release-review";
import { BuildsCard } from "./app-releases/builds-card";

// An App's Releases tab: the ship flow. In a development workspace App the
// author uploads a build, captures it as an immutable release, previews it
// and deploys it; a production App shows what it runs and where to develop.
// The production deployment card follows the real deployment states.
export function AppReleases({
  app,
  tenant,
  deployment,
  deploymentError,
  refresh,
}: {
  app: Item;
  tenant: string;
  deployment: Deployment | null | undefined;
  deploymentError: string;
  refresh: () => void;
}) {
  const [selected, setSelected] = useState<Release>(),
    [error, setError] = useState(""),
    [modal, setModal] = useState<Modal | null>(null);
  const { config } = useConsoleConfig();
  const states = useDeploymentStates();
  const development = !!app.workspace_id;
  const { releasePages, releases, hasMore, listError } = useReleaseHistory(
    app.id,
  );
  const builds =
    useApiQuery<Page<Build>>(development ? `/v1/apps/${app.id}/builds` : null)
      .data?.items || [];
  const deploymentPath = `/v1/apps/${app.id}/deployment`;
  // When this deployment goes live, the App itself changed: refresh it.
  const previousDeployment = useRef<Deployment | undefined>(undefined);
  useEffect(() => {
    const previous = previousDeployment.current;
    previousDeployment.current = deployment || undefined;
    if (
      deployment?.state === states.live &&
      previous &&
      previous.id === deployment.id &&
      previous.state !== states.live
    )
      refresh();
  }, [deployment, refresh, states.live]);
  const { busy, run } = useAction(app.id, {
    onSuccess: () => {},
    onError: setError,
  });
  async function upload(files: FileList, source?: File) {
    await run(async () => {
      await capture(await uploadBuild(app.id, files, source));
    });
  }
  async function capture(buildID: string) {
    setModal(
      await releaseModal(
        tenant,
        app,
        buildID,
        releases,
        config.development_policy.max_release_microapps,
        setSelected,
      ),
    );
  }
  async function preview(release: string) {
    const current = await api<Item>(`/v1/tenants/${tenant}/apps/${app.id}`),
      result = await api<{ url: string }>(
        `/v1/releases/${release}/preview`,
        "POST",
        {},
        num(current.revision),
      );
    refresh();
    window.open(result.url, "_blank", "noopener,noreferrer");
  }
  // Show a new or retried deployment at once; its state restarts the polling.
  function show(result: Deployment) {
    previousDeployment.current = result;
    queryClient.setQueryData(apiKey(deploymentPath), { deployment: result });
  }
  // Deploys a release to production.
  function deploy(release: Release) {
    setModal(deployModal(tenant, release, show));
  }
  const failed = deployment?.state === states.failed,
    live = deployment?.state === states.live;
  const listed = listedReleases(releases, deployment, development);
  const serving = optText(app.active_release_id);
  const servingName = listed.find((r) => r.id === serving)?.display_name;
  function open(id: string) {
    return run(async () =>
      setSelected(await api<Release>("/v1/releases/" + id)),
    );
  }
  return (
    <PageStack>
      {(error || listError || deploymentError) && (
        <Alert>{error || listError || deploymentError}</Alert>
      )}
      {development && (
        <ShipCard
          app={app.id}
          busy={busy}
          builds={builds}
          releases={releases}
          deployment={deployment}
          live={live}
          failed={failed}
          upload={upload}
        />
      )}
      <DeploymentCard
        deployment={deployment}
        stateSteps={states.steps}
        live={live}
        failed={failed}
        development={development}
        servingName={servingName}
        busy={busy}
        retry={(d) => setModal(retryModal(d, show))}
      />
      <ReleaseList
        listed={listed}
        deployment={deployment}
        liveState={states.live}
        live={live}
        failed={failed}
        development={development}
        serving={serving}
        busy={busy}
        loaded={!!releasePages.data}
        hasMore={hasMore}
        loadMore={() => void releasePages.fetchNextPage()}
        preview={(id) => run(() => preview(id))}
        open={open}
      />
      {selected && (
        <ReleaseReview
          release={selected}
          busy={busy}
          preview={
            development ? () => run(() => preview(selected.id)) : undefined
          }
          deploy={() => deploy(selected)}
        />
      )}
      {development && builds.length > 0 && (
        <BuildsCard
          builds={builds}
          busy={busy}
          capture={(id) => run(() => capture(id))}
        />
      )}
      {modal && (
        <FormModal
          modal={modal}
          close={() => setModal(null)}
          done={() => setModal(null)}
          lookups={{}}
        />
      )}
    </PageStack>
  );
}
