import { useState } from "react";
import { Plus, RefreshCw } from "lucide-react";
import { api } from "./api";
import {
  Alert,
  Badge,
  Button,
  CardHeader,
  Collection,
  confirmAction,
  FormModal,
  IconButton,
  Inline,
  InlineError,
  Progress,
  Stack,
  StatusBadge,
  statusTone,
  toast,
  type Modal,
  Icon,
} from "./ui";
import { num, text, type Item } from "./api-types";
import { useAction } from "./use-action";
import { refreshQueries } from "./query";
import { useWorkspaceProgress } from "./workspaces";
import { createWorkspaceModal } from "./development-workspace-form";
import { poll } from "./polling";

// A Space's Development tab: the signed-in author's isolated workspaces (an
// agent's own workspaces included). Each copies the published Space
// configuration into its own database; the list refreshes itself while one
// is still being prepared (a work-in-progress state, statusTone "warning").
export function DevelopmentWorkspaces({
  space,
  openSpace,
}: {
  space: string;
  openSpace: (id: string) => void;
}) {
  const [modal, setModal] = useState<Modal | null>(null),
    [error, setError] = useState("");
  const progress = useWorkspaceProgress();
  const { busy, run } = useAction(space, {
    onSuccess: () => {},
    onError: setError,
  });
  const change = (row: Item, retry: boolean) =>
    api(
      `/v1/workspaces/${row.id}/${retry ? "retry" : "archive"}`,
      "POST",
      {},
      num(row.revision),
    );
  async function archive(row: Item) {
    const confirmed = await confirmAction({
      title: `Archive ${text(row.display_name)}?`,
      message: row.package_preview
        ? "Stops this preview and deletes its test data. Production and release history are preserved."
        : "Stops its development APIs and previews. Production and stored data are preserved.",
      confirmLabel: "Archive workspace",
      danger: true,
    });
    if (!confirmed) return;
    await run(async () => {
      await change(row, false);
      toast(`${text(row.display_name)} archived`, { tone: "success" });
    });
  }
  return (
    <Stack>
      {error && <Alert>{error}</Alert>}
      <Collection
        key={space}
        path={`/v1/spaces/${space}/workspaces`}
        caption="Development workspaces"
        header={
          <CardHeader
            title="Development workspaces"
            description="Work privately against an empty database. Production stays separate."
            actions={
              <>
                <IconButton
                  label="Refresh workspaces"
                  icon={<Icon icon={RefreshCw} size="lg" aria-hidden />}
                  tooltip="bottom"
                  onClick={() => void refreshQueries()}
                />
                <Button
                  variant="primary"
                  icon={<Icon icon={Plus} size="lg" aria-hidden />}
                  onClick={() => setModal(createWorkspaceModal(space))}
                >
                  Create workspace
                </Button>
              </>
            }
          />
        }
        poll={(rows) =>
          rows.some((r) => statusTone(r.state) === "warning")
            ? poll.workspaces
            : false
        }
        columns={[
          [
            "Workspace",
            (r) => (
              <Stack gap="sm">
                <strong>{text(r.display_name)}</strong>
                {!!r.owner_agent_grant_id && (
                  <Inline>
                    <Badge tone="info">Agent workspace</Badge>
                  </Inline>
                )}
                {text(r.error_message) && (
                  <InlineError>{text(r.error_message)}</InlineError>
                )}
              </Stack>
            ),
          ],
          [
            "Status",
            (r) => {
              const step =
                r.state === "preparing" ? progress(r.preparation_step) : null;
              return step ? (
                <Stack gap="sm">
                  <Inline>
                    <StatusBadge state={r.state} />
                  </Inline>
                  <Progress {...step} />
                </Stack>
              ) : (
                <StatusBadge state={r.state} />
              );
            },
          ],
          ["Created", (r) => new Date(text(r.created_at)).toLocaleString()],
        ]}
        empty="Create a workspace here, or let your connected agent create its own."
        actions={(r) => (
          <>
            {r.state === "ready" && (
              <Button
                size="sm"
                variant="primary"
                onClick={() => openSpace(text(r.backing_space_id))}
              >
                Open workspace
              </Button>
            )}
            {r.state === "failed" && (
              <Button
                size="sm"
                disabled={busy}
                onClick={() => void run(() => change(r, true))}
              >
                Retry
              </Button>
            )}
            {r.state !== "archived" && (
              <Button
                size="sm"
                variant="danger"
                disabled={busy}
                onClick={() => void archive(r)}
              >
                Archive
              </Button>
            )}
          </>
        )}
      />
      <Alert tone="info" live={false}>
        Each connected agent creates its own workspace through MCP. Source and
        compilation stay on the author’s computer.
      </Alert>
      {modal && (
        <FormModal
          modal={modal}
          close={() => setModal(null)}
          done={() => setModal(null)}
          lookups={{}}
        />
      )}
    </Stack>
  );
}
