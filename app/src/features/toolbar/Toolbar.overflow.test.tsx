// The reader segment when the window is narrow (docs/specs/ui.md, Toolbar):
// the search shrinks first, then the message actions clip from their end,
// and Settings with the window controls never shrink, so they stay in the
// window. happy-dom lays nothing out, so this checks the structure.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarProps } from "./Toolbar";

function toolbar(over: Partial<ToolbarProps> = {}) {
  const props: ToolbarProps = {
    sidebarWidth: 220,
    listWidth: 360,
    title: "Inbox",
    subtitle: "1,234 messages, 12 unread",
    syncing: false,
    selection: { count: 1, seen: true, flagColor: 0 },
    canArchive: true,
    moveTargets: [{ mailboxId: 1, label: "Inbox", depth: 0 }],
    maximized: false,
    search: <input aria-label="Search" />,
    onCommand: vi.fn(),
    ...over,
  };
  render(<Toolbar {...props} />);
  return props;
}

describe("Toolbar in a narrow window", () => {
  it("keeps Settings and the window controls in one group that never shrinks", () => {
    toolbar();
    const segment = document.querySelector('[data-segment="reader"]');
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(segment).not.toBeNull();
    expect(group).not.toBeNull();
    if (!group) return;
    expect(group.className).toContain("shrink-0");
    expect(segment?.lastElementChild).toBe(group);
    for (const name of ["Settings", "Minimize", "Maximize", "Close"]) {
      expect(group.contains(screen.getByRole("button", { name }))).toBe(true);
    }
  });

  it("puts the search before that group, to shrink from 220", () => {
    toolbar();
    const search = document.querySelector<HTMLElement>("[data-search-slot]");
    expect(search).not.toBeNull();
    expect(search?.className).toContain("w-[220px]");
    expect(search?.nextElementSibling?.hasAttribute("data-window-controls")).toBe(true);
    expect(search?.contains(screen.getByRole("textbox", { name: "Search" }))).toBe(true);
  });

  it("clips the message actions from their end", () => {
    toolbar();
    const actions = document.querySelector<HTMLElement>("[data-message-actions]");
    expect(actions).not.toBeNull();
    expect(actions?.className).toContain("min-w-0");
    expect(actions?.className).toContain("overflow-hidden");
    // Move is the last action, so clipping hides it first.
    const buttons = actions?.querySelectorAll("button") ?? [];
    expect(buttons[buttons.length - 1]?.getAttribute("aria-label")).toBe("Move");
  });
});
