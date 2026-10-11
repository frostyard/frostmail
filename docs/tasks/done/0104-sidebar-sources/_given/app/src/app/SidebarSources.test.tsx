// CONTRACT TEST for task card T-0104 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { PersonSummary, ViewQuery } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null });
});

const ANN = ["ann.smith@northwind.test", "ann@smith-family.test"];

async function setup() {
  const data = mockData({ inbox: 30, now: new Date("2026-10-08T12:00:00Z") });
  const mock = new MockTransport(data);
  const people = await mock.call<PersonSummary[]>("people.list", { query: "ann" });
  const ann = people.find((p) => p.displayName === "Ann Smith");
  if (!ann) throw new Error("no Ann in the fixture");
  await mock.call("vip.add", { personId: ann.id });
  await mock.call("settings.set", { flagNames: ["Urgent", "", "", "", "", "", ""] });
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { data, mock };
}

const row = (key: string) => document.querySelector<HTMLElement>(`[role="treeitem"][data-key="${key}"]`);

function lastQuery(mock: MockTransport): ViewQuery | undefined {
  const opens = mock.calls.filter((c) => c.method === "view.open");
  return (opens.at(-1)?.params as { query: ViewQuery } | undefined)?.query;
}

describe("the sidebar's VIPs and flag colors", () => {
  it("lists them with their counts", async () => {
    const { data } = await setup();
    const fromAnn = data.messages.filter((m) => ANN.includes(m.summary.from.address));
    const flagged = data.messages.filter((m) => m.summary.flags.flagged);
    const red = flagged.filter((m) => m.summary.flags.flagColor === 1);
    expect(red.length).toBeGreaterThan(0);

    await waitFor(() => expect(row("vips")).not.toBeNull());
    const annRow = row(`vip:${ANN[0]}`);
    expect(annRow?.textContent).toContain("Ann Smith");
    expect(annRow?.getAttribute("aria-level")).toBe("2");
    expect(annRow?.querySelector('[data-icon="user"]')).not.toBeNull();
    const annUnread = fromAnn.filter((m) => !m.summary.flags.seen).length;
    if (annUnread > 0) await waitFor(() => expect(row(`vip:${ANN[0]}`)?.textContent).toContain(String(annUnread)));

    await waitFor(() => expect(row("flagged")?.textContent).toContain(String(flagged.length)));
    await waitFor(() => expect(row("flag:1")).not.toBeNull());
    const urgent = row("flag:1");
    expect(urgent?.textContent).toContain("Urgent");
    expect(urgent?.textContent).toContain(String(red.length));
    expect(urgent?.querySelector('[data-icon="flag"]')?.getAttribute("class")).toContain("text-flag-1");
    for (let color = 1; color <= 7; color++) {
      const n = flagged.filter((m) => m.summary.flags.flagColor === color).length;
      if (n === 0) expect(row(`flag:${color}`)).toBeNull();
    }
  });

  it("shows a VIP's mail and a color's mail", async () => {
    const { mock } = await setup();
    await waitFor(() => expect(row(`vip:${ANN[0]}`)).not.toBeNull());
    const annRow = row(`vip:${ANN[0]}`);
    if (!annRow) throw new Error("no Ann row");
    fireEvent.click(annRow);
    await waitFor(() =>
      expect(lastQuery(mock)).toEqual({
        conditions: {
          match: "any",
          conditions: ANN.map((value) => ({ field: "from", op: "is", value })),
        },
        threads: true,
      }),
    );
    await waitFor(() => expect(document.querySelector(".text-toolbar-title")?.textContent).toBe("Ann Smith"));
    const list = screen.getByRole("listbox", { name: "Messages" });
    await waitFor(() => expect(within(list).queryAllByRole("option").length).toBeGreaterThan(0));

    await waitFor(() => expect(row("flag:1")).not.toBeNull());
    const urgent = row("flag:1");
    if (!urgent) throw new Error("no Urgent row");
    fireEvent.click(urgent);
    await waitFor(() =>
      expect(lastQuery(mock)).toEqual({
        conditions: { match: "all", conditions: [{ field: "color", op: "is", value: "1" }] },
        threads: true,
      }),
    );
    await waitFor(() => expect(document.querySelector(".text-toolbar-title")?.textContent).toBe("Urgent"));
  });

  it("follows VIP changes", async () => {
    const { mock } = await setup();
    await waitFor(() => expect(row(`vip:${ANN[0]}`)).not.toBeNull());
    await mock.call("vip.add", { addresses: ["kofi.okafor@acme.test"] });
    await waitFor(() => expect(row("vip:kofi.okafor@acme.test")?.textContent).toContain("Kofi Okafor"));
    await mock.call("vip.remove", { addresses: ["kofi.okafor@acme.test", ...ANN] });
    await waitFor(() => expect(row("vips")).toBeNull());
  });
});
