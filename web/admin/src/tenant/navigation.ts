import {
  Activity,
  Bot,
  House,
  LayoutGrid,
  Package,
  PanelsTopLeft,
  Shield,
  AlignLeft,
} from "lucide-react";
import type { NavItem, TabItem } from "../ui";
import type { Item } from "../api-types";

// The Tenant console's navigation, the one source for the sidebar, the
// command palette and the page tabs. Visibility follows the capabilities the
// backend reports (GET …/capabilities); the backend still authorizes every
// request a screen makes.
export type Capabilities = {
  all: (action: string) => boolean;
  can: (action: string) => boolean;
  scoped: (action: string, spaceID?: string, appID?: string) => boolean;
};

// The Tenant's members and teams (page "people"): reached from the account
// menu, the Space Access page and the command palette, and from Access when
// the account reads no Space's or App's assignments.
export const peopleVisible = (c: Capabilities) =>
  c.all("membership.read") || c.can("team.manage");

// Entities open the current (or first) Space's Entities tab, Access the
// current (or first) Space's Access tab (see accessTarget), Agents the
// Tenant's agent access for a security manager and otherwise the person's
// own. Spaces are listed under the sidebar's Spaces heading; Settings is in
// the account menu.
const sections: (NavItem & { visible?: (c: Capabilities) => boolean })[] = [
  { id: "home", label: "Home", icon: House },
  { id: "apps", label: "Apps", icon: LayoutGrid },
  { id: "my-apps", label: "My Apps", icon: PanelsTopLeft },
  {
    id: "packages",
    label: "Packages",
    icon: Package,
    visible: (c) => c.can("workspace.manage"),
  },
  {
    id: "entities",
    label: "Entities",
    icon: AlignLeft,
    visible: (c) => c.can("space.read") && c.can("entity.manage"),
  },
  {
    id: "access",
    label: "Access",
    icon: Shield,
    visible: (c) => c.can("authorization.read") || peopleVisible(c),
  },
  { id: "agents", label: "Agents", icon: Bot },
  {
    id: "activity",
    label: "Activity",
    icon: Activity,
    visible: (c) => c.can("audit.read"),
  },
];

// Pages reached from elsewhere than the sidebar sections, by their title.
const otherPages: Record<string, string> = {
  people: "Members and teams",
  spaces: "Spaces",
  settings: "Settings",
};

// The sections this account may open, in sidebar order; `badges` adds a
// count to a section (Agents).
export const tenantNav = (
  c: Capabilities,
  badges: Record<string, number | undefined> = {},
): NavItem[] =>
  sections
    .filter((s) => !s.visible || s.visible(c))
    .map(({ id, label, icon }) => ({ id, label, icon, badge: badges[id] }));

// Where the Access section opens: the open Space's (an App's Space's) Access
// tab when this account reads its assignments; otherwise the first Space
// that it may read (the sidebar's list, then the capabilities); otherwise
// the open or first such App; otherwise the Tenant's members and teams.
// `read` is the authorization.read capability.
export function accessTarget(
  c: Capabilities,
  read: { spaces: string[] | null; apps: string[] | null } | undefined,
  open: { space: string; app: string },
  listed: string[],
):
  | { page: "space"; space: string }
  | { page: "app"; app: string }
  | { page: "people" }
  | null {
  const space = [open.space, ...listed, ...(read?.spaces || [])].find(
    (id) => !!id && c.scoped("authorization.read", id, ""),
  );
  if (space) return { page: "space", space };
  const app =
    open.app && c.scoped("authorization.read", open.space, open.app)
      ? open.app
      : read?.apps?.[0];
  if (app) return { page: "app", app };
  return peopleVisible(c) ? { page: "people" } : null;
}

export const sectionLabel = (id: string) =>
  sections.find((s) => s.id === id)?.label || otherPages[id];

// A page's tabs: a Space or App (`space` is the open Space, for the
// development-workspace check), People or Settings.
export function tenantTabs(
  page: string,
  c: Capabilities,
  space: Item = {},
): TabItem[] {
  const tabs: [string, string, boolean?][] =
    page === "space"
      ? [
          ["apps", "Apps"],
          [
            "development",
            "Development",
            !space.workspace_id && c.scoped("workspace.manage"),
          ],
          ["entities", "Entities", c.scoped("entity.manage")],
          ["access", "Access", c.scoped("authorization.read")],
          ["settings", "Settings"],
        ]
      : page === "app"
        ? [
            ["releases", "Releases", c.scoped("release.manage")],
            ["microapps", "Micro-apps", c.scoped("microapp.read")],
            ["data", "Data access", c.scoped("app.manage")],
            ["access", "Access", c.scoped("authorization.read")],
            ["settings", "Settings"],
          ]
        : page === "people"
          ? [
              ["members", "Members", c.all("membership.read")],
              ["teams", "Teams", c.can("team.read")],
            ]
          : page === "settings"
            ? [
                ["general", "General"],
                ["databases", "Databases", c.all("database.read")],
                ["agents", "Agent access", c.all("authorization.manage")],
              ]
            : [];
  return tabs
    .filter(([, , shown]) => shown !== false)
    .map(([id, label]) => ({ id, label }));
}
