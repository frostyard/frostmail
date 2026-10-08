// CONTRACT TEST for task card T-0032 (docs/tasks). Do not edit.
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
    moveTargets: [
      { mailboxId: 1, label: "Inbox", depth: 0 },
      { mailboxId: 7, label: "Projects", depth: 0 },
      { mailboxId: 8, label: "Frost", depth: 1 },
    ],
    maximized: false,
    search: <input aria-label="Search" />,
    onCommand: vi.fn(),
    ...over,
  };
  render(<Toolbar {...props} />);
  return props;
}

const button = (name: string) => screen.getByRole("button", { name });

describe("Toolbar", () => {
  it("is a draggable toolbar split at the pane widths", () => {
    toolbar();
    const bar = screen.getByRole("toolbar", { name: "Toolbar" });
    expect(bar.hasAttribute("data-tauri-drag-region")).toBe(true);
    expect(bar.className).toContain("h-[52px]");
    expect(bar.className).toContain("bg-toolbar");
    const segments = bar.querySelectorAll(":scope > [data-segment]");
    expect([...segments].map((s) => s.getAttribute("data-segment"))).toEqual(["sidebar", "list", "reader"]);
    expect((segments[0] as HTMLElement).style.width).toBe("221px");
    expect((segments[1] as HTMLElement).style.width).toBe("361px");
    for (const s of segments) expect(s.hasAttribute("data-tauri-drag-region")).toBe(true);
  });

  it("shows the title and subtitle", () => {
    toolbar();
    expect(screen.getByText("Inbox").className).toContain("text-toolbar-title");
    expect(screen.getByText("1,234 messages, 12 unread").className).toContain("text-toolbar-subtitle");
  });

  it("places the search slot", () => {
    toolbar();
    expect(screen.getByRole("textbox", { name: "Search" })).toBeTruthy();
  });

  it("sends commands from its buttons, with shortcuts in the tooltips", () => {
    const props = toolbar();
    const cases: [string, unknown][] = [
      ["Toggle Sidebar", { kind: "toggleSidebar" }],
      ["Get Mail", { kind: "getMail" }],
      ["Delete", { kind: "delete" }],
      ["Archive", { kind: "archive" }],
      ["Mark as Unread", { kind: "toggleRead" }],
      ["Minimize", { kind: "minimize" }],
      ["Maximize", { kind: "toggleMaximize" }],
      ["Close", { kind: "close" }],
    ];
    for (const [name, cmd] of cases) {
      fireEvent.click(button(name));
      expect(props.onCommand).toHaveBeenLastCalledWith(cmd);
    }
    expect(button("Delete").getAttribute("title")).toBe("Delete (Delete)");
    expect(button("Archive").getAttribute("title")).toBe("Archive (Ctrl+Alt+A)");
    expect(button("Get Mail").getAttribute("title")).toBe("Get Mail (Ctrl+Shift+N)");
    expect(button("Mark as Unread").getAttribute("title")).toBe("Mark as Unread (Ctrl+Shift+U)");
  });

  it("disables what needs a selection", () => {
    toolbar({ selection: { count: 0, seen: false, flagColor: 0 }, canArchive: false });
    for (const name of ["Delete", "Archive", "Flag", "Mark as Read", "Move", "Reply", "Reply All", "Forward"]) {
      expect(button(name).hasAttribute("disabled")).toBe(true);
    }
    for (const name of ["Toggle Sidebar", "Get Mail", "New Message", "Minimize", "Maximize", "Close"]) {
      expect(button(name).hasAttribute("disabled")).toBe(false);
    }
  });

  it("disables Archive without an archive mailbox", () => {
    toolbar({ canArchive: false });
    expect(button("Archive").hasAttribute("disabled")).toBe(true);
    expect(button("Delete").hasAttribute("disabled")).toBe(false);
  });

  it("offers flag colors in a menu", () => {
    const props = toolbar({ selection: { count: 2, seen: false, flagColor: 3 } });
    const flag = button("Flag");
    expect(flag.getAttribute("aria-haspopup")).toBe("menu");
    expect(flag.getAttribute("title")).toBe("Flag (Ctrl+Shift+L)");
    fireEvent.click(flag);
    // Colors are checkable (menuitemcheckbox); Clear Flag is a plain item.
    const colors = screen.getAllByRole("menuitemcheckbox").map((m) => m.textContent);
    expect(colors).toEqual(["Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Gray"]);
    expect(screen.getAllByRole("menuitem").map((m) => m.textContent)).toEqual(["Clear Flag"]);
    expect(screen.getByRole("menuitemcheckbox", { name: /Yellow/ }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("menuitemcheckbox", { name: /Red/ }).getAttribute("aria-checked")).toBe("false");
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: /Green/ }));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "flag", color: 4 });
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("disables Clear Flag when nothing is flagged", () => {
    toolbar();
    fireEvent.click(button("Flag"));
    expect(screen.getByRole("menuitem", { name: /Clear Flag/ }).getAttribute("aria-disabled")).toBe("true");
  });

  it("offers mailboxes in the Move menu", () => {
    const props = toolbar();
    fireEvent.click(button("Move"));
    const items = screen.getAllByRole("menuitem");
    expect(items.map((m) => m.textContent?.trim())).toEqual(["Inbox", "Projects", "Frost"]);
    fireEvent.click(screen.getByRole("menuitem", { name: /Frost/ }));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "move", mailboxId: 8 });
  });

  it("labels Mark by the selection and the window button by its state", () => {
    toolbar({ selection: { count: 1, seen: false, flagColor: 0 }, maximized: true });
    expect(button("Mark as Read")).toBeTruthy();
    expect(button("Restore")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Maximize" })).toBeNull();
  });

  it("spins Get Mail while syncing", () => {
    toolbar({ syncing: true });
    expect(button("Get Mail").querySelector("svg")?.getAttribute("class")).toContain("animate-spin");
  });

  it("leaves double clicks on the drag region to Tauri", () => {
    // Tauri's drag-region script maximizes on double click itself
    // (tauri/src/window/scripts/drag.js); sending toggleMaximize too would
    // undo it.
    const props = toolbar();
    fireEvent.doubleClick(screen.getByRole("toolbar", { name: "Toolbar" }));
    expect(props.onCommand).not.toHaveBeenCalled();
  });

  it("keeps the sidebar segment's buttons while the sidebar is hidden", () => {
    toolbar({ sidebarWidth: 0 });
    const segment = document.querySelector('[data-segment="sidebar"]') as HTMLElement;
    expect(segment.style.width).toBe("");
    expect(button("Toggle Sidebar")).toBeTruthy();
  });
});

describe("Toolbar compose actions", () => {
  it("composes, replies and forwards", () => {
    const props = toolbar();
    const cases: [string, string, string][] = [
      ["New Message", "compose", "New Message (Ctrl+N)"],
      ["Reply", "reply", "Reply (Ctrl+R)"],
      ["Reply All", "replyAll", "Reply All (Ctrl+Shift+R)"],
      ["Forward", "forward", "Forward (Ctrl+Shift+F)"],
    ];
    for (const [name, kind, title] of cases) {
      expect(button(name).getAttribute("title")).toBe(title);
      fireEvent.click(button(name));
      expect(props.onCommand).toHaveBeenLastCalledWith({ kind });
    }
  });
});
