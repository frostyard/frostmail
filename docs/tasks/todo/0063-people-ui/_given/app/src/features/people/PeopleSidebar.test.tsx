// CONTRACT TEST for task card T-0063 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type AddressBookSection, PeopleSidebar, type PeopleSidebarProps } from "./PeopleSidebar";

const sections: AddressBookSection[] = [
  { accountId: 1, title: "Gmail", books: [{ id: 10, name: "Contacts", readOnly: false }] },
  {
    accountId: 2,
    title: "iCloud",
    books: [
      { id: 20, name: "Card", readOnly: false },
      { id: 21, name: "Shared", readOnly: true },
    ],
  },
];

function sidebar(over: Partial<PeopleSidebarProps> = {}) {
  const props: PeopleSidebarProps = { sections, selected: "all", focused: false, onSelect: vi.fn(), ...over };
  render(<PeopleSidebar {...props} />);
  return props;
}

const row = (name: string) => screen.getByRole("treeitem", { name: new RegExp(`^${name}`) });

describe("PeopleSidebar", () => {
  it("lists All Contacts, then each account's address books", () => {
    sidebar();
    const tree = screen.getByRole("tree", { name: "Address Books" });
    expect(tree.getAttribute("tabindex")).toBe("0");
    expect(within(tree).getAllByRole("treeitem").map((r) => r.getAttribute("data-key"))).toEqual([
      "all",
      "book:10",
      "book:20",
      "book:21",
    ]);
    for (const title of ["People", "Gmail", "iCloud"]) {
      expect(screen.getByRole("button", { name: title }).getAttribute("aria-expanded")).toBe("true");
    }
    expect(row("All Contacts").textContent).toBe("All Contacts");
  });

  it("marks read-only address books", () => {
    sidebar();
    expect(within(row("Shared")).getByLabelText("Read-only")).toBeTruthy();
    expect(within(row("Card")).queryByLabelText("Read-only")).toBeNull();
  });

  it("shows the selection in the accent color while focused, gray otherwise", () => {
    sidebar({ selected: 20, focused: true });
    expect(row("Card").getAttribute("aria-selected")).toBe("true");
    expect(row("Card").className).toContain("bg-accent");
    expect(row("All Contacts").getAttribute("aria-selected")).toBe("false");
  });

  it("shows an unfocused selection in the sidebar's gray", () => {
    sidebar({ selected: "all" });
    expect(row("All Contacts").className).toContain("bg-selection-sidebar");
  });

  it("selects by click and by arrow keys", () => {
    const props = sidebar({ selected: 10 });
    fireEvent.click(row("Shared"));
    expect(props.onSelect).toHaveBeenLastCalledWith(21);
    fireEvent.click(row("All Contacts"));
    expect(props.onSelect).toHaveBeenLastCalledWith("all");
    const tree = screen.getByRole("tree", { name: "Address Books" });
    fireEvent.keyDown(tree, { key: "ArrowDown" });
    expect(props.onSelect).toHaveBeenLastCalledWith(20);
    fireEvent.keyDown(tree, { key: "ArrowUp" });
    expect(props.onSelect).toHaveBeenLastCalledWith("all");
    fireEvent.keyDown(tree, { key: "End" });
    expect(props.onSelect).toHaveBeenLastCalledWith(21);
  });

  it("collapses a section, which the arrow keys then skip", () => {
    const props = sidebar({ selected: 10 });
    fireEvent.click(screen.getByRole("button", { name: "iCloud" }));
    expect(screen.getByRole("button", { name: "iCloud" }).getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("treeitem", { name: /^Card/ })).toBeNull();
    fireEvent.keyDown(screen.getByRole("tree", { name: "Address Books" }), { key: "ArrowDown" });
    expect(props.onSelect).not.toHaveBeenCalled();
  });
});
