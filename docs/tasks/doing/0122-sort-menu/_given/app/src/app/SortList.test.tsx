// CONTRACT TEST for task card T-0122 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import type { ViewQuery } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 12, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("listbox", { name: "Messages" });
  const queries = () =>
    mock.calls.filter((c) => c.method === "view.open").map((c) => (c.params as { query: ViewQuery }).query);
  return { mock, queries };
}

function choose(label: string) {
  fireEvent.click(screen.getByRole("button", { name: /^Sort by / }));
  fireEvent.click(within(screen.getByRole("menu")).getByText(label));
}

describe("sorting the list", () => {
  it("opens the list in the chosen order, and keeps the filter bar as it was", async () => {
    const { queries } = await setup();
    const filters = screen.getByRole("toolbar", { name: "Filter messages" });
    expect(within(filters).queryByRole("button", { name: /^Sort by / })).toBeNull();
    choose("Size");
    await waitFor(() => expect(queries().at(-1)).toMatchObject({ sort: "size" }));
    expect(queries().at(-1)?.ascending).toBeUndefined();
    expect(screen.getByRole("button", { name: /^Sort by / }).textContent).toBe("Sort by Size");
    choose("Ascending");
    await waitFor(() => expect(queries().at(-1)).toMatchObject({ sort: "size", ascending: true }));
    choose("Conversations");
    await waitFor(() => expect(queries().at(-1)?.threads).toBeUndefined());
    expect(useUI.getState().conversations).toBe(false);
  });
});
