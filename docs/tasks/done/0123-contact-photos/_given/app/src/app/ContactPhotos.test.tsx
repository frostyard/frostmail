// CONTRACT TEST for task card T-0123 (docs/tasks). Do not edit.
import { render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

async function setup(photos: boolean) {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  const ann = data.pim?.people.find((p) =>
    p.contacts.some((c) => c.emails.some((e) => e.value === "ann.smith@northwind.test")),
  );
  if (!ann || !data.pim) throw new Error("the fixture has no Ann Smith");
  data.pim.photos = { [ann.id]: { contentType: "image/png", data: "iVBORw0KGgo=" } };
  useUI.setState({ contactPhotos: photos });
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  await waitFor(() => expect(within(list).queryAllByRole("option").length).toBeGreaterThan(3));
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { list, calls, mock };
}

describe("contact photos in the list", () => {
  it("shows the senders' photos, asking about each sender and person once", async () => {
    const { list, calls, mock } = await setup(true);
    const fromAnn = (
      await mock.call<{ id: number; from: { address: string } }[]>("message.summaries", {
        ids: within(list)
          .getAllByRole("option")
          .map((r) => Number(r.getAttribute("data-message-id"))),
      })
    ).filter((s) => s.from.address === "ann.smith@northwind.test");
    expect(fromAnn.length).toBeGreaterThan(0);
    const row = list.querySelector(`[data-message-id="${fromAnn[0]?.id}"]`) as HTMLElement;
    await waitFor(() =>
      expect(row.querySelector("[data-avatar] img")?.getAttribute("src")).toBe("data:image/png;base64,iVBORw0KGgo="),
    );
    const others = within(list)
      .getAllByRole("option")
      .filter((r) => !fromAnn.some((s) => String(s.id) === r.getAttribute("data-message-id")));
    expect(others.every((r) => r.querySelector("[data-avatar] img") === null)).toBe(true);
    expect(others.every((r) => r.querySelector("[data-avatar]") !== null)).toBe(true);
    const asked = calls("people.senders").flatMap((p) => (p as { addresses: string[] }).addresses);
    expect(new Set(asked).size).toBe(asked.length);
    expect(calls("people.photo")).toHaveLength(1);
  });

  it("asks nothing with Contact Photos off", async () => {
    const { list, calls } = await setup(false);
    expect(list.querySelector("[data-avatar]")).toBeNull();
    expect(calls("people.senders")).toEqual([]);
  });
});
