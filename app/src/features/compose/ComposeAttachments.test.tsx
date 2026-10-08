// CONTRACT TEST for task card T-0046 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { DraftAttachment } from "../../rpc/gen/api";
import { ComposeAttachments, type PendingAttachment } from "./ComposeAttachments";

const pdf: DraftAttachment = { id: 1, filename: "notes.pdf", contentType: "application/pdf", size: 1_200_000 };
const photo: DraftAttachment = { id: 2, filename: "photo.jpg", contentType: "image/jpeg", size: 3_400_000 };
const zip: DraftAttachment = { id: 3, filename: "code.zip", contentType: "application/zip", size: 900 };
const bin: DraftAttachment = { id: 4, filename: "data.bin", contentType: "application/octet-stream", size: 1 };
const text: DraftAttachment = { id: 5, filename: "readme.txt", contentType: "text/plain", size: 2_000 };
const tgz: DraftAttachment = { id: 6, filename: "src.tar.gz", contentType: "application/gzip", size: 5_000 };

function strip(attachments: DraftAttachment[], pending: PendingAttachment[] = [], limit = 25_000_000) {
  const onRemove = vi.fn();
  const view = render(
    <ComposeAttachments attachments={attachments} pending={pending} limit={limit} onRemove={onRemove} />,
  );
  return { onRemove, ...view };
}

function chip(filename: string): HTMLElement {
  const name = screen.getByTitle(filename);
  const el = name.closest("[data-attachment]");
  if (!(el instanceof HTMLElement)) throw new Error(`no chip for ${filename}`);
  return el;
}

describe("ComposeAttachments", () => {
  it("renders nothing without attachments", () => {
    const { container } = strip([]);
    expect(container.innerHTML).toBe("");
  });

  it("is a labeled region of chips with names and sizes", () => {
    strip([pdf, photo]);
    const region = screen.getByRole("region", { name: "Attachments" });
    expect(region.className).toContain("bg-banner");
    expect(chip("notes.pdf").textContent).toContain("1.2 MB");
    expect(chip("photo.jpg").textContent).toContain("3.4 MB");
    expect(screen.getByTitle("notes.pdf").className).toContain("max-w-[200px]");
    expect(screen.getByTitle("notes.pdf").className).toContain("truncate");
  });

  it("picks an icon by type", () => {
    strip([pdf, photo, zip, bin, text, tgz]);
    const icon = (name: string) => chip(name).querySelector("svg")?.getAttribute("class") ?? "";
    expect(icon("notes.pdf")).toContain("lucide-file-text");
    expect(icon("readme.txt")).toContain("lucide-file-text");
    expect(icon("photo.jpg")).toContain("lucide-file-image");
    expect(icon("code.zip")).toContain("lucide-file-archive");
    expect(icon("src.tar.gz")).toContain("lucide-file-archive");
    expect(icon("data.bin")).toMatch(/lucide-file(\s|$)/);
  });

  it("removes an attachment", () => {
    const { onRemove } = strip([pdf, photo]);
    fireEvent.click(screen.getByRole("button", { name: "Remove photo.jpg" }));
    expect(onRemove).toHaveBeenCalledWith(2);
  });

  it("shows files still being attached with a spinner and no size or Remove", () => {
    strip([pdf], [{ key: "p1", filename: "big.mov" }]);
    const busy = chip("big.mov");
    expect(busy.getAttribute("aria-busy")).toBe("true");
    expect(busy.querySelector("svg")?.getAttribute("class")).toContain("animate-spin");
    expect(busy.textContent).toBe("big.mov");
    expect(screen.queryByRole("button", { name: "Remove big.mov" })).toBeNull();
    expect(screen.getByText("1 file, 1.2 MB")).toBeTruthy();
  });

  it("shows only pending files before any is stored", () => {
    strip([], [{ key: "p1", filename: "big.mov" }]);
    expect(chip("big.mov")).toBeTruthy();
    expect(screen.queryByText(/files?, /)).toBeNull();
  });

  it("totals the attachments", () => {
    strip([pdf, photo, zip]);
    const total = screen.getByText("3 files, 4.6 MB");
    expect(total.className).toContain("text-secondary");
    expect(total.className).toContain("ml-auto");
  });

  it("marks a total over the limit", () => {
    strip([pdf, photo], [], 4_000_000);
    const total = screen.getByText("2 files, 4.6 MB — over the 4 MB limit");
    expect(total.className).toContain("text-flag-1");
    expect(total.className).not.toContain("text-secondary");
  });
});
