import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ContextMenu, type MenuItem } from "./ContextMenu";

const items: MenuItem[] = [
  {
    kind: "submenu",
    id: "copy",
    label: "Copy to",
    items: ["Drafts", "Sent", "Junk", "Trash", "Archive"].map(
      (label): MenuItem => ({ kind: "item", id: label, label }),
    ),
  },
];

// Lay out the window at 1000 × 600 with menus 180 × 200 and the submenu's
// row at the given place; happy-dom lays out nothing itself.
function layout(row: { top: number; right: number }, parentLeft: number) {
  vi.spyOn(window, "innerWidth", "get").mockReturnValue(1000);
  vi.spyOn(window, "innerHeight", "get").mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(180);
  vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(200);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const r = this.getAttribute("role") === "menu" ? { top: 0, left: parentLeft, right: parentLeft + 180 } : row;
    return { ...r, bottom: r.top + 24, width: 180, height: 24, x: 0, y: 0, toJSON: () => ({}) } as DOMRect;
  });
}

function openSubmenu(): HTMLElement {
  render(<ContextMenu items={items} x={0} y={0} onSelect={vi.fn()} onClose={vi.fn()} />);
  fireEvent.mouseEnter(screen.getByRole("menuitem", { name: "Copy to" }));
  const sub = screen.getAllByRole("menu")[1];
  if (!sub) throw new Error("no submenu");
  return sub;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ContextMenu submenus", () => {
  it("open beside their row", () => {
    layout({ top: 100, right: 300 }, 120);
    const sub = openSubmenu();
    expect(sub.style.left).toBe("300px");
    expect(sub.style.top).toBe("100px");
  });

  it("move up to stay in the window", () => {
    layout({ top: 500, right: 300 }, 120);
    expect(openSubmenu().style.top).toBe("396px");
  });

  it("open to the left when the window ends on the right", () => {
    layout({ top: 100, right: 900 }, 720);
    expect(openSubmenu().style.left).toBe("540px");
  });
});
