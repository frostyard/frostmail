// A narrow window's toolbar and message bar (docs/specs/ui.md, Narrow
// windows), and the window controls only the desktop app draws. The message
// bar moves the actions that do not fit into a » menu, from their end.
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MessageBar, Toolbar, type ToolbarNarrow, type ToolbarProps } from "./Toolbar";

function toolbar(over: Partial<ToolbarProps> = {}) {
  const props: ToolbarProps = {
    sidebarWidth: 220,
    listWidth: 360,
    title: "Inbox",
    subtitle: "12 messages, 3 unread",
    syncing: false,
    selection: { count: 1, seen: true, flagColor: 0 },
    canArchive: true,
    moveTargets: [{ mailboxId: 7, label: "Projects", depth: 0 }],
    maximized: false,
    search: <input aria-label="Search" />,
    onCommand: vi.fn(),
    ...over,
  };
  render(<Toolbar {...props} />);
  return props;
}

const narrow = (pane: ToolbarNarrow["pane"]) => ({ pane, onBack: vi.fn() });

describe("a narrow window's toolbar", () => {
  it("over the list: Back to Mailboxes, the title, New Message, and the search on a row of its own", () => {
    const n = narrow("list");
    const props = toolbar({ narrow: n });
    const bar = screen.getByRole("toolbar", { name: "Toolbar" });
    expect(bar.querySelector("[data-segment]")).toBeNull();
    expect(within(bar).getByText("Inbox")).toBeTruthy();
    expect(within(bar).getByText("12 messages, 3 unread")).toBeTruthy();
    expect(within(bar).queryByRole("textbox", { name: "Search" })).toBeNull();
    expect(screen.getByRole("textbox", { name: "Search" })).toBeTruthy();
    fireEvent.click(within(bar).getByRole("button", { name: "Back to Mailboxes" }));
    expect(n.onBack).toHaveBeenCalledOnce();
    const compose = within(bar).getByRole("button", { name: "New Message" });
    expect(compose.className).toContain("size-11");
    fireEvent.click(compose);
    expect(props.onCommand).toHaveBeenCalledWith({ kind: "compose" });
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
  });

  it("over a message: Back names the list it returns to", () => {
    toolbar({ narrow: narrow("reader") });
    expect(screen.getByRole("button", { name: "Back to Inbox" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Search" })).toBeNull();
  });

  it("over Mailboxes: no Back, with Get Mail and Settings", () => {
    const props = toolbar({ narrow: narrow("mailboxes") });
    expect(screen.queryByRole("button", { name: /^Back to/ })).toBeNull();
    expect(screen.getByText("Mailboxes")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Get Mail" }));
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    expect(props.onCommand).toHaveBeenCalledWith({ kind: "getMail" });
    expect(props.onCommand).toHaveBeenCalledWith({ kind: "settings" });
  });

  it("over another module: its title, and no Back, since the module tabs move between modules", () => {
    toolbar({
      mode: "tasks",
      tasks: { title: "Today", showCompleted: null, paneWidth: 320 },
      narrow: narrow("module"),
    });
    expect(screen.getByText("Today")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^Back to/ })).toBeNull();
  });
});

describe("the message bar", () => {
  it("holds the message actions as touch targets", () => {
    const onCommand = vi.fn();
    render(
      <MessageBar
        selection={{ count: 1, seen: false, flagColor: 0 }}
        canArchive
        moveTargets={[{ mailboxId: 7, label: "Projects", depth: 0 }]}
        onCommand={onCommand}
      />,
    );
    const bar = screen.getByRole("toolbar", { name: "Message Actions" });
    const names = within(bar)
      .getAllByRole("button")
      .map((b) => b.getAttribute("aria-label"));
    expect(names).toEqual(["Delete", "Archive", "Reply", "Reply All", "Forward", "Flag", "Mark as Read", "Move"]);
    for (const b of within(bar).getAllByRole("button")) expect(b.className).toContain("h-11");
    fireEvent.click(within(bar).getByRole("button", { name: "Reply" }));
    expect(onCommand).toHaveBeenCalledWith({ kind: "reply" });
    fireEvent.click(within(bar).getByRole("button", { name: "Move" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Projects" }));
    expect(onCommand).toHaveBeenCalledWith({ kind: "move", mailboxId: 7 });
  });
});

/** layout gives the message bar and its buttons the widths a phone has. */
function layout(barWidth: number) {
  let width = barWidth;
  const observers: (() => void)[] = [];
  const disconnect = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        observers.push(callback);
      }
      observe() {}
      disconnect = disconnect;
    },
  );
  const computedStyle = window.getComputedStyle.bind(window);
  vi.spyOn(window, "getComputedStyle").mockImplementation((element) => {
    const style = computedStyle(element);
    if (element instanceof HTMLElement && element.getAttribute("aria-label") === "Message Actions") {
      const measured = document.createElement("div").style;
      measured.cssText = style.cssText;
      measured.paddingLeft = "4px";
      measured.paddingRight = "4px";
      measured.columnGap = "4px";
      return measured;
    }
    return style;
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    if (this.getAttribute("aria-label") === "Message Actions") return new DOMRect(0, 0, width, 44);
    return new DOMRect(0, 0, 0, 44);
  });
  return {
    disconnect,
    resize(nextWidth: number) {
      width = nextWidth;
      act(() => {
        for (const notify of observers) notify();
      });
    },
  };
}

/** barButtons lists the bar's visible buttons, by name, in order. */
function barButtons() {
  return within(screen.getByRole("toolbar", { name: "Message Actions" }))
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? "");
}

const barProps = {
  selection: { count: 1, seen: false, flagColor: 0 },
  canArchive: true,
  moveTargets: [{ mailboxId: 7, label: "Projects", depth: 0 }],
};

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("the message bar at narrow widths", () => {
  it("shows all 8 actions at 390px, each at least 44 wide, and no »", () => {
    layout(390);
    render(<MessageBar {...barProps} onCommand={vi.fn()} />);
    expect(barButtons()).toEqual([
      "Delete",
      "Archive",
      "Reply",
      "Reply All",
      "Forward",
      "Flag",
      "Mark as Read",
      "Move",
    ]);
    expect(screen.queryByRole("button", { name: "More Toolbar Items" })).toBeNull();
    for (const button of within(screen.getByRole("toolbar", { name: "Message Actions" })).getAllByRole("button")) {
      expect(button.className).toContain("min-w-11");
    }
  });

  it("moves Flag, Mark and Move into » at 320px, in toolbar order", () => {
    layout(320);
    const onCommand = vi.fn();
    render(<MessageBar {...barProps} onCommand={onCommand} />);
    expect(barButtons()).toEqual(["Delete", "Archive", "Reply", "Reply All", "Forward", "More Toolbar Items"]);

    fireEvent.click(screen.getByRole("button", { name: "More Toolbar Items" }));
    const menu = screen.getByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.getAttribute("aria-label")),
    ).toEqual(["Flag", "Mark as Read", "Move"]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Mark as Read" }));
    expect(onCommand).toHaveBeenLastCalledWith({ kind: "toggleRead" });

    fireEvent.click(screen.getByRole("button", { name: "More Toolbar Items" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Move" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Projects" }));
    expect(onCommand).toHaveBeenLastCalledWith({ kind: "move", mailboxId: 7 });
  });

  it("refits when the bar widens again", () => {
    const sizes = layout(320);
    render(<MessageBar {...barProps} onCommand={vi.fn()} />);
    expect(barButtons()).toEqual(["Delete", "Archive", "Reply", "Reply All", "Forward", "More Toolbar Items"]);
    sizes.resize(390);
    expect(barButtons()).toEqual([
      "Delete",
      "Archive",
      "Reply",
      "Reply All",
      "Forward",
      "Flag",
      "Mark as Read",
      "Move",
    ]);
    expect(screen.queryByRole("button", { name: "More Toolbar Items" })).toBeNull();
  });
});

describe("window controls", () => {
  it("are left out where the window is not the desktop app's", () => {
    toolbar({ windowControls: false });
    expect(screen.queryByRole("button", { name: "Minimize" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    expect(screen.getByRole("button", { name: "Settings" })).toBeTruthy();
  });
});
