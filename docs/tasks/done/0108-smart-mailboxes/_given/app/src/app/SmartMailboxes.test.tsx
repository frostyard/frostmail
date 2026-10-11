// CONTRACT TEST for task card T-0108 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { SmartMailbox, ViewQuery } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null, smarts: [] });
});

function setup() {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { data, mock };
}

const row = (key: string) => document.querySelector<HTMLElement>(`[role="treeitem"][data-key="${key}"]`);
const calls = (mock: MockTransport, method: string) =>
  mock.calls.filter((c) => c.method === method).map((c) => c.params as Record<string, unknown>);

function lastQuery(mock: MockTransport): ViewQuery | undefined {
  const opens = mock.calls.filter((c) => c.method === "view.open");
  return (opens.at(-1)?.params as { query: ViewQuery } | undefined)?.query;
}

describe("smart mailboxes in the main window", () => {
  it("are made in the sheet, listed, edited and deleted", async () => {
    const { mock } = setup();
    fireEvent.click(await screen.findByRole("button", { name: "New Smart Mailbox" }));
    const dialog = await screen.findByRole("dialog", { name: "New Smart Mailbox" });
    fireEvent.change(within(dialog).getByLabelText("Smart Mailbox Name:"), { target: { value: "From Ann" } });
    fireEvent.change(within(dialog).getByRole("combobox", { name: "Condition 1 operator" }), {
      target: { value: "is" },
    });
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Condition 1 value" }), {
      target: { value: "ann.smith@northwind.test" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "OK" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls(mock, "smart.create")[0]).toMatchObject({
      name: "From Ann",
      conditions: { match: "all", conditions: [{ field: "from", op: "is", value: "ann.smith@northwind.test" }] },
      includeTrash: false,
      includeSent: false,
    });
    const smarts = await mock.call<SmartMailbox[]>("smart.list", {});
    const id = smarts[0]?.id;
    await waitFor(() => expect(row(`smart:${id}`)?.textContent).toContain("From Ann"));
    await waitFor(() => expect(lastQuery(mock)).toEqual({ smartMailboxId: id, threads: true }));
    await waitFor(() => expect(document.querySelector(".text-toolbar-title")?.textContent).toBe("From Ann"));

    const smartRow = row(`smart:${id}`);
    if (!smartRow) throw new Error("no row");
    fireEvent.contextMenu(smartRow, { clientX: 20, clientY: 20 });
    fireEvent.click(await screen.findByText("Edit Smart Mailbox…"));
    const edit = await screen.findByRole("dialog", { name: "Edit Smart Mailbox" });
    expect((within(edit).getByLabelText("Smart Mailbox Name:") as HTMLInputElement).value).toBe("From Ann");
    fireEvent.change(within(edit).getByLabelText("Smart Mailbox Name:"), { target: { value: "Ann" } });
    fireEvent.click(within(edit).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(calls(mock, "smart.update")[0]).toMatchObject({ id, name: "Ann" }));
    await waitFor(() => expect(row(`smart:${id}`)?.textContent).toContain("Ann"));

    const again = row(`smart:${id}`);
    if (!again) throw new Error("no row");
    fireEvent.contextMenu(again, { clientX: 20, clientY: 20 });
    fireEvent.click(await screen.findByText("Delete Smart Mailbox"));
    await waitFor(() => expect(row(`smart:${id}`)).toBeNull());
    await waitFor(() => expect(useUI.getState().source).toEqual({ kind: "allInboxes" }));
  });

  it("saves a search", async () => {
    useUI.setState({ search: "Offsite", searchDraft: "Offsite" });
    const { mock } = setup();
    fireEvent.click(await screen.findByRole("button", { name: "Save as Smart Mailbox" }));
    const dialog = await screen.findByRole("dialog", { name: "New Smart Mailbox" });
    await waitFor(() =>
      expect((within(dialog).getByRole("textbox", { name: "Condition 1 value" }) as HTMLInputElement).value).toBe(
        "Offsite",
      ),
    );
    expect(calls(mock, "smart.fromSearch")[0]).toEqual({ text: "Offsite" });
    expect((within(dialog).getByLabelText("Smart Mailbox Name:") as HTMLInputElement).value).toBe("Offsite");
    expect(
      (within(dialog).getByRole("checkbox", { name: "Include messages from Trash" }) as HTMLInputElement).checked,
    ).toBe(true);
    fireEvent.click(within(dialog).getByRole("button", { name: "OK" }));
    await waitFor(() =>
      expect(calls(mock, "smart.create")[0]).toMatchObject({ name: "Offsite", includeTrash: true, includeSent: true }),
    );
  });
});
