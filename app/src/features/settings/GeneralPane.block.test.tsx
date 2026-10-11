// CONTRACT TEST for task card T-0129 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { BlockedSender, Settings } from "../../rpc/gen/api";
import { GeneralPane } from "./GeneralPane";

const settings: Settings = {
  undoDelay: 10,
  notifyScope: "inbox",
  flagNames: ["", "", "", "", "", "", ""],
  blockAction: "mark",
};

const blocked: BlockedSender[] = [
  { address: "deals@shop.test", name: "", at: "2026-10-01T09:00:00Z" },
  { address: "spam@y.test", name: "Spam Co", at: "2026-10-02T09:00:00Z" },
];

describe("GeneralPane blocking", () => {
  it("sets what happens to blocked mail", () => {
    const onChange = vi.fn();
    render(<GeneralPane settings={settings} onChange={onChange} />);
    const action = screen.getByLabelText("Blocked mail:") as HTMLSelectElement;
    expect(action.value).toBe("mark");
    expect(Array.from(action.options).map((o) => [o.value, o.textContent])).toEqual([
      ["mark", "Mark it, and leave it where it arrives"],
      ["trash", "Move it to the Trash"],
    ]);
    fireEvent.change(action, { target: { value: "trash" } });
    expect(onChange).toHaveBeenLastCalledWith({ blockAction: "trash" });
  });

  it("reads mark when maild says nothing", () => {
    const { blockAction: _, ...older } = settings;
    render(<GeneralPane settings={older} onChange={vi.fn()} />);
    expect((screen.getByLabelText("Blocked mail:") as HTMLSelectElement).value).toBe("mark");
  });

  it("lists blocked senders with Unblock", () => {
    const onUnblock = vi.fn();
    render(<GeneralPane settings={settings} blocked={blocked} onChange={vi.fn()} onUnblock={onUnblock} />);
    const list = screen.getByRole("list", { name: "Blocked senders" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows.map((r) => r.textContent)).toEqual(["deals@shop.testUnblock", "Spam Cospam@y.testUnblock"]);
    fireEvent.click(within(list).getByRole("button", { name: "Unblock spam@y.test" }));
    expect(onUnblock).toHaveBeenCalledWith("spam@y.test");
  });

  it("says None when nobody is blocked", () => {
    render(<GeneralPane settings={settings} blocked={[]} onChange={vi.fn()} />);
    expect(screen.queryByRole("list", { name: "Blocked senders" })).toBeNull();
    expect(screen.getByText("None")).toBeTruthy();
  });
});
