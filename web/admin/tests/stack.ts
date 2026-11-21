import { readFileSync } from "node:fs";

// The disposable stack started by scripts/e2e.py (`make e2e`). Browser
// regressions never run against the development stack and its data, so there
// is no fallback to the development ports.
const file = process.env.LASTAPP_E2E;
if (!file)
  throw new Error(
    "Browser regressions run on a disposable database: use `make e2e` (scripts/e2e.py), not `npm run test:e2e` directly.",
  );
const e2e = JSON.parse(readFileSync(file, "utf8")) as {
  config: string;
  app_port: number;
  root_sql: string[];
};
const config = JSON.parse(readFileSync(e2e.config, "utf8")) as {
  public_url: string;
  development_ui_origin: string;
  runtime_account_host: string;
};

export const e2eConfigPath = e2e.config;
export const apiOrigin = config.public_url;
export const uiOrigin = config.development_ui_origin;
// Database account host the server provisions with ('%' for the container).
export const accountHost = config.runtime_account_host;
// Test-owned static server for the example data App.
export const appPort = e2e.app_port;
export const appOrigin = "http://localhost:" + appPort;
// argv that runs SQL as MariaDB root on the disposable database.
export const rootSQLCommand = e2e.root_sql;
