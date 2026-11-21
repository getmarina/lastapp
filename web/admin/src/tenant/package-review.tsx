import { Alert, Card, CardBody, CardHeader, Stack } from "../ui";

type Entity = {
  definition: {
    label: string;
    permissions:
      | {
          key: string;
          label: string;
          operation: string;
          fields?: string[];
          relationship?: string;
        }[]
      | null;
  };
};

export type PackagePlan = {
  plan_hash: string;
  base_hash: string;
  inheriting_apps: string[];
  dependencies: Record<string, string> | null;
  quota_use: {
    workspaces: number;
    entities: number;
    roles: number;
    apps: number;
    microapps: number;
  };
  changes: {
    entities: Entity[] | null;
    roles: { display_name: string }[] | null;
    apps: { display_name: string }[] | null;
  };
};

function count(value: number, singular: string, plural = singular + "s") {
  return `${value} ${value === 1 ? singular : plural}`;
}

export function PackageReview({ review }: { review: PackagePlan }) {
  const { apps, entities, roles, microapps, workspaces } = review.quota_use;
  const dependencies = Object.entries(review.dependencies || {});
  return (
    <Card>
      <CardHeader
        title="Installation review"
        description={[
          count(workspaces, "workspace database"),
          count(entities, "entity", "entities"),
          count(roles, "role"),
          count(apps, "App"),
          count(microapps, "micro-app"),
        ].join(", ")}
      />
      <CardBody>
        <Stack gap="sm">
          {(review.changes.entities || []).map(({ definition }) => (
            <Stack key={definition.label} gap="sm">
              <strong>{definition.label}</strong>
              {definition.permissions?.map((permission) => (
                <p key={permission.key}>
                  {permission.label} ({permission.operation}
                  {permission.relationship && `, ${permission.relationship}`})
                  {Boolean(permission.fields?.length) &&
                    ` · ${permission.fields?.join(", ")}`}
                </p>
              ))}
            </Stack>
          ))}
          {review.changes.roles?.map((role) => (
            <p key={role.display_name}>Starter role · {role.display_name}</p>
          ))}
          {review.changes.apps?.map((app) => (
            <p key={app.display_name}>App · {app.display_name}</p>
          ))}
          {dependencies.map(([key, version]) => (
            <p key={key}>
              Requires {key} {version} in this Space
            </p>
          ))}
          {review.inheriting_apps.length > 0 && (
            <Alert tone="warning">
              Apps inheriting Space access may expose the new objects (
              {review.inheriting_apps.join(", ")}).
            </Alert>
          )}
        </Stack>
      </CardBody>
    </Card>
  );
}
