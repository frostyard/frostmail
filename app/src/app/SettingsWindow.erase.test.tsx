// The settings window reads and saves an account's mailbox behaviors
// (docs/specs/settings-ui.md, Behavior (container): Edit; ADR-0051): the
// stored setting, the mailboxes it erases, and the save that sends them.
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Session } from "../data/session";
import type { AccountUpdateParams, Mailbox, MailboxListParams } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import type { Transport } from "../rpc/transport";
import { SettingsWindow } from "./SettingsWindow";

/** DeferredLists answers mailbox.list for one account only when the test says so, in the order the test chose. */
class DeferredLists implements Transport {
  /** The requests for one account's mailboxes, oldest first. */
  readonly asked: { resolve: (list: Mailbox[]) => void; reject: (err: unknown) => void }[] = [];
  constructor(private readonly inner: Transport) {}
  call<T>(method: string, params: unknown): Promise<T> {
    // The mail store asks for every mailbox: only the form's own read, which
    // names an account, waits for the test.
    if (method !== "mailbox.list" || (params as MailboxListParams).accountId === undefined)
      return this.inner.call<T>(method, params);
    return new Promise<T>((resolve, reject) => {
      this.asked.push({ resolve: (list) => resolve(list as T), reject });
    });
  }
  onEvent(handler: Parameters<Transport["onEvent"]>[0]): () => void {
    return this.inner.onEvent(handler);
  }
  onClose(handler: (reason: string) => void): () => void {
    return this.inner.onClose(handler);
  }
}

/** fixture is the fixture account, its Trash kept `trashDays` days and named Old Mail. */
function fixture(trashDays: number) {
  const data = mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") });
  const account = data.accounts[0];
  if (!account) throw new Error("no account");
  account.eraseTrashDays = trashDays;
  const trash = data.mailboxes.find((mb) => mb.id === FIXTURE.trash);
  if (!trash) throw new Error("no trash mailbox");
  trash.path = "Old Mail";
  trash.name = "Old Mail";
  return data;
}

