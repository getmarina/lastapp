import { useState } from "react";
import { Download, RefreshCw, Upload } from "lucide-react";
import { refreshQueries } from "../query";
import {
  Alert,
  Button,
  Inline,
  Card,
  CardBody,
  CardHeader,
  IconButton,
  LinkButton,
  Stack,
  Stepper,
  UploadButton,
  type Step,
  Icon,
} from "../ui";
import type { Build, Deployment, ReleaseItem } from "../api-types";

// A development workspace App's ship flow: get the starter, upload a build
// and follow it on the way to production.
export function ShipCard({
  app,
  busy,
  builds,
  releases,
  deployment,
  live,
  failed,
  upload,
}: {
  app: unknown;
  busy: boolean;
  builds: Build[];
  releases: ReleaseItem[];
  deployment: Deployment | null | undefined;
  live: boolean;
  failed: boolean;
  upload: (files: FileList, source?: File) => Promise<void>;
}) {
  // Where this development App is on the way to production.
  const [source, setSource] = useState<File>();
  const ship: Step[] = [
    {
      id: "workspace",
      label: "Workspace",
      note: "Isolated from production",
      state: "done",
    },
    {
      id: "build",
      label: "Upload a build",
      note: builds.length
        ? `${builds.length} uploaded`
        : "Build locally, then upload the folder",
      state: builds.length ? "done" : "active",
    },
    {
      id: "release",
      label: "Create a release",
      note: releases[0]
        ? `Latest: ${releases[0].display_name}`
        : "Pins the build, micro-apps and Space snapshot",
      state: releases.length ? "done" : builds.length ? "active" : "pending",
    },
    {
      id: "deploy",
      label: "Deploy to production",
      note: deployment ? `Latest deployment: ${deployment.state}` : undefined,
      state: !deployment
        ? "pending"
        : live
          ? "done"
          : failed
            ? "failed"
            : "active",
    },
  ];
  return (
    <Card>
      <CardHeader
        title="Ship this App"
        description="Build locally, test this workspace, then deploy a reviewed release."
        actions={
          <>
            <IconButton
              label="Refresh releases"
              icon={<Icon icon={RefreshCw} size="lg" aria-hidden />}
              tooltip="bottom"
              disabled={busy}
              onClick={() => void refreshQueries()}
            />
            <LinkButton
              href={`/v1/apps/${app}/starter.zip`}
              icon={<Icon icon={Download} size="lg" aria-hidden />}
            >
              Get starter
            </LinkButton>
            <UploadButton
              directory
              variant="primary"
              loading={busy}
              icon={<Icon icon={Upload} size="lg" aria-hidden />}
              onFiles={(files) => upload(files, source)}
            >
              {busy ? "Working…" : "Upload build folder"}
            </UploadButton>
          </>
        }
      />
      <CardBody>
        <Stack gap="md">
          <Stepper label="Ship flow" steps={ship} />
          <Inline>
            <UploadButton
              accept=".zip"
              disabled={busy}
              onFiles={(files) => setSource(files[0])}
            >
              {source ? "Replace source ZIP" : "Attach source ZIP"}
            </UploadButton>
            {source && (
              <>
                <span>{source.name}</span>
                <Button disabled={busy} onClick={() => setSource(undefined)}>
                  Clear source
                </Button>
              </>
            )}
          </Inline>
          <Alert tone="info" live={false}>
            Your agent can obtain the starter and perform uploads through MCP.
            The starter download contains files to write locally, pinned package
            versions and step-by-step instructions.
          </Alert>
        </Stack>
      </CardBody>
    </Card>
  );
}
