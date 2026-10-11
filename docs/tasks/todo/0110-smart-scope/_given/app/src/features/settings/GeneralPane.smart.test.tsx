// CONTRACT TEST for task card T-0110 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Settings, SmartMailbox } from "../../rpc/gen/api";
import { GeneralPane } from "./GeneralPane";

const smart = (id: number, name: string): SmartMailbox => ({
  id,
  name,
  position: 0,
  conditions: { match: "all", conditions: [] },
  includeTrash: false,
  includeSent: false,
  unread: 0,
});

const settings: Settings = { undoDelay: 10, notifyScope: "inbox", flagNames: ["", "", "", "", "", "", ""] };

describe("the General pane's smart mailbox scopes", () => {
  it("lists each smart mailbox after the other scopes", () => {
    render(<GeneralPane settings={settings} smarts={[smart(4, "Urgent"), smart(9, "Family")]} onChange={vi.fn()} />);
    const scope = screen.getByLabelText("New message notifications:") as HTMLSelectElement;
    expect(Array.from(scope.options).map((o) => [o.value, o.textContent])).toEqual([
      ["inbox", "Inbox Only"],
      ["vips", "VIPs"],
      ["contacts", "Contacts"],
      ["all", "All Mailboxes"],
      ["smart:4", "Urgent"],
      ["smart:9", "Family"],
    ]);
    expect(scope.querySelector("optgroup")?.label).toBe("Smart Mailboxes");
  });

  it("sets and shows a smart mailbox scope", () => {
    const onChange = vi.fn();
    const { unmount } = render(<GeneralPane settings={settings} smarts={[smart(4, "Urgent")]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("New message notifications:"), { target: { value: "smart:4" } });
    expect(onChange).toHaveBeenLastCalledWith({ notifyScope: "smart", notifySmartId: 4 });
    unmount();
    render(
      <GeneralPane
        settings={{ ...settings, notifyScope: "smart", notifySmartId: 4 }}
        smarts={[smart(4, "Urgent")]}
        onChange={vi.fn()}
      />,
    );
    expect((screen.getByLabelText("New message notifications:") as HTMLSelectElement).value).toBe("smart:4");
  });

  it("has no group without smart mailboxes", () => {
    render(<GeneralPane settings={settings} onChange={vi.fn()} />);
    expect(screen.getByLabelText("New message notifications:").querySelector("optgroup")).toBeNull();
  });
});
