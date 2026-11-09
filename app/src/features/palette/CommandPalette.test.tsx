// The command palette's sheet (docs/specs/ui.md, Command palette).
import { fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { paletteItems } from "../../lib/keymap";
import { CommandPalette } from "./CommandPalette";

function palette(selected = 1) {
  const onRun = vi.fn();
  const onClose = vi.fn();
  render(<CommandPalette items={paletteItems("mail", selected)} onRun={onRun} onClose={onClose} />);
  const field = screen.getByRole("combobox", { name: "Command" });
  const options = () => within(screen.getByRole("listbox", { name: "Commands" })).queryAllByRole("option");
  const type = (text: string) => fireEvent.change(field, { target: { value: text } });
  return { onRun, onClose, field, options, type };
}

const selected = (option: HTMLElement) => option.getAttribute("aria-selected") === "true";

/** Toggled opens the palette, and closes it on its own Escape. */
function Toggled() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      {open && <CommandPalette items={paletteItems("mail", 1)} onRun={vi.fn()} onClose={() => setOpen(false)} />}
    </>
  );
}

describe("the command palette", () => {
  it("finds a command by name, shows its keys, and runs it on Enter", () => {
    const { onRun, options, field, type } = palette();
    expect(screen.getByRole("dialog", { name: "Commands" })).toBeTruthy();
    expect(field.getAttribute("placeholder")).toBe("Find a Command");
    expect(field.getAttribute("aria-expanded")).toBe("true");
    expect(field.getAttribute("aria-controls")).toBe(screen.getByRole("listbox", { name: "Commands" }).id);

    type("arch");
    const rows = options();
    expect(rows).toHaveLength(1);
    const archive = rows[0] as HTMLElement;
    expect(archive.textContent).toContain("Archive");
    expect(archive.textContent).toContain("Ctrl+Alt+A");
    // The field names the highlighted row.
    expect(field.getAttribute("aria-activedescendant")).toBe(archive.id);
    expect(selected(archive)).toBe(true);

    fireEvent.keyDown(field, { key: "Enter" });
    expect(onRun).toHaveBeenCalledWith("archive");
  });

  it("moves the highlight with the arrows, and stops at the ends", () => {
    const { field, options, type } = palette();
    type("");
    fireEvent.keyDown(field, { key: "ArrowUp" });
    expect(selected(options()[0] as HTMLElement)).toBe(true);
    fireEvent.keyDown(field, { key: "ArrowDown" });
    fireEvent.keyDown(field, { key: "ArrowDown" });
    expect(selected(options()[2] as HTMLElement)).toBe(true);
    expect(selected(options()[0] as HTMLElement)).toBe(false);
    fireEvent.keyDown(field, { key: "ArrowUp" });
    expect(selected(options()[1] as HTMLElement)).toBe(true);
    expect(selected(options()[2] as HTMLElement)).toBe(false);
  });

  it("closes with Escape", () => {
    const { onClose, field } = palette();
    fireEvent.keyDown(field, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("runs a row that a click chooses", () => {
    const { onRun, options } = palette();
    const row = options().find((option) => option.textContent?.includes("New Message")) as HTMLElement;
    fireEvent.click(row);
    expect(onRun).toHaveBeenCalledWith("compose");
  });

  it("shows a command that needs a selection as disabled, and keeps it on Enter", () => {
    const { onRun, options, field, type } = palette(0);
    type("reply");
    const reply = options()[0] as HTMLElement;
    expect(reply.textContent).toContain("Reply");
    expect(reply.getAttribute("aria-disabled")).toBe("true");
    expect(reply.textContent).toContain("Select a message first");
    fireEvent.keyDown(field, { key: "Enter" });
    expect(onRun).not.toHaveBeenCalled();
    // A click on a disabled row does nothing either.
    fireEvent.click(reply);
    expect(onRun).not.toHaveBeenCalled();
  });

  it("shows no row for a query that matches nothing", () => {
    const { options, type } = palette();
    type("zzz");
    expect(options()).toHaveLength(0);
    expect(screen.getByText("No Commands")).toBeTruthy();
  });

  it("returns focus where it was when it closes", () => {
    render(<Toggled />);
    const button = screen.getByRole("button", { name: "Open" });
    button.focus();
    fireEvent.click(button);
    const field = screen.getByRole("combobox", { name: "Command" });
    expect(document.activeElement).toBe(field);
    fireEvent.keyDown(field, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Commands" })).toBeNull();
    expect(document.activeElement).toBe(button);
  });
});
