// Raw Source's Save As (docs/specs/ui.md, Reader: More Actions).
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { RawSourceSheet } from "./RawSourceSheet";

describe("RawSourceSheet's Save As", () => {
  it("saves, and Tab moves between it and Close", () => {
    const onSave = vi.fn();
    render(<RawSourceSheet text="x" truncated onClose={vi.fn()} onSave={onSave} />);
    const dialog = screen.getByRole("dialog", { name: "Raw Source" });
    const close = screen.getByRole("button", { name: "Close" });
    const save = screen.getByRole("button", { name: "Save As…" });
    expect(document.activeElement).toBe(close);
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(document.activeElement).toBe(save);
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(document.activeElement).toBe(close);
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledOnce();
  });

  it("is not offered without a way to save", () => {
    render(<RawSourceSheet text="x" truncated={false} onClose={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Save As…" })).toBeNull();
  });
});
