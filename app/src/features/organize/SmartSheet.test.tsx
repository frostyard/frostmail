// CONTRACT TEST for task card T-0108 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { newSmartDraft, type SmartDraft, SmartSheet, type SmartSheetProps } from "./SmartSheet";

function sheet(over: Partial<SmartSheetProps> = {}) {
  const props: SmartSheetProps = {
    title: "New Smart Mailbox",
    initial: newSmartDraft(),
    accounts: [],
    mailboxes: [],
    today: "2026-10-11",
    onSave: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<SmartSheet {...props} />);
  return props;
}

const name = () => screen.getByLabelText("Smart Mailbox Name:") as HTMLInputElement;
const ok = () => screen.getByRole("button", { name: "OK" }) as HTMLButtonElement;

describe("SmartSheet", () => {
  it("starts a new smart mailbox", () => {
    expect(newSmartDraft()).toEqual({
      name: "Smart Mailbox",
      conditions: { match: "all", conditions: [{ field: "from", op: "contains", value: "" }] },
      includeTrash: false,
      includeSent: false,
    });
    sheet();
    const dialog = screen.getByRole("dialog", { name: "New Smart Mailbox" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("New Smart Mailbox");
    expect(name().value).toBe("Smart Mailbox");
    expect(name().maxLength).toBe(100);
    expect(screen.getByRole("combobox", { name: "Match" })).not.toBeNull();
    expect((screen.getByRole("checkbox", { name: "Include messages from Trash" }) as HTMLInputElement).checked).toBe(
      false,
    );
  });

  it("saves the name, the conditions and the boxes", () => {
    const initial: SmartDraft = {
      name: "Lunch",
      conditions: { match: "all", conditions: [{ field: "content", op: "contains", value: "lunch" }] },
      includeTrash: true,
      includeSent: true,
    };
    const props = sheet({ title: "Edit Smart Mailbox", initial });
    fireEvent.change(name(), { target: { value: " Lunch plans " } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Include messages from Sent" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Condition 1 value" }), { target: { value: "plans" } });
    fireEvent.click(ok());
    expect(props.onSave).toHaveBeenCalledWith({
      name: "Lunch plans",
      conditions: { match: "all", conditions: [{ field: "content", op: "contains", value: "plans" }] },
      includeTrash: true,
      includeSent: false,
    });
  });

  it("needs a name, waits while busy, and shows maild's error", () => {
    sheet({ busy: true, error: "condition 1 is invalid" });
    expect(ok().disabled).toBe(true);
    expect(screen.getByRole("alert").textContent).toContain("condition 1 is invalid");
  });

  it("will not save a blank name", () => {
    sheet();
    fireEvent.change(name(), { target: { value: "   " } });
    expect(ok().disabled).toBe(true);
  });

  it("cancels with Cancel or Escape", () => {
    const props = sheet();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(props.onCancel).toHaveBeenCalledTimes(2);
    expect(props.onSave).not.toHaveBeenCalled();
  });
});
