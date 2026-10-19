// Saving attachments, where the reader offers it (lodge's web build).
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Part } from "../../rpc/gen/api";
import { AttachmentStrip } from "./MessageHeader";

const pdf: Part = {
  path: "2",
  contentType: "application/pdf",
  filename: "contract-signed.pdf",
  disposition: "attachment",
  contentId: "",
  size: 1000,
};

describe("AttachmentStrip's Save", () => {
  it("is offered beside each attachment when the reader can save", () => {
    const onOpen = vi.fn();
    const onSave = vi.fn();
    render(<AttachmentStrip parts={[pdf]} onOpen={onOpen} onSave={onSave} />);
    fireEvent.click(screen.getByRole("button", { name: "Save contract-signed.pdf" }));
    expect(onSave).toHaveBeenCalledWith(pdf);
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("is not offered otherwise", () => {
    render(<AttachmentStrip parts={[pdf]} onOpen={() => {}} />);
    expect(screen.queryByRole("button", { name: /^Save/ })).toBeNull();
  });
});
