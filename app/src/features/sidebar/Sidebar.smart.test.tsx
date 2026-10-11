// CONTRACT TEST for task card T-0108 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SidebarSection } from "../../lib/mailboxTree";
import { Sidebar } from "./Sidebar";

const sections: SidebarSection[] = [
  {
    key: "smart",
    title: "Smart Mailboxes",
    addLabel: "New Smart Mailbox",
    items: [{ key: "smart:1", label: "Today", icon: "folder-cog", depth: 0, unread: 2, selectable: true }],
  },
];

describe("the sidebar's smart mailboxes", () => {
  it("adds from the section header and keeps the header's toggle", () => {
    const onAdd = vi.fn();
    render(
      <Sidebar sections={sections} selectedKey={null} focused={false} sync={{}} onSelect={vi.fn()} onAdd={onAdd} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "New Smart Mailbox" }));
    expect(onAdd).toHaveBeenCalledWith("smart");
    const toggle = screen.getByRole("button", { name: /Smart Mailboxes/ });
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(document.querySelector('[data-key="smart:1"] [data-icon="folder-cog"]')).not.toBeNull();
  });

  it("reports a right click on a row", () => {
    const onContextMenu = vi.fn();
    render(
      <Sidebar
        sections={sections}
        selectedKey={null}
        focused={false}
        sync={{}}
        onSelect={vi.fn()}
        onContextMenu={onContextMenu}
      />,
    );
    const row = document.querySelector<HTMLElement>('[data-key="smart:1"]');
    if (!row) throw new Error("no row");
    fireEvent.contextMenu(row, { clientX: 30, clientY: 40 });
    expect(onContextMenu).toHaveBeenCalledWith("smart:1", 30, 40);
  });
});
