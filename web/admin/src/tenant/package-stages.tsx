import { api } from "../api";
import { num, text, type Item } from "../api-types";
import { poll } from "../polling";
import {
  Button,
  CardHeader,
  Collection,
  Inline,
  LinkButton,
  StatusBadge,
  confirmAction,
} from "../ui";
import { PackageReview, type PackagePlan } from "./package-review";
import type { TenantScreen } from "./screen";

const working = new Set(["preparing", "queued", "publishing"]);

export function PackageStages({
  t,
  attach,
}: {
  t: TenantScreen;
  attach: (row: Item) => void;
}) {
  return (
    <Collection
      path={t.base + "/package-stages"}
      caption="Package installations"
      header={<CardHeader title="Installations" />}
      empty="No package installations in your Spaces."
      poll={(rows) =>
        rows.some((row) => working.has(text(row.state))) ? poll.tenant : false
      }
      columns={[
        ["Package", (row) => text(row.key)],
        ["Version", (row) => text(row.version)],
        ["State", (row) => <StatusBadge state={text(row.state)} />],
        ["Details", (row) => text(row.error_message) || text(row.failed_step)],
      ]}
      actions={(row) => <StageActions t={t} row={row} attach={attach} />}
    />
  );
}

function StageActions({
  t,
  row,
  attach,
}: {
  t: TenantScreen;
  row: Item;
  attach: (row: Item) => void;
}) {
  const stage = text(row.workspace_id);
  const state = text(row.state);
  const space = text(row.space_id);
  const path = t.base + "/package-stages/" + stage;
  const run = (action: string) =>
    void t.act(() => api(path + "/" + action, "POST", {}, num(row.revision)));
  async function discard() {
    const confirmed = await confirmAction({
      title: "Discard staged installation?",
      message: "This deletes the isolated preview workspace’s test data.",
      confirmLabel: "Discard",
      danger: true,
    });
    if (!confirmed) return;
    await t.act(async () => {
      if (state !== "archived") {
        await api(
          `/v1/workspaces/${stage}/archive`,
          "POST",
          {},
          num(row.revision),
        );
      }
      await api(path + "/discard", "POST", {});
    });
  }
  return (
    <Inline>
      {state === "ready" && row.mode !== "objects" && !row.release_id && (
        <Button disabled={t.busy} onClick={() => run("preview")}>
          Prepare preview
        </Button>
      )}
      {row.preview_url && state === "ready" ? (
        <LinkButton href={text(row.preview_url)} external>
          Open preview
        </LinkButton>
      ) : null}
      {state === "ready" &&
        (row.mode === "objects" || Boolean(row.release_id)) && (
          <Button
            disabled={t.busy}
            variant="primary"
            onClick={async () => {
              if (
                await confirmAction({
                  title: "Activate package in production?",
                  confirmLabel: "Activate",
                })
              )
                run("activate");
            }}
          >
            Activate
          </Button>
        )}
      {state === "failed" && (
        <Button
          disabled={t.busy}
          onClick={() => {
            if (!row.operation_id) {
              run("retry");
              return;
            }
            void t.act(async () => {
              const review = await api<PackagePlan>(path + "/retry-plan");
              if (
                await confirmAction({
                  title: "Retry package activation?",
                  confirmLabel: "Retry",
                  message: <PackageReview review={review} />,
                })
              ) {
                await api(
                  path +
                    "/retry?base_hash=" +
                    encodeURIComponent(review.base_hash),
                  "POST",
                  {},
                  num(row.revision),
                );
              }
            });
          }}
        >
          Retry
        </Button>
      )}
      {!row.operation_id && !working.has(state) && (
        <Button disabled={t.busy} onClick={() => void discard()}>
          {state === "archived" ? "Remove stage" : "Discard"}
        </Button>
      )}
      {state === "active" && row.mode === "objects" && Boolean(row.has_app) && (
        <Button onClick={() => attach(row)}>Add App</Button>
      )}
      {state === "active" && (
        <Button onClick={() => t.go("space", { space, tab: "access" })}>
          Set up access
        </Button>
      )}
      {state === "active" && row.app_id ? (
        <Button onClick={() => t.go("app", { app: text(row.app_id) })}>
          Manage App
        </Button>
      ) : null}
    </Inline>
  );
}
