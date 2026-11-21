import { ArrowUpRight, RefreshCw } from "lucide-react";
import { AppReleases } from "../app-releases";
import {
  AppHeader,
  Button,
  IconButton,
  LinkButton,
  PageStack,
  Tabs,
  Icon,
} from "../ui";
import { AccessControl } from "../access-control";
import { refreshQueries } from "../query";
import { keyOf, optText, text } from "../api-types";
import { useAppDeployment, useDeploymentStates } from "../deployment";
import { DataAccess } from "./app-data-access";
import { MicroApps } from "./app-microapps";
import { AppSettings } from "./app-settings";
import { appStatus, hostOf } from "./app-status";
import { tenantTabs } from "./navigation";
import { createMicro } from "./forms";
import { AppSource } from "./app-source";
import type { TenantScreen } from "./screen";

// An App: its header (tile, name, hosted address, status, launch) over its
// sections: Releases, Micro-apps and Data access, then Access and Settings.
// The status and address come from the App read and, where this account may
// manage releases, its latest deployment.
export function AppPage({ t }: { t: TenantScreen }) {
  const { all, app, can, go, route, scoped, tenant } = t;
  const deploymentStates = useDeploymentStates();
  const { deployment, error: deploymentError } = useAppDeployment(
    text(app.id),
    scoped("release.manage"),
  );
  const status = appStatus(app, deployment, deploymentStates);
  const tabs = tenantTabs("app", { all, can, scoped });
  const tab = tabs.some((x) => x.id === route.tab)
    ? route.tab
    : tabs[0]?.id || "";
  const origin = text(app.hosted_origin);
  return (
    <PageStack>
      <AppHeader
        name={text(app.display_name)}
        tone={status.tone}
        address={hostOf(origin)}
        status={status.badge}
        actions={
          <>
            {/* The shell's page header is hidden here, so its Refresh moves in. */}
            <IconButton
              label="Refresh"
              icon={<Icon icon={RefreshCw} size="lg" aria-hidden />}
              tooltip="bottom"
              onClick={() => void refreshQueries()}
            />
            {tab === "microapps" && scoped("microapp.manage") && (
              <Button onClick={() => createMicro(t)}>Add micro-app</Button>
            )}
            {!app.workspace_id &&
              scoped("workspace.manage", optText(app.space_id), "") && (
                <Button
                  onClick={() =>
                    go("space", {
                      space: optText(app.space_id),
                      tab: "development",
                    })
                  }
                >
                  Customize
                </Button>
              )}
            {scoped("release.manage") && (
              <AppSource
                app={text(app.id)}
                workspace={Boolean(app.workspace_id)}
              />
            )}
            {optText(app.active_release_id) && origin && (
              <LinkButton
                href={origin + "/"}
                external
                variant="primary"
                trailingIcon={
                  <Icon icon={ArrowUpRight} size="lg" aria-hidden />
                }
              >
                Launch app
              </LinkButton>
            )}
          </>
        }
      />
      <Tabs
        label={`${text(app.display_name)} sections`}
        variant="line"
        className="workspace-tabs"
        items={tabs}
        value={tab}
        onChange={(id) => go("app", { app: route.app, tab: id })}
      />
      {tab === "releases" && (
        <AppReleases
          key={keyOf(app.id)}
          app={app}
          tenant={tenant}
          deployment={deployment}
          deploymentError={deploymentError}
          refresh={() => void refreshQueries()}
        />
      )}
      {tab === "microapps" && <MicroApps t={t} />}
      {tab === "data" && <DataAccess t={t} />}
      {tab === "access" && (
        <AccessControl
          key={keyOf(app.id)}
          tenant={tenant}
          caps={t.caps}
          scopeKind="app"
          scopeId={text(app.id)}
        />
      )}
      {tab === "settings" && <AppSettings t={t} />}
    </PageStack>
  );
}
