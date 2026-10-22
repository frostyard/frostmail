// The reader segment when the window is narrow (docs/specs/ui.md, Toolbar):
// the search shrinks first, then the message actions that no longer fit move
// into a » menu from their end, and Settings with the window controls never
// shrink, so they stay in the window. happy-dom lays nothing out, so the
// measured tests supply the container's width and the actions' widths.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarProps } from "./Toolbar";

const ACTION_NAMES = ["Delete", "Archive", "Reply", "Reply All", "Forward", "Flag", "Mark as Unread", "Move"];

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

/** layout gives the actions' container a width, and each measured action 28. */
function layout(containerWidth: number) {
  const disconnect = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect = disconnect;
    },
  );
  const computedStyle = window.getComputedStyle.bind(window);
  vi.spyOn(window, "getComputedStyle").mockImplementation((element) => {
    const style = computedStyle(element);
    if (element instanceof HTMLElement && element.hasAttribute("data-message-actions")) {
      const measured = document.createElement("div").style;
      measured.cssText = style.cssText;
      measured.columnGap = "4px";
      return measured;
    }
    return style;
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    if (this.hasAttribute("data-message-actions")) return new DOMRect(0, 0, containerWidth, 28);
    if (this.closest("[inert]")) return new DOMRect(0, 0, 28, 28);
    return new DOMRect(0, 0, 0, 28);
  });
  return { disconnect };
}

/** actionNames lists the visible actions, without the hidden measuring row. */
function actionNames() {
  const actions = document.querySelector<HTMLElement>("[data-message-actions]");
  if (!actions) return [];
  return within(actions)
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? "");
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

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
    // The three window buttons are the package's WindowControls: a named group inside this one, which keeps Settings.
    const controls = screen.getByRole("group", { name: "Window controls" });
    expect(group.contains(controls)).toBe(true);
    expect(controls.className).toContain("shrink-0");
    expect(controls.contains(screen.getByRole("button", { name: "Settings" }))).toBe(false);
    for (const name of ["Minimize", "Maximize", "Close"]) {
      expect(controls.contains(screen.getByRole("button", { name }))).toBe(true);
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

  it("shows every message action, and no », before layout", () => {
    toolbar();
    expect(actionNames()).toEqual(ACTION_NAMES);
    expect(screen.queryByRole("button", { name: "More Toolbar Items" })).toBeNull();
  });

  it("keeps every action when the measured room holds them all", () => {
    layout(252);
    toolbar();
    expect(actionNames()).toEqual(ACTION_NAMES);
    expect(screen.queryByRole("button", { name: "More Toolbar Items" })).toBeNull();
  });

  it("moves the actions after Archive into » at 96px, and runs them from the menu", () => {
    layout(96);
    const props = toolbar();
    expect(actionNames()).toEqual(["Delete", "Archive", "More Toolbar Items"]);
    fireEvent.click(screen.getByRole("button", { name: "More Toolbar Items" }));
    const menu = screen.getByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.getAttribute("aria-label")),
    ).toEqual(["Reply", "Reply All", "Forward", "Flag", "Mark as Unread", "Move"]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Reply" }));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "reply" });

    fireEvent.click(screen.getByRole("button", { name: "More Toolbar Items" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Flag" }));
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Green" }));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "flag", color: 4 });
  });

  it("keeps the » inside the actions, and the window controls last in the segment", () => {
    layout(96);
    toolbar();
    const actions = document.querySelector<HTMLElement>("[data-message-actions]");
    expect(actions?.contains(screen.getByRole("button", { name: "More Toolbar Items" }))).toBe(true);
    const segment = document.querySelector('[data-segment="reader"]');
    expect(segment?.lastElementChild?.hasAttribute("data-window-controls")).toBe(true);
  });
});
