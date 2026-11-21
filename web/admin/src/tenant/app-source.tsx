import { useApiQuery } from "../query";
import { LinkButton } from "../ui";

export function AppSource({
  app,
  workspace,
}: {
  app: string;
  workspace: boolean;
}) {
  const { data } = useApiQuery<{ release_id: string }>(
    app ? `/v1/apps/${app}/source-info` : null,
  );
  return data?.release_id ? (
    <>
      <LinkButton href={`/v1/releases/${data.release_id}/source`}>
        Download source
      </LinkButton>
      {workspace && (
        <LinkButton href={`/v1/apps/${app}/local-config`}>
          Workspace configuration
        </LinkButton>
      )}
    </>
  ) : null;
}
