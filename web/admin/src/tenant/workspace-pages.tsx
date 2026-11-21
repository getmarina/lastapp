import type { ReactNode } from "react";
import { Card, EmptyState, ErrorBoundary, Tabs } from "../ui";
import { text, type Item } from "../api-types";
import { DevelopmentBanner } from "../development-banner";
import { TenantPeople } from "../tenant-people";
import { AccountPage } from "../account-page";
import { ActivityLog } from "../activity-log";
import { agentGrantsPath } from "../actors";
import { AgentAccess } from "../agent-access";
import type { TenantScreen } from "./screen";
import { tenantTabs } from "./navigation";
import { AppsPage, SpacesPage } from "./inventory";
import { HomePage } from "./home-page";
import { PackagesPage, MyAppsPage } from "./packages-page";
import { SpacePage } from "./space-page";
import { AppPage } from "./app-page";
import { TenantSettings } from "./settings-page";

type PageProps = {
  t: TenantScreen;
  refreshMe: () => Promise<void>;
  detailReady: boolean;
  peopleTab: string;
  title: string;
  actions: ReactNode;
  // A Tenant security manager.
  manager: boolean;
};

// The page region under the shell's header: a development workspace's
// banner, the page's tabs and the page itself, in an error boundary keyed
// by the URL.
export function TenantPages({
  url,
  detail,
  entityScreen,
  ...props
}: PageProps & { url: string; detail: Item; entityScreen: boolean }) {
  const { t, detailReady, peopleTab, title } = props;
  const { route, space, go } = t;
  const tabs = tenantTabs(route.page, t, space);
  const activeTab = route.page === "people" ? peopleTab : route.tab;
  return (
    <>
      {detailReady &&
        (route.page === "space" || route.page === "app") &&
        text(detail.workspace_id) && (
          <DevelopmentBanner
            workspace={text(detail.workspace_id)}
            openSpace={(id) => go("space", { space: id })}
          />
        )}
      {!!tabs.length && !entityScreen && route.page !== "app" && (
        <Tabs
          label={`${title} sections`}
          className="workspace-tabs"
          items={tabs}
          value={activeTab}
          onChange={(id) =>
            go(route.page, { space: route.space, app: route.app, tab: id })
          }
        />
      )}
      <ErrorBoundary key={url}>
        <TenantPage {...props} />
      </ErrorBoundary>
    </>
  );
}

// The page the route names: the account page, the empty state without a
// Tenant, or one of the Tenant's pages.
function TenantPage({
  t,
  refreshMe,
  detailReady,
  peopleTab,
  title,
  actions,
  manager,
}: PageProps) {
  const { me, tenant, base, route, caps, all, go } = t;
  return route.page === "account" ? (
    <AccountPage me={me} refreshMe={refreshMe} />
  ) : !tenant ? (
    <Card>
      <EmptyState title="No Tenant yet">
        Your administrator can add this account to a Tenant.
      </EmptyState>
    </Card>
  ) : (
    <>
      {route.page === "home" && <HomePage t={t} />}
      {route.page === "apps" && <AppsPage t={t} />}
      {route.page === "packages" && <PackagesPage t={t} />}
      {route.page === "my-apps" && <MyAppsPage t={t} />}
      {route.page === "spaces" && <SpacesPage t={t} />}
      {route.page === "space" && detailReady && <SpacePage t={t} />}
      {route.page === "app" && detailReady && <AppPage t={t} />}
      {route.page === "people" && (
        <TenantPeople
          key={tenant + "/" + peopleTab + "/" + route.team}
          tenant={tenant}
          caps={caps}
          tab={peopleTab}
          team={route.team}
          selectTeam={(id) => go("people", { tab: "teams", team: id })}
        />
      )}
      {route.page === "activity" && (
        <ActivityLog
          path={base + "/audit-events"}
          grants={agentGrantsPath(tenant, all("authorization.manage"))}
          title={title}
          actions={actions}
        />
      )}
      {route.page === "agents" && (
        // A Tenant security manager reviews every connection in
        // the Tenant; anyone else their own.
        <AgentAccess
          key={tenant}
          me={me}
          tenant={manager ? tenant : undefined}
          heading={false}
        />
      )}
      {route.page === "settings" && <TenantSettings t={t} />}
    </>
  );
}
