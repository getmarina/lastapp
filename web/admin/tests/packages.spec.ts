import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import {
  choose,
  confirmDialog,
  fixtureTenant,
  login,
  logout,
  nav,
  post,
  selectTenant,
} from "./helpers";
import { e2eConfigPath } from "./stack";

test.use({ reducedMotion: "reduce" });

test("a Tenant installs a prebuilt package and retrieves its editable source", async ({
  page,
  context,
}) => {
  const dir = await mkdtemp(join(tmpdir(), "lastapp-package-"));
  const build = join(dir, "build");
  const source = join(dir, "source");
  const key = "sample" + crypto.randomUUID().replaceAll("-", "").slice(0, 10);
  const title = "Package " + key;
  try {
    await mkdir(build);
    await mkdir(source);
    const html =
      "<!doctype html><html><head><title>Fixture</title></head><body><h1>Installed micro-app</h1></body></html>";
    await writeFile(join(build, "index.html"), html);
    await writeFile(
      join(source, "package.json"),
      JSON.stringify({ name: key, version: "1.0.0" }),
    );
    await writeFile(
      join(source, "package-lock.json"),
      JSON.stringify({
        name: key,
        version: "1.0.0",
        lockfileVersion: 3,
        packages: {},
      }),
    );
    await writeFile(join(source, "index.html"), html);
    const archive = join(dir, "source.zip");
    execFileSync(
      "python3",
      [
        "-m",
        "zipfile",
        "-c",
        archive,
        "package.json",
        "package-lock.json",
        "index.html",
      ],
      { cwd: source },
    );
    const manifest = join(dir, "manifest.json");
    await writeFile(
      manifest,
      JSON.stringify({
        format: 1,
        key,
        version: "1.0.0",
        name: title,
        roles: [{ key: "reader", name: "Reader" }],
        entities: [
          {
            format_version: 1,
            key: "item",
            label: "Item",
            fields: [
              { key: "name", label: "Name", type: "text", max_length: 60 },
            ],
            rules: [],
            permissions: [
              {
                key: "read",
                label: "Read",
                operation: "read",
                roles: ["reader"],
                fields: ["name"],
              },
            ],
          },
        ],
        app: {
          key,
          name: title,
          access: { ceiling: [{ permission: "item/read", fields: ["name"] }] },
          microapps: [{ key: "items", name: "Items", version: "1.0.0" }],
        },
      }),
    );
    execFileSync(join(dirname(e2eConfigPath), "lastapp"), [
      "publish-package",
      "-config",
      e2eConfigPath,
      "-manifest",
      manifest,
      "-build",
      build,
      "-source",
      archive,
    ]);
    await login(page);
    const { tenant, base, owner } = await fixtureTenant(page, "Packages");
    await logout(page);
    await login(page, "tenant-admin");
    await selectTenant(page, tenant);
    const space = crypto.randomUUID();
    expect(
      (
        await post(page, base + "/spaces", {
          id: space,
          slug: "destination",
          display_name: "Destination",
          initial_manager_user_id: owner.id,
          database: { mode: "managed", layout: "shared" },
        })
      ).status,
    ).toBe(201);
    await nav(page, "Packages");
    const catalog = page.getByRole("table", { name: "Package catalog" });
    await catalog
      .getByRole("row", { name: new RegExp(title) })
      .getByRole("button", { name: "Configure installation" })
      .click();
    await choose(page, "Destination Space", "Destination", "Destination");
    await page.getByRole("combobox", { name: "Install mode" }).click();
    await page
      .getByRole("option", { name: "Business objects and App", exact: true })
      .click();
    await page.getByRole("button", { name: "Review changes" }).click();
    await expect(
      page.getByRole("button", { name: "Stage installation" }),
    ).toBeVisible();
    await expect
      .poll(async () =>
        page
          .locator("main")
          .evaluate(
            (main) =>
              main.getBoundingClientRect().left >=
              (document.querySelector(".sidebar")?.getBoundingClientRect()
                .right || 0),
          ),
      )
      .toBe(true);
    await page.screenshot({
      path: "test-results/package-review-desktop.png",
      fullPage: true,
      animations: "disabled",
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 390);
    await expect(
      page.getByRole("button", { name: "Open navigation" }),
    ).toHaveAttribute("aria-expanded", "false");

    await page.screenshot({
      path: "test-results/package-review-mobile.png",
      fullPage: true,
      animations: "disabled",
    });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.getByRole("button", { name: "Stage installation" }).click();
    const installations = page.getByRole("table", {
      name: "Package installations",
    });
    const preview = installations.getByRole("button", {
      name: "Prepare preview",
    });
    await expect(preview).toBeVisible({ timeout: 30_000 });
    for (const width of [1280, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await installations.getByRole("button", { name: "Discard" }).click();
      const dialog = page.getByRole("alertdialog");
      await expect(
        dialog.getByText("test data", { exact: false }),
      ).toBeVisible();
      await dialog.getByRole("button", { name: "Cancel" }).click();
    }
    await page.setViewportSize({ width: 1280, height: 900 });
    await preview.click();
    const launch = installations.getByRole("link", { name: "Open preview" });
    await expect(launch).toBeVisible();
    const app = await context.newPage();
    await app.goto((await launch.getAttribute("href"))!);
    await expect(
      app.getByRole("heading", { name: "Installed micro-app" }),
    ).toBeVisible();
    await app.close();
    await installations
      .getByRole("button", { name: "Activate", exact: true })
      .click();
    await confirmDialog(page, "Activate");
    const manage = installations.getByRole("button", { name: "Manage App" });
    await expect(manage).toBeVisible({ timeout: 30_000 });
    await manage.click();
    const download = page.getByRole("link", { name: "Download source" });
    await expect(download).toBeVisible();
    expect(
      (await page.request.get((await download.getAttribute("href"))!)).ok(),
    ).toBeTruthy();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
