import { describe, expect, it } from "vitest";

import type { Client, DraftAttachDataParams } from "../rpc/gen/api";
import { base64, UPLOAD_CHUNK, uploadFile } from "./upload";

function fakeClient(calls: DraftAttachDataParams[]) {
  let received = 0;
  return {
    draft: {
      attachData: async (p: DraftAttachDataParams) => {
        calls.push(p);
        received = p.offset + atob(p.data).length;
        return received === p.size
          ? { uploadId: "u1", received, attachment: { id: 9, filename: p.filename, contentType: "x/y", size: p.size } }
          : { uploadId: "u1", received };
      },
    },
  } as unknown as Client;
}

describe("uploadFile", () => {
  it("sends a file in order, in chunks, and answers the attachment", async () => {
    const calls: DraftAttachDataParams[] = [];
    const bytes = new Uint8Array(UPLOAD_CHUNK * 2 + 10).map((_, i) => i % 251);
    const progress: number[] = [];
    const a = await uploadFile(fakeClient(calls), 3, new File([bytes], "big.bin"), (n) => progress.push(n));
    expect(a).toMatchObject({ id: 9, filename: "big.bin", size: bytes.length });
    expect(calls.map((c) => [c.offset, c.uploadId])).toEqual([
      [0, undefined],
      [UPLOAD_CHUNK, "u1"],
      [UPLOAD_CHUNK * 2, "u1"],
    ]);
    expect(progress).toEqual([UPLOAD_CHUNK, UPLOAD_CHUNK * 2]);
    const sent = calls.map((c) => atob(c.data)).join("");
    expect(sent.length).toBe(bytes.length);
    expect(sent.charCodeAt(UPLOAD_CHUNK + 5)).toBe((UPLOAD_CHUNK + 5) % 251);
  });

  it("refuses an empty file without asking maild", async () => {
    const calls: DraftAttachDataParams[] = [];
    await expect(uploadFile(fakeClient(calls), 3, new File([], "empty.txt"))).rejects.toThrow("empty");
    expect(calls).toHaveLength(0);
  });

  it("encodes base64 as atob decodes it", () => {
    const bytes = new Uint8Array(70_000).map((_, i) => (i * 7) % 256);
    const back = atob(base64(bytes));
    expect(back.length).toBe(bytes.length);
    expect(back.charCodeAt(69_999)).toBe(bytes[69_999]);
  });
});
