// CONTRACT TEST for task card T-0112 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useMail } from "../data/stores";
import type { Rule } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

beforeEach(() => {
  useMail.setState({ vips: [], settings: null, smarts: [], rules: [] });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const anyone = { match: "any", conditions: [{ field: "from", op: "contains", value: "" }] };

async function setup(seed: (mock: MockTransport) => Promise<void> = async () => {}) {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
  await seed(mock);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("navigation", { name: "Accounts" });
  fireEvent.click(screen.getByRole("button", { name: "Rules" }));
  return mock;
}

const calls = (mock: MockTransport, method: string) =>
  mock.calls.filter((c) => c.method === method).map((c) => c.params);
const names = () =>
  within(screen.getByRole("list", { name: "Rules" }))
    .queryAllByRole("button")
    .map((b) => b.textContent);

describe("the settings window's Rules pane", () => {
  it("makes a rule in the rule sheet", async () => {
    const mock = await setup();
    await screen.findByText("No Rules");
    fireEvent.click(screen.getByRole("button", { name: "Add Rule" }));
    const dialog = screen.getByRole("dialog", { name: "New Rule" });
    expect((within(dialog).getByLabelText("Description:") as HTMLInputElement).value).toBe("Rule 1");
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Condition 1 value" }), { target: { value: "shop" } });
    fireEvent.change(within(dialog).getByRole("combobox", { name: "Action 1 mailbox" }), {
      target: { value: String(FIXTURE.receipts) },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls(mock, "rule.create")).toEqual([
      {
        name: "Rule 1",
        conditions: { match: "any", conditions: [{ field: "from", op: "contains", value: "shop" }] },
        actions: [{ kind: "move", mailboxId: FIXTURE.receipts }],
      },
    ]);
    await waitFor(() => expect(names()).toEqual(["Rule 1"]));
  });

  it("edits, turns off, duplicates, reorders and removes rules", async () => {
    const mock = await setup(async (m) => {
      await m.call<Rule>("rule.create", { name: "Receipts", conditions: anyone, actions: [{ kind: "read" }] });
      await m.call<Rule>("rule.create", { name: "Ann", conditions: anyone, actions: [{ kind: "flag", color: 2 }] });
    });
    await waitFor(() => expect(names()).toEqual(["Receipts", "Ann"]));
    const [receipts, ann] = await mock.call<Rule[]>("rule.list", {});

    fireEvent.doubleClick(screen.getByRole("button", { name: "Ann" }));
    const dialog = screen.getByRole("dialog", { name: "Edit Rule" });
    fireEvent.change(within(dialog).getByLabelText("Description:"), { target: { value: "Ann's mail" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(names()).toEqual(["Receipts", "Ann's mail"]));
    expect(calls(mock, "rule.update")).toEqual([
      { id: ann?.id, name: "Ann's mail", conditions: anyone, actions: [{ kind: "flag", color: 2 }] },
    ]);

    fireEvent.click(screen.getByRole("checkbox", { name: "Enable Receipts" }));
    await waitFor(() => expect(calls(mock, "rule.update")).toContainEqual({ id: receipts?.id, enabled: false }));

    fireEvent.click(screen.getByRole("button", { name: "Receipts" }));
    fireEvent.click(screen.getByRole("button", { name: "Duplicate" }));
    await waitFor(() => expect(names()).toEqual(["Receipts", "Receipts Copy", "Ann's mail"]));
    expect(calls(mock, "rule.create").at(-1)).toEqual({
      name: "Receipts Copy",
      conditions: anyone,
      actions: [{ kind: "read" }],
      enabled: false,
    });

    fireEvent.click(screen.getByRole("button", { name: "Ann's mail" }));
    fireEvent.click(screen.getByRole("button", { name: "Move Up" }));
    await waitFor(() => expect(names()).toEqual(["Receipts", "Ann's mail", "Receipts Copy"]));
    expect(calls(mock, "rule.move").at(-1)).toEqual({ id: ann?.id, position: 1 });

    vi.stubGlobal("confirm", () => false);
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(names()).toHaveLength(3));
    expect(calls(mock, "rule.delete")).toEqual([]);
    const asked = vi.fn(() => true);
    vi.stubGlobal("confirm", asked);
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(names()).toEqual(["Receipts", "Receipts Copy"]));
    expect(asked).toHaveBeenCalledWith(`Remove the rule "Ann's mail"?`);
    expect(calls(mock, "rule.delete")).toEqual([{ id: ann?.id }]);
  });

  it("shows maild's refusal in the sheet and keeps it open", async () => {
    const mock = await setup(async (m) => {
      await m.call("rule.create", { name: "Receipts", conditions: anyone, actions: [{ kind: "read" }] });
    });
    await waitFor(() => expect(names()).toEqual(["Receipts"]));
    fireEvent.click(screen.getByRole("button", { name: "Receipts" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const dialog = screen.getByRole("dialog", { name: "Edit Rule" });
    const failing = vi.spyOn(mock, "call").mockRejectedValueOnce(new Error("condition 1: no such field"));
    fireEvent.click(within(dialog).getByRole("button", { name: "OK" }));
    expect(await within(dialog).findByRole("alert")).toHaveProperty("textContent", "condition 1: no such field");
    expect(screen.getByRole("dialog", { name: "Edit Rule" })).toBe(dialog);
    failing.mockRestore();
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
