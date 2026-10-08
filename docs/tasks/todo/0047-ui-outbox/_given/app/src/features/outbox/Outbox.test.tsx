// CONTRACT TEST for task card T-0047 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OutboxItem } from "../../rpc/gen/api";
import { OutboxStatus, UndoToast } from "./Outbox";

describe("UndoToast", () => {
  it("is a status line naming the message with Undo and the seconds left", () => {
    const onUndo = vi.fn();
    render(<UndoToast subject="Lunch" secondsLeft={7} onUndo={onUndo} />);
    const toast = screen.getByRole("status");
    expect(toast.className).toContain("h-9");
    expect(toast.className).toContain("bg-toolbar");
    expect(within(toast).getByText("Sending “Lunch”…").className).toContain("truncate");
    expect(within(toast).getByText("7s").className).toContain("tabular-nums");
    const undo = within(toast).getByRole("button", { name: "Undo" });
    expect(undo.className).toContain("text-accent");
    fireEvent.click(undo);
    expect(onUndo).toHaveBeenCalledTimes(1);
  });

  it("names a message without a subject", () => {
    render(<UndoToast subject="  " secondsLeft={3} onUndo={() => {}} />);
    expect(screen.getByText("Sending “(no subject)”…")).toBeTruthy();
  });

  it("drops Undo and the countdown when the delay has run out", () => {
    render(<UndoToast subject="Lunch" secondsLeft={0} onUndo={() => {}} />);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    expect(screen.queryByText("0s")).toBeNull();
    expect(screen.getByText("Sending “Lunch”…")).toBeTruthy();
  });
});

const now = new Date("2026-10-07T12:00:00Z");

function item(over: Partial<OutboxItem>): OutboxItem {
  return {
    id: 1,
    accountId: 1,
    draftId: 10,
    subject: "Lunch",
    to: [{ name: "Ann", address: "ann@x.test" }],
    state: "queued",
    sendAt: "2026-10-07T12:00:10Z",
    attempts: 0,
    ...over,
  };
}

function outbox(items: OutboxItem[]) {
  const onRetry = vi.fn();
  const onEdit = vi.fn();
  const view = render(<OutboxStatus items={items} now={now} onRetry={onRetry} onEdit={onEdit} />);
  return { onRetry, onEdit, ...view };
}

function row(subject: string): HTMLElement {
  const el = screen.getByText(subject).closest("li");
  if (!el) throw new Error(`no row for ${subject}`);
  return el;
}

describe("OutboxStatus", () => {
  it("renders nothing when every message is sent", () => {
    const { container } = outbox([item({ state: "sent" })]);
    expect(container.innerHTML).toBe("");
  });

  it("is a labeled section with a heading and one row per unsent message", () => {
    outbox([
      item({ id: 1, subject: "One" }),
      item({ id: 2, subject: "Two", state: "sent" }),
      item({ id: 3, subject: "" }),
    ]);
    const section = screen.getByRole("region", { name: "Outbox" });
    expect(within(section).getByRole("heading", { name: "Outbox" }).className).toContain("text-sidebar-section");
    const rows = within(section).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(within(rows[1] as HTMLElement).getByText("(no subject)")).toBeTruthy();
  });

  it("lists at most two recipients", () => {
    outbox([
      item({
        to: [
          { name: "Ann", address: "ann@x.test" },
          { name: "", address: "bob@x.test" },
          { name: "Carol", address: "carol@x.test" },
          { name: "Dan", address: "dan@x.test" },
        ],
      }),
    ]);
    expect(within(row("Lunch")).getByText("To: Ann, bob@x.test & 2 more").className).toContain("text-secondary");
  });

  it("describes each state", () => {
    outbox([
      item({ id: 1, subject: "Waiting" }),
      item({ id: 2, subject: "Later", attempts: 2, sendAt: "2026-10-07T12:04:30Z", error: "connection refused" }),
      item({ id: 3, subject: "Soon", attempts: 1, sendAt: "2026-10-07T12:00:05Z", error: "" }),
      item({ id: 4, subject: "Going", state: "sending" }),
      item({ id: 5, subject: "Saving", state: "accepted", draftId: undefined }),
      item({ id: 6, subject: "Broken", state: "failed", attempts: 1, error: "550 no such user" }),
    ]);
    expect(within(row("Waiting")).getByText("Waiting to send")).toBeTruthy();
    expect(within(row("Later")).getByText("Retrying in 5 min: connection refused")).toBeTruthy();
    expect(within(row("Soon")).getByText("Retrying in 1 min")).toBeTruthy();
    expect(within(row("Going")).getByText("Sending…")).toBeTruthy();
    expect(within(row("Saving")).getByText("Saving to Sent…")).toBeTruthy();
    const failed = within(row("Broken")).getByText("Not sent: 550 no such user");
    expect(failed.className).toContain("text-flag-1");
  });

  it("offers Edit for queued messages and Retry and Edit for failed ones", () => {
    const queued = item({ id: 1, subject: "Waiting" });
    const failed = item({ id: 6, subject: "Broken", state: "failed", attempts: 1, error: "550" });
    const { onRetry, onEdit } = outbox([queued, failed, item({ id: 4, subject: "Going", state: "sending" })]);

    expect(within(row("Going")).queryAllByRole("button")).toHaveLength(0);
    expect(
      within(row("Waiting"))
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Edit"]);
    expect(
      within(row("Broken"))
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Retry", "Edit"]);

    fireEvent.click(within(row("Broken")).getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledWith(6);
    fireEvent.click(within(row("Waiting")).getByRole("button", { name: "Edit" }));
    expect(onEdit).toHaveBeenCalledWith(queued);
    expect(within(row("Broken")).getByRole("button", { name: "Edit" }).className).toContain("text-accent");
  });
});
