// CONTRACT TEST for task card T-0137 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { PhotoWell } from "./PhotoWell";
import { RefusedNotice, refusedText } from "./RefusedNotice";

describe("PhotoWell", () => {
  it("picks a JPEG or PNG file", () => {
    const onPick = vi.fn();
    render(<PhotoWell name="Ann Smith" email="ann@x.test" onPick={onPick} />);
    expect(screen.getByRole("button", { name: "Change Photo" })).toBeTruthy();
    const input = screen.getByLabelText("Photo file") as HTMLInputElement;
    expect(input.type).toBe("file");
    expect(input.accept).toBe("image/jpeg,image/png");
    const file = new File(["jpeg"], "ann.jpg", { type: "image/jpeg" });
    fireEvent.change(input, { target: { files: [file] } });
    expect(onPick).toHaveBeenCalledWith(file);
    expect(screen.queryByRole("button", { name: "Remove Photo" })).toBeNull();
  });

  it("removes a photo it shows", () => {
    const onRemove = vi.fn();
    render(
      <PhotoWell
        name="Ann Smith"
        email="ann@x.test"
        photo="data:image/png;base64,AA=="
        onPick={vi.fn()}
        onRemove={onRemove}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Remove Photo" }));
    expect(onRemove).toHaveBeenCalled();
  });
});

describe("RefusedNotice", () => {
  it("says what became of the change", () => {
    expect(refusedText("Ann Smith", "changed")).toBe(
      "“Ann Smith” was changed on another device, so your change wasn't saved.",
    );
    expect(refusedText("Ann Smith", "deleted")).toBe("“Ann Smith” was deleted on another device.");
    expect(refusedText("Lunch", "refused")).toBe("The server didn't accept your change to “Lunch”.");
  });

  it("closes", () => {
    const onDismiss = vi.fn();
    render(<RefusedNotice text="gone" onDismiss={onDismiss} />);
    expect(screen.getByRole("status").textContent).toContain("gone");
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalled();
  });
});
