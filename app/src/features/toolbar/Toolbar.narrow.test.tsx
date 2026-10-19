// A narrow window's toolbar and message bar (docs/specs/ui.md, Narrow
// windows), and the window controls only the desktop app draws.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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

describe("window controls", () => {
  it("are left out where the window is not the desktop app's", () => {
    toolbar({ windowControls: false });
    expect(screen.queryByRole("button", { name: "Minimize" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    expect(screen.getByRole("button", { name: "Settings" })).toBeTruthy();
  });
});
