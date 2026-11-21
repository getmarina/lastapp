import { readdir, readFile } from "node:fs/promises";
import { resolve, relative, sep } from "node:path";
const directory = resolve(process.argv[2] || "dist"),
  endpoint = new URL(process.argv[3]);
if (
  endpoint.protocol !== "https:" &&
  !["127.0.0.1", "localhost", "[::1]"].includes(endpoint.hostname)
)
  throw new Error("Use HTTPS for remote uploads");
const token = process.env.LASTAPP_UPLOAD_TOKEN;
if (!token)
  throw new Error(
    "Supply the short-lived upload ticket in LASTAPP_UPLOAD_TOKEN",
  );
const files = [];
async function walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    if (entry.isSymbolicLink()) throw new Error("Symlinks are not supported");
    const name = resolve(dir, entry.name);
    if (entry.isDirectory()) await walk(name);
    else if (entry.isFile())
      files.push({
        path: relative(directory, name).split(sep).join("/"),
        content_base64: (await readFile(name)).toString("base64"),
      });
  }
}
await walk(directory);
const response = await fetch(endpoint, {
  method: "POST",
  headers: {
    Authorization: `Bearer ${token}`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    entrypoint: "index.html",
    files,
    source_zip: process.argv[4]
      ? (await readFile(process.argv[4])).toString("base64")
      : undefined,
  }),
});
const result = await response.json();
if (!response.ok)
  throw new Error(
    result.message ||
      result.error?.message ||
      `Upload failed (${response.status})`,
  );
console.log(JSON.stringify(result));
