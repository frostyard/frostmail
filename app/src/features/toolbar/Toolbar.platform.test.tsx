// The toolbar's window controls on each platform (docs/specs/ui.md, Layout:
// Toolbar): lodge-ui's `WindowControls` draws Windows' caption buttons on
// Windows, 46 wide and the strip's full height at its right edge, and the
// buttons it drew before on Linux (ADR-0038). The platform is the toolbar's
// prop here, as a test cannot change the navigator's.
import { windowPlatform } from "@frostyard/lodge-ui/react";
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

const button = (name: string) => screen.getByRole("button", { name });

describe("Toolbar's window controls on Windows", () => {
  it("draws Windows' three caption buttons in the window controls' group", () => {
    toolbar({ platform: "windows" });
    const controls = screen.getByRole("group", { name: "Window controls" });
    expect([...controls.querySelectorAll("button")].map((el) => el.getAttribute("aria-label"))).toEqual([
      "Minimize",
      "Maximize",
      "Close",
    ]);
    // A group that fills the strip, with no gap between the buttons.
    expect(controls.className).toContain("items-stretch");
    expect(controls.className).toContain("h-full");
    expect(controls.className).not.toContain("gap-1");
  });

  it("makes each caption button 46 wide, and Settings keeps the group", () => {
    toolbar({ platform: "windows" });
    const controls = screen.getByRole("group", { name: "Window controls" });
    for (const name of ["Minimize", "Maximize", "Close"]) {
      const el = button(name);
      expect(el.className).toContain("w-[46px]");
      expect(el.className).toContain("h-full");
      expect(controls.contains(el)).toBe(true);
    }
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(group?.contains(button("Settings"))).toBe(true);
    expect(controls.contains(button("Settings"))).toBe(false);
  });

  it("fills the strip's height, and keeps the drag region off the buttons", () => {
    toolbar({ platform: "windows" });
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(group?.className).toContain("h-full");
    expect(group?.hasAttribute("data-tauri-drag-region")).toBe(true);
    for (const name of ["Minimize", "Maximize", "Close", "Settings"]) {
      expect(button(name).hasAttribute("data-tauri-drag-region")).toBe(false);
    }
    // No padding after Close, so the last caption button reaches the window's edge.
    const segment = document.querySelector<HTMLElement>('[data-segment="reader"]');
    expect(segment?.className).toContain("pl-2");
    expect(segment?.className).not.toContain("px-2");
    expect(segment?.lastElementChild).toBe(group);
  });

  it("keeps the Linux padding when no window controls are drawn, as in a browser", () => {
    // A browser on Windows draws no window buttons (the web build), so Settings
    // must not sit at the window's edge.
    toolbar({ platform: "windows", windowControls: false });
    expect(screen.queryByRole("group", { name: "Window controls" })).toBeNull();
    const segment = document.querySelector<HTMLElement>('[data-segment="reader"]');
    expect(segment?.className).toContain("px-2");
    expect(segment?.className).not.toContain("pl-2");
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(group?.className).toBe("flex shrink-0 items-center gap-1");
    expect(group?.contains(button("Settings"))).toBe(true);
  });

  it("says Restore when the window is maximized, and sends its commands", () => {
    const props = toolbar({ platform: "windows", maximized: true });
    const controls = screen.getByRole("group", { name: "Window controls" });
    expect(controls.contains(button("Restore"))).toBe(true);
    expect(screen.queryByRole("button", { name: "Maximize" })).toBeNull();
    fireEvent.click(button("Close"));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "close" });
  });
});

describe("Toolbar's window controls on Linux", () => {
  it("keeps the classes it drew before, and the strip's padding", () => {
    toolbar({ platform: "linux" });
    const controls = screen.getByRole("group", { name: "Window controls" });
    expect(controls.className).toBe("flex shrink-0 items-center gap-1");
    for (const name of ["Minimize", "Maximize", "Close"]) {
      const el = button(name);
      expect(el.className).toContain("size-7");
      expect(el.className).toContain("rounded-md");
      expect(el.className).not.toContain("w-[46px]");
      expect(controls.contains(el)).toBe(true);
    }
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(group?.className).toBe("flex shrink-0 items-center gap-1");
    expect(group?.hasAttribute("data-tauri-drag-region")).toBe(true);
    const segment = document.querySelector<HTMLElement>('[data-segment="reader"]');
    expect(segment?.className).toContain("px-2");
    expect(segment?.className).not.toContain("pl-2");
  });

  it("is what no platform prop draws, since the test's navigator is not Windows", () => {
    expect(windowPlatform()).toBe("linux");
    toolbar();
    const controls = screen.getByRole("group", { name: "Window controls" });
    expect(controls.className).toBe("flex shrink-0 items-center gap-1");
    for (const name of ["Minimize", "Maximize", "Close"]) {
      expect(button(name).className).toContain("size-7");
    }
    const group = document.querySelector<HTMLElement>("[data-window-controls]");
    expect(group?.className).not.toContain("h-full");
  });
});
