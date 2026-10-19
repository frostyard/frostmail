// Attaching a file by its bytes (draft.attachData; docs/specs/compose-ui.md,
// Attachments): for a browser through lodge, or the desktop app on a maild
// behind lodge, neither of which can name a path on maild's machine. The
// file goes in chunks, in order; maild checks its name, size and type.
import type { Client, DraftAttachment } from "../rpc/gen/api";

/** UPLOAD_CHUNK is one chunk's bytes: its base64 stays within maild's 4 MiB a chunk. */
export const UPLOAD_CHUNK = 3 << 20;

/** base64 encodes bytes with the standard alphabet, padded. */
export function base64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

/** uploadFile attaches file to draft draftId, reporting the bytes maild holds after each chunk. */
export async function uploadFile(
  client: Client,
  draftId: number,
  file: Blob & { name: string },
  onProgress?: (received: number) => void,
): Promise<DraftAttachment> {
  const size = file.size;
  if (size === 0) throw new Error(`${file.name} is empty`);
  let uploadId: string | undefined;
  let offset = 0;
  for (;;) {
    const bytes = new Uint8Array(await file.slice(offset, Math.min(offset + UPLOAD_CHUNK, size)).arrayBuffer());
    const r = await client.draft.attachData({
      id: draftId,
      filename: file.name,
      size,
      offset,
      data: base64(bytes),
      ...(uploadId === undefined ? {} : { uploadId }),
    });
    if (r.attachment) return r.attachment;
    if (r.received <= offset) throw new Error(`maild took nothing of ${file.name} at ${offset}`);
    uploadId = r.uploadId;
    offset = r.received;
    onProgress?.(offset);
  }
}
