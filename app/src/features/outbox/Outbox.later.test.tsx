// CONTRACT TEST for task card T-0117 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OutboxItem } from "../../rpc/gen/api";
import { OutboxStatus, SendLaterStatus, type SendLaterStatusProps } from "./Outbox";

const now = new Date("2026-10-11T14:00:00Z"); // 10:00 AM in New York

function item(over: Partial<OutboxItem>): OutboxItem {
  return {
    id: 1,
    accountId: 1,
    draftId: 10,
    subject: "Lunch",
    to: [{ name: "Ann", address: "ann@x.test" }],
    state: "queued",
    sendAt: "2026-10-12T12:00:00Z",
    scheduled: true,
    attempts: 0,
    ...over,
  };
}

const ITEMS = [
  item({ id: 1, subject: "Monday", sendAt: "2026-10-12T12:00:00Z" }),
  item({ id: 2, subject: "Tonight", sendAt: "2026-10-12T01:00:00Z" }),
  item({ id: 3, subject: "Undo window", scheduled: false, sendAt: "2026-10-11T14:00:08Z" }),
  item({ id: 4, subject: "Broken", scheduled: true, state: "failed", error: "550 no" }),
];

function later(over: Partial<SendLaterStatusProps> = {}) {
  const props: SendLaterStatusProps = {
    items: ITEMS,
    now,
    timeZone: "America/New_York",
    locale: "en-US",
    onEdit: vi.fn(),
    onSendNow: vi.fn(),
    onChangeTime: vi.fn(),
    ...over,
  };
  const view = render(<SendLaterStatus {...props} />);
  return { props, ...view };
}

describe("SendLaterStatus", () => {
  it("lists the messages waiting for their time, soonest first", () => {
    later();
    const section = screen.getByRole("region", { name: "Send Later" });
    expect(within(section).getByRole("heading", { level: 2 }).textContent).toBe("Send Later");
    const rows = within(section).getAllByRole("listitem");
    expect(rows.map((r) => r.querySelector(".font-semibold")?.textContent)).toEqual(["Tonight", "Monday"]);
    expect(rows[0]?.textContent).toContain("To: Ann");
    expect(rows[0]?.textContent).toContain("Sends Today at 9:00 PM");
    expect(rows[1]?.textContent).toContain("Sends Tomorrow at 8:00 AM");
  });

  it("edits, sends now and changes the time of a message", () => {
    const { props } = later();
    const row = screen.getAllByRole("listitem")[1] as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Edit" }));
    expect(props.onEdit).toHaveBeenCalledWith(ITEMS[0]);
    fireEvent.click(within(row).getByRole("button", { name: "Send Now" }));
    expect(props.onSendNow).toHaveBeenCalledWith(ITEMS[0]);
    fireEvent.click(within(row).getByRole("button", { name: "Change Time…" }));
    expect(props.onChangeTime).toHaveBeenCalledWith(ITEMS[0]);
  });

  it("is not there without a message waiting", () => {
    const { container } = later({ items: [ITEMS[2] as OutboxItem, ITEMS[3] as OutboxItem] });
    expect(container.innerHTML).toBe("");
  });
});

describe("OutboxStatus and Send Later", () => {
  it("leaves the waiting messages to Send Later", () => {
    render(<OutboxStatus items={ITEMS} now={now} onRetry={vi.fn()} onEdit={vi.fn()} />);
    const section = screen.getByRole("region", { name: "Outbox" });
    expect(
      within(section)
        .getAllByRole("listitem")
        .map((r) => r.querySelector(".font-semibold")?.textContent),
    ).toEqual(["Undo window", "Broken"]);
  });
});