function show(transport: Transport): void {
  render(
    <Session connect={() => Promise.resolve(transport)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
}

/** setup serves the fixture through the mock transport itself. */
function setup(trashDays = 7) {
  const data = fixture(trashDays);
  const mock = new MockTransport(data);
  show(mock);
  return { data, mock };
}

/** behaviorList is a mailbox.list reply that keeps only the roles the names cover. */
async function behaviorList(mock: MockTransport, names: { trash?: string; junk?: string }): Promise<Mailbox[]> {
  const all = await mock.call<Mailbox[]>("mailbox.list", {});
  return all.map((mb) => {
    if (mb.role === "trash") return names.trash === undefined ? { ...mb, role: "none" } : { ...mb, name: names.trash };
    if (mb.role === "junk") return names.junk === undefined ? { ...mb, role: "none" } : { ...mb, name: names.junk };
    return mb;
  });
}

function junk(): HTMLSelectElement {
  return screen.getByLabelText("Erase junk messages:") as HTMLSelectElement;
}

function chosen(select: HTMLSelectElement): string {
  return select.options[select.selectedIndex]?.text ?? "";
}

function changes(mock: MockTransport): AccountUpdateParams[] {
  return mock.calls
    .filter((call) => call.method === "account.update")
    .map((call) => call.params as AccountUpdateParams);
}

const NOTE = (names: string) =>
  `Messages are erased from the server after they have been in ${names} that long, and cannot be recovered. Mail older than this account's sync window is left on the server.`;

describe("an existing account's mailbox behaviors", () => {
  it("starts at the stored behavior and names the mailboxes it erases", async () => {
    const user = userEvent.setup();
    const { mock } = setup();
    await screen.findByRole("navigation", { name: "Accounts" });
    const select = (await screen.findByLabelText("Erase deleted messages:")) as HTMLSelectElement;
    const list = await screen.findByLabelText("Erase junk messages:");
    expect(chosen(select)).toBe("After One Week");
    expect(chosen(list as HTMLSelectElement)).toBe("Never");

    // The names are the account's trash and junk mailboxes, from mailbox.list.
    await screen.findByText(NOTE("“Old Mail” or “Junk”"));
    expect(
      mock.calls.some(
        (call) => call.method === "mailbox.list" && (call.params as MailboxListParams).accountId === FIXTURE.accountId,
      ),
    ).toBe(true);

    await user.selectOptions(list, "After One Month");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const updated = () => changes(mock).find((params) => params.eraseJunkDays === 30);
    await waitFor(() => expect(updated()).toBeDefined());
    expect(updated()?.eraseTrashDays).toBe(7);
    await waitFor(() => expect(chosen(select)).toBe("After One Week"));
    await waitFor(() => expect(chosen(list as HTMLSelectElement)).toBe("After One Month"));
  });

  it("saves the chosen behavior of a mailbox the account has", async () => {
    const user = userEvent.setup();
    const { data, mock } = setup(0);
    await screen.findByRole("navigation", { name: "Accounts" });
    const select = (await screen.findByLabelText("Erase deleted messages:")) as HTMLSelectElement;
    await screen.findByText(NOTE("“Old Mail” or “Junk”"));
    await user.selectOptions(select, "After One Day");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(changes(mock).some((params) => params.eraseTrashDays === 1)).toBe(true));
    expect(data.accounts[0]?.eraseTrashDays).toBe(1);
  });

  it("leaves out a behavior whose mailbox the account has not got", async () => {
    const user = userEvent.setup();
    const { data, mock } = setup(0);
    await screen.findByRole("navigation", { name: "Accounts" });
    const select = (await screen.findByLabelText("Erase deleted messages:")) as HTMLSelectElement;
    await screen.findByText(NOTE("“Old Mail” or “Junk”"));

    // The user chooses a week; then Trash loses its role elsewhere, as a
    // mailbox.changed from maild says, and the control says Never and cannot
    // be used.
    await user.selectOptions(select, "After One Week");
    await act(async () => {
      const mailbox = data.mailboxes.find((mb) => mb.id === FIXTURE.trash);
      if (!mailbox) throw new Error("no trash mailbox");
      mailbox.role = "none";
      mock.emit({
        event: "mailbox.changed",
        data: { id: FIXTURE.trash, accountId: FIXTURE.accountId, deleted: false },
      });
    });
    await waitFor(() => expect(select.disabled).toBe(true));
    expect(chosen(select)).toBe("Never");

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(changes(mock).length).toBeGreaterThan(0));
    // Save changes nothing the user could not choose: maild keeps its value.
    expect(changes(mock).every((params) => !("eraseTrashDays" in params))).toBe(true);
    expect(data.accounts[0]?.eraseTrashDays).toBe(0);
  });

  it("keeps the newest read of the mailboxes when an older one arrives late", async () => {
    const data = fixture(0);
    const mock = new MockTransport(data);
    const transport = new DeferredLists(mock);
    show(transport);
    await screen.findByRole("navigation", { name: "Accounts" });
    const select = (await screen.findByLabelText("Erase deleted messages:")) as HTMLSelectElement;
    const list = junk();

    // The first read names both mailboxes.
    await waitFor(() => expect(transport.asked.length).toBe(1));
    await act(async () => transport.asked[0]?.resolve(await behaviorList(mock, { trash: "Old Mail", junk: "Junk" })));
    await screen.findByText(NOTE("“Old Mail” or “Junk”"));
    expect(list.disabled).toBe(false);

    // Two more reads start; the second (older) one answers last, with other names.
    for (let i = 0; i < 2; i++) {
      await act(async () => {
        mock.emit({
          event: "mailbox.changed",
          data: { id: FIXTURE.trash, accountId: FIXTURE.accountId, deleted: false },
        });
      });
    }
    await waitFor(() => expect(transport.asked.length).toBe(3));
    await act(async () => transport.asked[2]?.resolve(await behaviorList(mock, { trash: "Newest" })));
    await screen.findByText(NOTE("“Newest”"));
    expect(list.disabled).toBe(true);

    await act(async () =>
      transport.asked[1]?.resolve(await behaviorList(mock, { trash: "Stale", junk: "Stale Junk" })),
    );
    expect(screen.queryByText(/Stale/)).toBeNull();
    expect(screen.getByText(NOTE("“Newest”"))).toBeTruthy();
    expect(list.disabled).toBe(true);

    // An older read that fails must not clear the names of a newer one.
    for (let i = 0; i < 2; i++) {
      await act(async () => {
        mock.emit({
          event: "mailbox.changed",
          data: { id: FIXTURE.trash, accountId: FIXTURE.accountId, deleted: false },
        });
      });
    }
    await waitFor(() => expect(transport.asked.length).toBe(5));
    await act(async () => transport.asked[4]?.resolve(await behaviorList(mock, { trash: "Newer" })));
    await screen.findByText(NOTE("“Newer”"));
    await act(async () => transport.asked[3]?.reject(new Error("mailbox.list failed")));
    expect(screen.getByText(NOTE("“Newer”"))).toBeTruthy();
    expect(chosen(select)).toBe("Never");
    expect(select.disabled).toBe(false);
  });
});
