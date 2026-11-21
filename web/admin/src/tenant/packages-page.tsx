import { useState } from "react";
import { api } from "../api";
import { text, type Item } from "../api-types";
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Collection,
  Inline,
  LinkButton,
  RemoteSelect,
  Select,
  Stack,
} from "../ui";
import type { TenantScreen } from "./screen";
import { PackageReview, type PackagePlan } from "./package-review";
import { PackageStages } from "./package-stages";

type Selection = {
  key: string;
  version: string;
  space_id: string;
  mode: string;
};

export function PackagesPage({ t }: { t: TenantScreen }) {
  const [picked, setPicked] = useState<Item>();
  const [space, setSpace] = useState("");
  const [mode, setMode] = useState("objects");
  const [review, setReview] = useState<PackagePlan>();
  const [request, setRequest] = useState("");
  const selection: Selection = {
    key: text(picked?.key),
    version: text(picked?.version),
    space_id: space,
    mode,
  };
  function select(row: Item, destination = "", attach = false) {
    setPicked(row);
    setSpace(destination);
    setMode(attach ? "attach" : "objects");
    setReview(undefined);
  }
  return (
    <Stack>
      <Collection
        path={t.base + "/packages"}
        caption="Package catalog"
        empty="No packages have been published."
        header={<CardHeader title="Package catalog" />}
        columns={[
          ["Package", (row) => text(row.display_name || row.name)],
          ["Version", (row) => text(row.version)],
          ["Includes", (row) => (row.has_app ? "Objects and App" : "Objects")],
        ]}
        actions={(row) => (
          <Button onClick={() => select(row)}>Configure installation</Button>
        )}
      />
      {picked && (
        <Card>
          <CardHeader title={text(picked.name) || selection.key} />
          <CardBody>
            <Stack>
              <Inline>
                <RemoteSelect
                  path={t.base + "/spaces"}
                  initial={[]}
                  label="Destination Space"
                  required
                  excludeInactive
                  value={space}
                  onChange={(value) => {
                    setSpace(value);
                    setReview(undefined);
                  }}
                />
                <Select
                  aria-label="Install mode"
                  value={mode}
                  options={[
                    { value: "objects", label: "Business objects" },
                    ...(picked.has_app
                      ? [
                          {
                            value: "package",
                            label: "Business objects and App",
                          },
                          {
                            value: "attach",
                            label: "Attach App to installed objects",
                          },
                        ]
                      : []),
                  ]}
                  onChange={(value) => {
                    setMode(value);
                    setReview(undefined);
                  }}
                />
                <Button onClick={() => t.go("spaces")}>Manage Spaces</Button>
                <Button
                  disabled={!space || t.busy}
                  onClick={() =>
                    void t.act(async () => {
                      setReview(undefined);
                      const plan = await api<PackagePlan>(
                        t.base + "/package-plans",
                        "POST",
                        selection,
                      );
                      setRequest(crypto.randomUUID());
                      setReview(plan);
                    })
                  }
                >
                  Review changes
                </Button>
              </Inline>
              {review && (
                <Stack>
                  <PackageReview review={review} />
                  <Inline>
                    <Button
                      variant="primary"
                      disabled={t.busy}
                      onClick={() =>
                        void t.act(async () => {
                          await api(t.base + "/package-stages", "POST", {
                            ...selection,
                            plan_hash: review.plan_hash,
                            request_key: request,
                          });
                          setPicked(undefined);
                        })
                      }
                    >
                      Stage installation
                    </Button>
                  </Inline>
                </Stack>
              )}
            </Stack>
          </CardBody>
        </Card>
      )}
      <PackageStages
        t={t}
        attach={(row) =>
          select({ ...row, has_app: true }, text(row.space_id), true)
        }
      />
    </Stack>
  );
}

export function MyAppsPage({ t }: { t: TenantScreen }) {
  return (
    <Collection
      path={t.base + "/my-apps"}
      caption="My Apps"
      empty="Apps assigned to you or your teams will appear here."
      columns={[["App", (row) => text(row.display_name || row.name)]]}
      actions={(row) => (
        <LinkButton href={text(row.url)} external>
          Open App
        </LinkButton>
      )}
    />
  );
}
