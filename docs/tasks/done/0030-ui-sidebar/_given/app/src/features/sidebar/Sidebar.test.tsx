// CONTRACT TEST for task card T-0030 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SidebarItem, SidebarSection } from "../../lib/mailboxTree";
import { Sidebar, type SidebarProps } from "./Sidebar";

const item = (key: string, label: string, over: Partial<SidebarItem> = {}): SidebarItem => ({
  key,
  label,
  icon: "folder",
  depth: 0,
  unread: 0,
  selectable: true,
  ...over,
});

const sections: SidebarSection[] = [
  {
    key: "favorites",
    title: "Favorites",
    items: [
      item("all-inboxes", "All Inboxes", { icon: "inbox", unread: 7 }),
      item("flagged", "Flagged", { icon: "flag" }),
    ],
  },
  {
    key: "account:1",
    title: "test1@mailtest.test",
    accountId: 1,
    items: [
      item("mailbox:1", "Inbox", { icon: "inbox", unread: 5, mailboxId: 1 }),
      item("mailbox:5", "Trash", { icon: "trash-2", mailboxId: 5 }),
      item("path:1:Travel", "Travel", { selectable: false }),
      item("mailbox:9", "2026", { depth: 1, unread: 2, mailboxId: 9 }),
    ],
  },
];

function sidebar(over: Partial<SidebarProps> = {}) {
  const props: SidebarProps = {
    sections,
    selectedKey: "mailbox:1",
    focused: false,
    sync: {},
    onSelect: vi.fn(),
    ...over,
  };
  render(<Sidebar {...props} />);
  return props;
}

const rowOf = (name: string) => screen.getByRole("treeitem", { name: new RegExp(`^${name}`) });

describe("Sidebar", () => {
  it("renders a focusable tree with section headers and rows", () => {
    sidebar();
    const tree = screen.getByRole("tree", { name: "Mailboxes" });
    expect(tree.getAttribute("tabindex")).toBe("0");
    expect(screen.getByRole("button", { name: "Favorites" }).getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("button", { name: /test1@mailtest.test/ }).getAttribute("aria-expanded")).toBe("true");
    expect(screen.getAllByRole("treeitem").map((r) => r.getAttribute("data-key"))).toEqual([
      "all-inboxes",
      "flagged",
      "mailbox:1",
      "mailbox:5",
      "path:1:Travel",
      "mailbox:9",
    ]);
  });

  it("shows icons, indentation and unread counts", () => {
    sidebar();
    expect(rowOf("Trash").querySelector("svg")?.getAttribute("data-icon")).toBe("trash-2");
    expect(rowOf("All Inboxes").querySelector("svg")?.getAttribute("data-icon")).toBe("inbox");
    expect(rowOf("Inbox").style.paddingLeft).toBe("4px");
    expect(rowOf("2026").style.paddingLeft).toBe("20px");
    expect(rowOf("2026").getAttribute("aria-level")).toBe("2");
    expect(within(rowOf("Inbox")).getByText("5").className).toContain("tabular-nums");
    expect(within(rowOf("Trash")).queryByText("0")).toBeNull();
  });

  it("marks the selected row, in the accent color only with focus", () => {
    sidebar({ focused: true });
    expect(rowOf("Inbox").getAttribute("aria-selected")).toBe("true");
    expect(rowOf("Inbox").className).toContain("bg-accent");
    expect(rowOf("Trash").getAttribute("aria-selected")).toBe("false");
  });

  it("uses the sidebar selection color without focus", () => {
    sidebar({ focused: false });
    expect(rowOf("Inbox").className).toContain("bg-selection-sidebar");
    expect(rowOf("Inbox").className).not.toContain("bg-accent");
  });

  it("selects rows on click, except parents that are not mailboxes", () => {
    const props = sidebar();
    fireEvent.click(rowOf("Trash"));
    expect(props.onSelect).toHaveBeenCalledWith("mailbox:5");
    expect(rowOf("Travel").getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(rowOf("Travel"));
    expect(props.onSelect).toHaveBeenCalledTimes(1);
  });

  it("collapses and expands sections", () => {
    sidebar();
    const header = screen.getByRole("button", { name: /test1@mailtest.test/ });
    fireEvent.click(header);
    expect(header.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("treeitem", { name: /^Trash/ })).toBeNull();
    fireEvent.click(header);
    expect(rowOf("Trash")).toBeTruthy();
  });

  it("moves the selection with the arrow keys, skipping parents and collapsed sections", () => {
    const onSelect = vi.fn();
    sidebar({ selectedKey: "mailbox:5", onSelect });
    const tree = screen.getByRole("tree", { name: "Mailboxes" });
    const down = fireEvent.keyDown(tree, { key: "ArrowDown" });
    expect(down).toBe(false);
    expect(onSelect).toHaveBeenLastCalledWith("mailbox:9");
    fireEvent.keyDown(tree, { key: "ArrowUp" });
    expect(onSelect).toHaveBeenLastCalledWith("mailbox:1");
    fireEvent.keyDown(tree, { key: "Home" });
    expect(onSelect).toHaveBeenLastCalledWith("all-inboxes");
    fireEvent.keyDown(tree, { key: "End" });
    expect(onSelect).toHaveBeenLastCalledWith("mailbox:9");
    fireEvent.click(screen.getByRole("button", { name: "Favorites" }));
    fireEvent.keyDown(tree, { key: "Home" });
    expect(onSelect).toHaveBeenLastCalledWith("mailbox:1");
  });

  it("stops at the ends and starts from the first row without a selection", () => {
    const onSelect = vi.fn();
    sidebar({ selectedKey: "mailbox:9", onSelect });
    const tree = screen.getByRole("tree", { name: "Mailboxes" });
    fireEvent.keyDown(tree, { key: "ArrowDown" });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("selects the first row on ArrowDown when nothing is selected", () => {
    const onSelect = vi.fn();
    sidebar({ selectedKey: null, onSelect });
    fireEvent.keyDown(screen.getByRole("tree", { name: "Mailboxes" }), { key: "ArrowDown" });
    expect(onSelect).toHaveBeenCalledWith("all-inboxes");
  });

  it("keeps arrow keys from reaching the window", () => {
    const onWindow = vi.fn();
    window.addEventListener("keydown", onWindow);
    sidebar();
    fireEvent.keyDown(screen.getByRole("tree", { name: "Mailboxes" }), { key: "ArrowDown" });
    fireEvent.keyDown(screen.getByRole("tree", { name: "Mailboxes" }), { key: "Delete" });
    window.removeEventListener("keydown", onWindow);
    expect(onWindow).toHaveBeenCalledTimes(1);
  });

  it("shows sync activity and errors next to the account", () => {
    sidebar({ sync: { 1: { state: "syncing" } } });
    expect(screen.getByLabelText("Syncing")).toBeTruthy();
  });

  it("shows a sync error with its message", () => {
    sidebar({ sync: { 1: { state: "error", message: "login rejected" } } });
    const err = screen.getByLabelText("Sync error");
    expect(err.closest("[title]")?.getAttribute("title")).toBe("login rejected");
  });
});
