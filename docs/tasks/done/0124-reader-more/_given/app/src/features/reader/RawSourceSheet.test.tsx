// CONTRACT TEST for task card T-0124 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { RawSourceSheet } from "./RawSourceSheet";

describe("RawSourceSheet", () => {
  it("shows the source, says when it is cut short, and closes", () => {
    const onClose = vi.fn();
    render(<RawSourceSheet text={"From: a@x.test\r\n\r\nHi"} truncated onClose={onClose} />);
    const dialog = screen.getByRole("dialog", { name: "Raw Source" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.querySelector("pre")?.textContent).toBe("From: a@x.test\r\n\r\nHi");
    expect(dialog.textContent).toContain("The message is longer than what is shown.");
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("says nothing more when it is whole", () => {
    render(<RawSourceSheet text="x" truncated={false} onClose={vi.fn()} />);
    expect(screen.getByRole("dialog").textContent).not.toContain("longer");
  });
});
