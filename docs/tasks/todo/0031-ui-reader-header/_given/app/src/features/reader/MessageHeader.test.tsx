// CONTRACT TEST for task card T-0031 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { avatarTone, formatHeaderDate, formatSize } from "../../lib/format";
import type { Message, Part } from "../../rpc/gen/api";
import { AttachmentStrip, MessageHeader, RemoteBanner } from "./MessageHeader";

const date = new Date(2026, 9, 7, 9, 41);

function message(over: Partial<Message> = {}): Message {
  return {
    summary: {
      id: 1,
      accountId: 1,
      mailboxIds: [1],
      threadId: 1,
      subject: "Offsite plan",
      from: { name: "Ann Smith", address: "ann@northwind.test" },
      date: date.toISOString(),
      preview: "",
      flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
      hasAttachments: false,
      size: 100,
      threadCount: 1,
    },
    to: [
      { name: "Test One", address: "test1@mailtest.test" },
      { name: "", address: "b@x.test" },
    ],
    cc: [],
    replyTo: [],
    messageId: "a@x",
    inReplyTo: "",
    references: [],
    listId: "",
    listUnsubscribe: "",
    parts: [],
    bodyFetched: true,
    ...over,
  };
}

const part = (path: string, over: Partial<Part> = {}): Part => ({
  path,
  contentType: "application/pdf",
  filename: "",
  disposition: "",
  contentId: "",
  size: 1000,
  ...over,
});

describe("MessageHeader", () => {
  it("shows the avatar, sender, date, subject and recipients", () => {
    render(<MessageHeader message={message()} />);
    const avatar = screen.getByText("AS");
    expect(avatar.getAttribute("aria-hidden")).toBe("true");
    expect(avatar.className).toContain(`bg-avatar-${avatarTone("ann@northwind.test")}`);
    expect(avatar.className).toContain("size-10");
    const sender = screen.getByText("Ann Smith");
    expect(sender.className).toContain("text-reader-sender");
    expect(sender.getAttribute("title")).toBe("ann@northwind.test");
    expect(screen.getByText(formatHeaderDate(date)).className).toContain("text-secondary");
    expect(screen.getByText("Offsite plan").className).toContain("text-reader-subject");
    expect(screen.getByText("To: Test One, b@x.test")).toBeTruthy();
    expect(screen.queryByText(/^Cc:/)).toBeNull();
  });

  it("shows Cc when present and summarizes long lists", () => {
    const cc = ["a", "b", "c", "d", "e"].map((n) => ({ name: n.toUpperCase(), address: `${n}@x.test` }));
    render(<MessageHeader message={message({ cc })} />);
    expect(screen.getByText("Cc: A, B, C & 2 more")).toBeTruthy();
  });

  it("shows (No Subject) for an empty subject", () => {
    const m = message();
    render(<MessageHeader message={{ ...m, summary: { ...m.summary, subject: "" } }} />);
    expect(screen.getByText("(No Subject)")).toBeTruthy();
  });
});

describe("AttachmentStrip", () => {
  it("renders nothing without attachments", () => {
    const { container } = render(
      <AttachmentStrip
        parts={[part("1", { contentType: "text/plain" }), part("2", { disposition: "inline", filename: "logo.png" })]}
        onOpen={() => {}}
      />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("lists attachments as chips that open on click", () => {
    const onOpen = vi.fn();
    const pdf = part("2", { filename: "contract-signed.pdf", disposition: "attachment", size: 482_113 });
    const csv = part("3", { filename: "invoice.csv", size: 2_310 });
    const unnamed = part("4", { disposition: "attachment" });
    render(<AttachmentStrip parts={[part("1"), pdf, csv, unnamed]} onOpen={onOpen} />);
    const list = screen.getByRole("list", { name: "Attachments" });
    const chips = within(list).getAllByRole("button");
    expect(chips.map((c) => c.textContent)).toEqual([
      `contract-signed.pdf${formatSize(482_113)}`,
      `invoice.csv${formatSize(2_310)}`,
      `Untitled${formatSize(1000)}`,
    ]);
    expect(chips[0]?.getAttribute("title")).toBe("contract-signed.pdf");
    expect(chips[0]?.className).toContain("bg-banner");
    fireEvent.click(chips[1] as HTMLElement);
    expect(onOpen).toHaveBeenCalledWith(csv);
  });
});

describe("RemoteBanner", () => {
  it("renders nothing when nothing was left out", () => {
    const { container } = render(<RemoteBanner remote={0} trackers={0} loading={false} onLoad={() => {}} />);
    expect(container.innerHTML).toBe("");
  });

  it("offers to load remote content and counts trackers", () => {
    const onLoad = vi.fn();
    render(<RemoteBanner remote={3} trackers={2} loading={false} onLoad={onLoad} />);
    const status = screen.getByRole("status");
    expect(status.className).toContain("bg-banner");
    expect(status.textContent).toContain("This message contains remote content. 2 trackers blocked.");
    fireEvent.click(screen.getByRole("button", { name: "Load Remote Content" }));
    expect(onLoad).toHaveBeenCalledTimes(1);
  });

  it("says when only trackers were removed, in the singular", () => {
    render(<RemoteBanner remote={0} trackers={1} loading={false} onLoad={() => {}} />);
    expect(screen.getByRole("status").textContent).toBe("1 tracker blocked.");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("disables the button while loading", () => {
    render(<RemoteBanner remote={1} trackers={0} loading={true} onLoad={() => {}} />);
    const button = screen.getByRole("button", { name: "Loading…" });
    expect(button.hasAttribute("disabled")).toBe(true);
  });
});
