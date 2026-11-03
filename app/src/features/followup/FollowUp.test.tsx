import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Address, Awaiting, AwaitingThread, MessageSummary } from "../../rpc/gen/api";
import { FollowUpSection } from "./FollowUp";

const ann: Address = { name: "Ann Smith", address: "ann.smith@northwind.test" };
const bob: Address = { name: "Bob Okafor", address: "bob.okafor@acme.test" };
const carol: Address = { name: "Carol", address: "carol@vertex.test" };
const dmitri: Address = { name: "Dmitri", address: "dmitri@frostyard.test" };

function summary(id: number, subject: string, over: Partial<MessageSummary> = {}): MessageSummary {
  return {
    id,
    accountId: 1,
    mailboxIds: [3],
    threadId: id,
    subject,
    from: { name: "Test One", address: "test1@mailtest.test" },
    date: "2026-10-04T12:00:00Z",
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 2000,
    threadCount: 1,
    ...over,
  };
}

function awaitingThread(over: Partial<AwaitingThread> = {}): AwaitingThread {
  return {
    threadId: 1,
    accountId: 1,
    message: summary(1, "Offsite plan"),
    recipients: [ann],
    recipientCount: 1,
    vip: false,
    waitingDays: 1,
    ...over,
  };
}

function awaiting(threads: AwaitingThread[], total = threads.length): Awaiting {
  return { total, threads };
}

function show(threads: AwaitingThread[], total = threads.length) {
  const onOpen = vi.fn();
  const view = render(<FollowUpSection awaiting={awaiting(threads, total)} onOpen={onOpen} />);
  return { onOpen, ...view };
}

function row(text: string): HTMLElement {
  const el = screen.getByText(text).closest("li")?.querySelector("button");
  if (!el) throw new Error(`no row with ${text}`);
  return el;
}

describe("FollowUpSection", () => {
  it("renders nothing while the threads load, and when none wait", () => {
    const { container, rerender } = render(<FollowUpSection awaiting={undefined} onOpen={() => {}} />);
    expect(container.innerHTML).toBe("");
    rerender(<FollowUpSection awaiting={awaiting([])} onOpen={() => {}} />);
    expect(container.innerHTML).toBe("");
    expect(screen.queryByRole("region", { name: "Follow Up" })).toBeNull();
  });

  it("is a labeled section with a collapsible header", () => {
    show([awaitingThread()]);
    const section = screen.getByRole("region", { name: "Follow Up" });
    const header = within(section).getByRole("button", { name: "Follow Up" });
    expect(header.className).toContain("h-[26px]");
    expect(header.className).toContain("text-sidebar-section");
    expect(header.className).toContain("text-secondary");
    expect(header.getAttribute("title")).toBe("Sent mail with no reply for 3 days or more");
    expect(header.getAttribute("aria-expanded")).toBe("true");
    expect(within(section).getAllByRole("listitem")).toHaveLength(1);

    fireEvent.click(header);
    expect(header.getAttribute("aria-expanded")).toBe("false");
    expect(within(section).queryAllByRole("listitem")).toHaveLength(0);

    fireEvent.click(header);
    expect(within(section).getAllByRole("listitem")).toHaveLength(1);
  });

  it("shows each thread's subject and its recipients, counting the rest", () => {
    show([
      awaitingThread({ threadId: 1, message: summary(1, "Offsite plan"), recipients: [ann], recipientCount: 1 }),
      awaitingThread({
        threadId: 2,
        message: summary(2, "Signed contract"),
        recipients: [ann, bob, carol, dmitri],
        recipientCount: 6,
      }),
    ]);
    const first = row("Offsite plan");
    expect(within(first).getByText("To: Ann Smith").className).toContain("text-secondary");
    expect(within(row("Signed contract")).getByText("To: Ann Smith, Bob Okafor & 4 more")).toBeTruthy();
  });

  it("says how long each thread has waited, in days", () => {
    show([
      awaitingThread({ threadId: 1, message: summary(1, "One day"), waitingDays: 1 }),
      awaitingThread({ threadId: 2, message: summary(2, "Four days"), waitingDays: 4 }),
    ]);
    expect(within(row("One day")).getByText("Waiting 1 day")).toBeTruthy();
    expect(within(row("Four days")).getByText("Waiting 4 days")).toBeTruthy();
  });

  it("marks a thread with a VIP recipient", () => {
    show([
      awaitingThread({ threadId: 1, message: summary(1, "Plain") }),
      awaitingThread({ threadId: 2, message: summary(2, "Important"), vip: true }),
    ]);
    expect(within(row("Important")).getByLabelText("VIP")).toBeTruthy();
    expect(within(row("Plain")).queryByLabelText("VIP")).toBeNull();
  });

  it("states the count when more wait than it lists", () => {
    const many = Array.from({ length: 25 }, (_, index) =>
      awaitingThread({ threadId: index + 1, message: summary(index + 1, `Thread ${index + 1}`) }),
    );
    const { unmount } = show(many, 40);
    expect(screen.getByText("Showing 25 of 40")).toBeTruthy();
    unmount();

    show([awaitingThread()], 1);
    expect(screen.queryByText(/^Showing /)).toBeNull();
  });

  it("opens a thread when its row is clicked", () => {
    const thread = awaitingThread({ threadId: 7, message: summary(7, "Offsite plan"), waitingDays: 3 });
    const { onOpen } = show([thread]);
    fireEvent.click(row("Offsite plan"));
    expect(onOpen).toHaveBeenCalledWith(thread);
  });
});
