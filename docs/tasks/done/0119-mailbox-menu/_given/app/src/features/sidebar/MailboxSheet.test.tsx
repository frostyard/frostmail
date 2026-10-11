// CONTRACT TEST for task card T-0119 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { MailboxSheet, type MailboxSheetProps } from "./MailboxSheet";

function sheet(over: Partial<MailboxSheetProps> = {}) {
  const props: MailboxSheetProps = {
    mode: "new",
    name: "",
    locations: [
      { id: 7, path: "Projects" },
      { id: 8, path: "Projects/Frost" },
    ],
    onSave: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<MailboxSheet {...props} />);
  return props;
}

const name = () => screen.queryByLabelText("Name:") as HTMLInputElement | null;
const place = () => screen.queryByLabelText("Location:") as HTMLSelectElement | null;
const ok = () => screen.getByRole("button", { name: "OK" }) as HTMLButtonElement;

describe("MailboxSheet", () => {
  it("asks a new mailbox's name and place", () => {
    const props = sheet({ parentId: 7 });
    expect(screen.getByRole("dialog", { name: "New Mailbox" }).getAttribute("aria-modal")).toBe("true");
    expect(document.activeElement).toBe(name());
    expect(name()?.maxLength).toBe(100);
    expect(Array.from(place()?.options ?? [], (o) => [o.value, o.textContent])).toEqual([
      ["", "Top Level"],
      ["7", "Projects"],
      ["8", "Projects/Frost"],
    ]);
    expect(place()?.value).toBe("7");
    expect(ok().disabled).toBe(true);
    fireEvent.change(name() as HTMLInputElement, { target: { value: " Taxes " } });
    fireEvent.click(ok());
    expect(props.onSave).toHaveBeenLastCalledWith({ name: "Taxes", parentId: 7 });
    fireEvent.change(place() as HTMLSelectElement, { target: { value: "" } });
    fireEvent.click(ok());
    expect(props.onSave).toHaveBeenLastCalledWith({ name: "Taxes" });
  });

  it("asks only the name to rename, and only the place to move", () => {
    const rename = sheet({ mode: "rename", name: "Frost" });
    expect(screen.getByRole("dialog", { name: "Rename Mailbox" })).toBeTruthy();
    expect(place()).toBeNull();
    expect(name()?.value).toBe("Frost");
    fireEvent.change(name() as HTMLInputElement, { target: { value: "Ice" } });
    fireEvent.click(ok());
    expect(rename.onSave).toHaveBeenCalledWith({ name: "Ice" });
  });

  it("moves to a place", () => {
    const move = sheet({ mode: "move", name: "Frost", parentId: 7 });
    expect(screen.getByRole("dialog", { name: "Move Mailbox" })).toBeTruthy();
    expect(name()).toBeNull();
    expect(ok().disabled).toBe(false);
    fireEvent.change(place() as HTMLSelectElement, { target: { value: "" } });
    fireEvent.click(ok());
    expect(move.onSave).toHaveBeenCalledWith({ name: "Frost" });
  });

  it("waits while busy, shows maild's error, and cancels", () => {
    const props = sheet({ name: "Taxes", busy: true, error: 'a mailbox "Taxes" already exists' });
    expect(ok().disabled).toBe(true);
    expect(screen.getByRole("alert").textContent).toContain("already exists");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(props.onCancel).toHaveBeenCalledTimes(2);
  });
});
