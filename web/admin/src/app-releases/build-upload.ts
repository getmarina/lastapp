import { api } from "../api";
import { refreshQueries } from "../query";
import type { UploadTicket } from "../api-types";

async function encode(file: File) {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let text = "";
  for (let i = 0; i < bytes.length; i += 8192)
    text += String.fromCharCode(...bytes.subarray(i, i + 8192));
  return btoa(text);
}

// Uploads a build folder through a bounded upload ticket and returns the new
// build's ID.
export async function uploadBuild(
  app: unknown,
  files: FileList,
  source?: File,
) {
  const ticket = await api<UploadTicket>(
      `/v1/apps/${app}/upload-tickets`,
      "POST",
      {},
    ),
    list = Array.from(files);
  if (
    list.length > ticket.limits.max_files ||
    list.some((f) => f.size > ticket.limits.max_file_bytes) ||
    list.reduce((n, f) => n + f.size, 0) > ticket.limits.max_bytes
  )
    throw new Error("The build exceeds the upload limits.");
  if (source && source.size > ticket.limits.max_source_bytes)
    throw new Error("The source archive exceeds the upload limit.");
  const content = await Promise.all(
    list.map(async (f) => ({
      path: f.webkitRelativePath.split("/").slice(1).join("/"),
      content_base64: await encode(f),
    })),
  );
  const response = await fetch(ticket.upload_url, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${ticket.upload_token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      entrypoint: "index.html",
      files: content,
      source_zip: source ? await encode(source) : undefined,
    }),
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error?.message || "Upload failed");
  void refreshQueries();
  return result.id;
}
