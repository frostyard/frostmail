// CONTRACT TEST for task card T-0064 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarProps } from "./Toolbar";

function toolbar(over: Partial<ToolbarProps> = {}) {
  const props: ToolbarProps = {
    sidebarWidth: 220,
    listWidth: 360,
    title: "All Contacts",
    subtitle: "12 contacts",
    syncing: false,
    selection: { count: 0, seen: true, flagColor: 0 },
    canArchive: false,
    moveTargets: [],
    maximized: false,
    search: <input aria-label="Search" />,
    onCommand: vi.fn(),
    ...over,
  };
  render(<Toolbar {...props} />);
  return props;
}

const MAIL_ONLY = ["Get Mail", "Delete", "Archive", "Reply", "Reply All", "Forward", "Flag", "Move"];

describe("Toolbar in People", () => {
  it("keeps the sidebar toggle, the title, New Message, search, Settings and the window controls", () => {
    toolbar({ mode: "people" });
    for (const name of ["Toggle Sidebar", "New Message", "Settings", "Minimize", "Close"]) {
      expect(screen.getByRole("button", { name })).toBeTruthy();
    }
    expect(screen.getByText("All Contacts")).toBeTruthy();
    expect(screen.getByText("12 contacts")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Search" })).toBeTruthy();
  });

  it("drops Mail's actions", () => {
    toolbar({ mode: "people" });
    for (const name of MAIL_ONLY) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Mark as/ })).toBeNull();
  });

  it("keeps them in Mail, the default", () => {
    toolbar();
    for (const name of MAIL_ONLY) expect(screen.getByRole("button", { name })).toBeTruthy();
  });
});
