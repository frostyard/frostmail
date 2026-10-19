// Notifications in a browser (docs/specs/lodge-app.md, The web build).
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useMail, useUI } from "../data/stores";
import { useFrameWindow } from "../lodge/frameWindow";
import { type Lodge, setLodge } from "../lodge/lodge";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { notify, watchAnnouncements } from "./announce";

class FakeNotification {
  static permission: NotificationPermission = "granted";
  static shown: FakeNotification[] = [];
  onclick: (() => void) | null = null;
  closed = false;
  constructor(
    readonly title: string,
    readonly options: NotificationOptions,
  ) {
    FakeNotification.shown.push(this);
  }
  close() {
    this.closed = true;
  }
}

const open = vi.fn();

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useFrameWindow.setState({ open: null });
  FakeNotification.permission = "granted";
  FakeNotification.shown = [];
  open.mockReset();
  vi.stubGlobal("Notification", FakeNotification);
  setLodge({ open } as unknown as Lodge);
});

afterEach(() => {
  setLodge(null);
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function setup() {
  const data = mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") });
  useMail.setState({ mailboxes: data.mailboxes.map((mb) => ({ ...mb, total: 0, unread: 0 })) });
  const mock = new MockTransport(data);
  const stop = watchAnnouncements(new Client(mock));
  return { data, mock, stop };
}

describe("watchAnnouncements", () => {
  it("shows each announced note, and its click opens the message in mail", async () => {
    const { data, mock, stop } = setup();
    const [first] = data.messages;
    if (!first) throw new Error("no messages");
    const id = first.summary.id;
    mock.emit({
      event: "message.announced",
      data: { accountId: first.summary.accountId, notes: [{ title: "Ann", body: "Plan\nSee you", messageId: id }] },
    });
    await vi.waitFor(() => expect(FakeNotification.shown).toHaveLength(1));
    const [n] = FakeNotification.shown;
    expect(n?.title).toBe("Ann");
    expect(n?.options).toEqual({ body: "Plan\nSee you", tag: `mail:${first.summary.accountId}:${id}` });
    n?.onclick?.();
    await vi.waitFor(() => expect(open).toHaveBeenCalledOnce());
    expect(useUI.getState().selected).toEqual([id]);
    expect(open.mock.calls[0]?.[0]).toMatch(new RegExp(`^/mail/mailbox/[0-9]+/message/${id}$`));
    expect(n?.closed).toBe(true);
    stop();
  });

  it("opens the account's inbox from a group note", async () => {
    const { data, mock, stop } = setup();
    const inbox = data.mailboxes.find((mb) => mb.role === "inbox");
    if (!inbox) throw new Error("no inbox");
    mock.emit({
      event: "message.announced",
      data: { accountId: inbox.accountId, notes: [{ title: "4 new messages", body: "From Ann, Bob and 2 others" }] },
    });
    await vi.waitFor(() => expect(FakeNotification.shown).toHaveLength(1));
    FakeNotification.shown[0]?.onclick?.();
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: inbox.id });
    expect(open).toHaveBeenCalledWith(`/mail/mailbox/${inbox.id}`);
    stop();
  });

  it("shows nothing until the browser allows it", async () => {
    FakeNotification.permission = "default";
    const { mock, stop } = setup();
    mock.emit({ event: "message.announced", data: { accountId: 1, notes: [{ title: "Ann", body: "Plan" }] } });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(FakeNotification.shown).toHaveLength(0);
    stop();
  });
});

describe("notify", () => {
  it("falls back to a service worker where the browser has no Notification constructor", async () => {
    const showNotification = vi.fn();
    vi.stubGlobal(
      "Notification",
      Object.assign(
        () => {
          throw new TypeError("Illegal constructor");
        },
        { permission: "granted" },
      ),
    );
    vi.stubGlobal("navigator", { serviceWorker: { getRegistration: async () => ({ showNotification }) } });
    await notify("Ann", "Plan", "mail:1:2", () => {});
    expect(showNotification).toHaveBeenCalledWith("Ann", { body: "Plan", tag: "mail:1:2" });
  });
});
