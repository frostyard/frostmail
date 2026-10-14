// CONTRACT TEST for task card T-0146 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ScopeSheet } from "./ScopeSheet";

const buttons = () => screen.getAllByRole("button").map((b) => b.textContent);

describe("ScopeSheet", () => {
  it("asks which occurrences a change takes", () => {
    const onChoose = vi.fn();
    render(<ScopeSheet action="change" first={false} onChoose={onChoose} onCancel={() => {}} />);
    const dialog = screen.getByRole("dialog", { name: "Change Repeating Event" });
    expect(dialog.textContent).toContain(
      "Change only this event, this and all future events, or every event in the series?",
    );
    expect(buttons()).toEqual(["This Event", "All Future Events", "All Events", "Cancel"]);
    expect(document.activeElement?.textContent).toBe("This Event");
    fireEvent.click(screen.getByRole("button", { name: "All Future Events" }));
    expect(onChoose).toHaveBeenCalledWith("future");
  });

  it("leaves out All Future Events from the first occurrence, and says Delete for a delete", () => {
    const onChoose = vi.fn();
    render(<ScopeSheet action="delete" first onChoose={onChoose} onCancel={() => {}} />);
    expect(screen.getByRole("dialog", { name: "Delete Repeating Event" }).textContent).toContain(
      "Delete only this event, this and all future events, or every event in the series?",
    );
    expect(buttons()).toEqual(["This Event", "All Events", "Cancel"]);
    fireEvent.click(screen.getByRole("button", { name: "All Events" }));
    expect(onChoose).toHaveBeenCalledWith("all");
  });

  it("cancels with Cancel and Escape", () => {
    const onCancel = vi.fn();
    render(<ScopeSheet action="delete" first={false} onChoose={() => {}} onCancel={onCancel} />);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(2);
  });
});
