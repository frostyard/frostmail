// The settings window reads and saves an account's mailbox behaviors
// (docs/specs/settings-ui.md, Behavior (container): Edit; ADR-0051): the
// stored setting, the mailboxes it erases, and the save that sends them.
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Session } from "../data/session";
import type { AccountUpdateParams, MailboxListParams } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

/** setup serves the fixture account with its Trash kept a week and named Old Mail. */
function setup() {
  const data = mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") });
  const account = data.accounts[0];
  if (!account) throw new Error("no account");
  account.eraseTrashDays = 7;
  const trash = data.mailboxes.find((mb) => mb.id === FIXTURE.trash);
  if (!trash) throw new Error("no trash mailbox");
  trash.path = "Old Mail";
  trash.name = "Old Mail";
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  return mock;
}

function trash(): HTMLSelectElement {
  return screen.getByLabelText("Erase deleted messages:") as HTMLSelectElement;
}

function chosen(select: HTMLSelectElement): string {
  return select.options[select.selectedIndex]?.text ?? "";
}

describe("an existing account's mailbox behaviors", () => {
  it("starts at the stored behavior and names the mailboxes it erases", async () => {
    const user = userEvent.setup();
    const mock = setup();
    await screen.findByRole("navigation", { name: "Accounts" });
    const select = (await screen.findByLabelText("Erase deleted messages:")) as HTMLSelectElement;
    const list = (await screen.findByLabelText("Erase junk messages:")) as HTMLSelectElement;
    expect(chosen(select)).toBe("After One Week");
    expect(chosen(list)).toBe("Never");

    // The names are the account's trash and junk mailboxes, from mailbox.list.
    await screen.findByText(
      "Messages are erased from the server after they have been in “Old Mail” or “Junk” that long, and cannot be recovered. Mail older than this account's sync window is left on the server.",
    );
    expect(
      mock.calls.some(
        (call) => call.method === "mailbox.list" && (call.params as MailboxListParams).accountId === FIXTURE.accountId,
      ),
    ).toBe(true);

    await user.selectOptions(list, "After One Month");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const updated = () =>
      mock.calls.find(
        (call) => call.method === "account.update" && (call.params as AccountUpdateParams).eraseJunkDays === 30,
      );
    await waitFor(() => expect(updated()).toBeDefined());
    const saved = updated()?.params as AccountUpdateParams | undefined;
    expect(saved?.eraseTrashDays).toBe(7);
    await waitFor(() => expect(chosen(trash())).toBe("After One Week"));
    await waitFor(() => expect(chosen(list)).toBe("After One Month"));
  });
});
