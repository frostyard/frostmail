// The toolbar's Commands button (docs/specs/ui.md, Command palette: The web
// build): the web build draws it, the desktop app does not.
import { fireEvent, render, screen } from "@testing-library/react";
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

describe("the toolbar's Commands button", () => {
  it("is absent without the palette prop", () => {
    toolbar();
    expect(screen.queryByRole("button", { name: "Commands" })).toBeNull();
  });

  it("opens the palette, just before Settings", () => {
    const props = toolbar({ palette: true });
    const button = screen.getByRole("button", { name: "Commands" });
    expect(screen.getByRole("toolbar", { name: "Toolbar" })).toBeTruthy();
    expect(button.getAttribute("title")).toBe("Commands (Ctrl+Shift+P)");
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(group?.contains(button)).toBe(true);
    expect(button.nextElementSibling).toBe(screen.getByRole("button", { name: "Settings" }));

    fireEvent.click(button);
    expect(props.onCommand).toHaveBeenCalledWith({ kind: "palette" });
  });

  it("is not in a narrow window's toolbar", () => {
    toolbar({ palette: true, narrow: { pane: "list", onBack: vi.fn() } });
    expect(screen.queryByRole("button", { name: "Commands" })).toBeNull();
  });
});
