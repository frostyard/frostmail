// CONTRACT TEST for task card T-0105 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { PersonSummary } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null });
});

const ANN = "ann.smith@northwind.test";

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

function rows(): HTMLElement[] {
  return within(screen.getByRole("listbox", { name: "Messages" })).queryAllByRole("option");
}

const star = (row: HTMLElement | undefined) => (row ? within(row).queryByRole("img", { name: "VIP" }) : null);

describe("VIP stars and Add to VIPs", () => {
  it("star a VIP's rows and not the others", async () => {
    const { data, mock } = setup();
    await waitFor(() => expect(rows().length).toBeGreaterThan(1));
    const sender = (row: HTMLElement) =>
      data.messages.find((m) => String(m.summary.id) === row.getAttribute("data-message-id"))?.summary.from.address;
    const first = rows()[0];
    if (!first) throw new Error("no row");
    const address = sender(first);
    if (!address) throw new Error("no sender");
    expect(star(first)).toBeNull();
    await mock.call("vip.add", { addresses: [address] });
    await waitFor(() => {
      for (const row of rows()) {
        if (sender(row) === address) expect(star(row)).not.toBeNull();
        else expect(star(row)).toBeNull();
      }
      expect(star(rows()[0])).not.toBeNull();
    });
  });

  it("make the sender a VIP from the contact card", async () => {
    useUI.setState({ search: "Offsite", searchDraft: "Offsite" });
    const { mock } = setup();
    const people = await mock.call<PersonSummary[]>("people.list", { query: "ann" });
    const ann = people.find((p) => p.displayName === "Ann Smith");
    if (!ann) throw new Error("no Ann in the fixture");
    await waitFor(() => expect(rows().length).toBeGreaterThan(0));
    const first = rows()[0];
    if (!first) throw new Error("no row");
    fireEvent.click(first);

    const sender = (await screen.findAllByRole("button", { name: ANN }))[0];
    if (!sender) throw new Error("no sender button");
    const header = sender.closest("header");
    expect(header && within(header).queryByRole("img", { name: "VIP" })).toBeNull();
    fireEvent.click(sender);
    fireEvent.click(await screen.findByRole("button", { name: "Add to VIPs" }));
    await waitFor(() =>
      expect(
        mock.calls.find((c) => c.method === "vip.add" && (c.params as { personId?: number }).personId === ann.id),
      ).toBeDefined(),
    );
    await screen.findByRole("button", { name: "Remove from VIPs" });
    await waitFor(() => expect(star(rows()[0])).not.toBeNull());
    await waitFor(() => {
      const h = screen.getAllByRole("button", { name: ANN })[0]?.closest("header");
      expect(h && within(h).queryByRole("img", { name: "VIP" })).not.toBeNull();
    });
  });
});
