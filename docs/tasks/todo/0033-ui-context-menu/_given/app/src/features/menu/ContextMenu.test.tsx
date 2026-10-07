// CONTRACT TEST for task card T-0033 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ContextMenu, type MenuItem } from "./ContextMenu";

const items: MenuItem[] = [
  { kind: "item", id: "read", label: "Mark as Read", shortcut: "Ctrl+Shift+U" },
  { kind: "item", id: "junk", label: "Mark as Junk", disabled: true },
  {
    kind: "submenu",
    id: "flag",
    label: "Flag",
    items: [
      { kind: "item", id: "flag:1", label: "Red", checked: true },
      { kind: "item", id: "flag:2", label: "Orange" },
      { kind: "separator" },
      { kind: "item", id: "flag:0", label: "Clear Flag" },
    ],
  },
  { kind: "separator" },
  { kind: "item", id: "delete", label: "Delete" },
];

function open(over: Partial<Parameters<typeof ContextMenu>[0]> = {}) {
  const onSelect = vi.fn();
  const onClose = vi.fn();
  render(
    <div>
      <p>outside</p>
      <ContextMenu items={items} x={40} y={50} onSelect={onSelect} onClose={onClose} {...over} />
    </div>,
  );
  return { onSelect, onClose };
}

const highlighted = () => document.querySelector('[role="menuitem"][data-highlighted="true"]')?.textContent ?? null;

describe("ContextMenu", () => {
  it("renders a positioned menu with items, separators and shortcuts", () => {
    open();
    const menu = screen.getByRole("menu");
    expect(menu.style.position).toBe("fixed");
    expect(menu.style.left).toBe("40px");
    expect(menu.style.top).toBe("50px");
    expect(screen.getAllByRole("menuitem").map((m) => m.textContent)).toEqual([
      "Mark as ReadCtrl+Shift+U",
      "Mark as Junk",
      "Flag",
      "Delete",
    ]);
    expect(screen.getAllByRole("separator")).toHaveLength(1);
    expect(screen.getByRole("menuitem", { name: /Mark as Junk/ }).getAttribute("aria-disabled")).toBe("true");
    const flag = screen.getByRole("menuitem", { name: "Flag" });
    expect(flag.getAttribute("aria-haspopup")).toBe("menu");
    expect(flag.getAttribute("aria-expanded")).toBe("false");
  });

  it("is rendered into document.body and takes focus", () => {
    open();
    const menu = screen.getByRole("menu");
    expect(menu.parentElement).toBe(document.body);
    expect(document.activeElement).toBe(menu);
  });

  it("chooses an item on click, then closes", () => {
    const { onSelect, onClose } = open();
    fireEvent.click(screen.getByRole("menuitem", { name: /Delete/ }));
    expect(onSelect).toHaveBeenCalledWith("delete");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("ignores clicks on disabled items", () => {
    const { onSelect, onClose } = open();
    fireEvent.click(screen.getByRole("menuitem", { name: /Mark as Junk/ }));
    expect(onSelect).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("moves the highlight with the arrow keys, skipping disabled items and wrapping", () => {
    open();
    const menu = screen.getByRole("menu");
    expect(highlighted()).toBeNull();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("Mark as ReadCtrl+Shift+U");
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("Flag");
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("Delete");
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("Mark as ReadCtrl+Shift+U");
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    expect(highlighted()).toBe("Delete");
  });

  it("chooses the highlighted item with Enter", () => {
    const { onSelect, onClose } = open();
    const menu = screen.getByRole("menu");
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    fireEvent.keyDown(menu, { key: "Enter" });
    expect(onSelect).toHaveBeenCalledWith("delete");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("opens a submenu with ArrowRight and closes it with ArrowLeft", () => {
    const { onSelect } = open();
    const menu = screen.getByRole("menu");
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.keyDown(menu, { key: "ArrowRight" });
    expect(screen.getAllByRole("menu")).toHaveLength(2);
    expect(screen.getByRole("menuitem", { name: "Flag" }).getAttribute("aria-expanded")).toBe("true");
    expect(highlighted()).toBe("Red");
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("Orange");
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("Clear Flag");
    fireEvent.keyDown(menu, { key: "ArrowLeft" });
    expect(screen.getAllByRole("menu")).toHaveLength(1);
    expect(highlighted()).toBe("Flag");
    fireEvent.keyDown(menu, { key: "Enter" });
    expect(screen.getAllByRole("menu")).toHaveLength(2);
    fireEvent.keyDown(menu, { key: "Enter" });
    expect(onSelect).toHaveBeenCalledWith("flag:1");
  });

  it("opens a submenu on hover and marks checked items", () => {
    const { onSelect } = open();
    fireEvent.mouseEnter(screen.getByRole("menuitem", { name: "Flag" }));
    const red = screen.getByRole("menuitem", { name: /Red/ });
    expect(red.getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("menuitem", { name: /Orange/ }).getAttribute("aria-checked")).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: /Orange/ }));
    expect(onSelect).toHaveBeenCalledWith("flag:2");
  });

  it("closes with Escape, the submenu first", () => {
    const { onClose } = open();
    const menu = screen.getByRole("menu");
    fireEvent.mouseEnter(screen.getByRole("menuitem", { name: "Flag" }));
    expect(screen.getAllByRole("menu")).toHaveLength(2);
    fireEvent.keyDown(menu, { key: "Escape" });
    expect(screen.getAllByRole("menu")).toHaveLength(1);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(menu, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes on a mouse press outside, not inside", () => {
    const { onClose } = open();
    fireEvent.mouseDown(screen.getByRole("menuitem", { name: /Delete/ }));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.mouseDown(screen.getByText("outside"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("stays inside the window", () => {
    open({ x: window.innerWidth - 2, y: window.innerHeight - 2 });
    const menu = screen.getByRole("menu");
    expect(Number.parseFloat(menu.style.left)).toBeLessThanOrEqual(window.innerWidth - 2);
    expect(Number.parseFloat(menu.style.left)).toBeGreaterThanOrEqual(0);
    expect(Number.parseFloat(menu.style.top)).toBeGreaterThanOrEqual(0);
  });
});
