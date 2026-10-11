// CONTRACT TEST for task card T-0122 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SortMenu, type SortMenuProps } from "./SortMenu";

function menu(over: Partial<SortMenuProps> = {}) {
  const props: SortMenuProps = {
    sort: "date",
    ascending: false,
    conversations: true,
    contactPhotos: false,
    onSort: vi.fn(),
    onAscending: vi.fn(),
    onConversations: vi.fn(),
    onContactPhotos: vi.fn(),
    ...over,
  };
  render(<SortMenu {...props} />);
  return props;
}

function open(): HTMLElement {
  fireEvent.click(screen.getByRole("button", { name: /^Sort by / }));
  return screen.getByRole("menu");
}

/** entries lists a menu's rows: "—" for separators, a ✓ before checked ones. */
function entries(m: HTMLElement): string[] {
  return Array.from(m.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) =>
      el.tagName === "HR"
        ? "—"
        : `${el.getAttribute("aria-checked") === "true" ? "✓ " : ""}${el.querySelector(".flex-1")?.textContent ?? ""}`,
    );
}

describe("SortMenu", () => {
  it("names the sort and lists the choices", () => {
    menu({ sort: "subject", ascending: true });
    expect(screen.getByRole("button", { name: /^Sort by / }).textContent).toBe("Sort by Subject");
    expect(entries(open())).toEqual([
      "Date",
      "From",
      "To",
      "✓ Subject",
      "Size",
      "Flags",
      "Unread",
      "Attachments",
      "—",
      "✓ Ascending",
      "Descending",
      "—",
      "✓ Conversations",
      "Contact Photos",
    ]);
  });

  it("reports each choice", () => {
    const props = menu();
    fireEvent.click(within(open()).getByText("Size"));
    expect(props.onSort).toHaveBeenCalledWith("size");
    fireEvent.click(within(open()).getByText("Ascending"));
    expect(props.onAscending).toHaveBeenCalledWith(true);
    fireEvent.click(within(open()).getByText("Conversations"));
    expect(props.onConversations).toHaveBeenCalledWith(false);
    fireEvent.click(within(open()).getByText("Contact Photos"));
    expect(props.onContactPhotos).toHaveBeenCalledWith(true);
  });
});
