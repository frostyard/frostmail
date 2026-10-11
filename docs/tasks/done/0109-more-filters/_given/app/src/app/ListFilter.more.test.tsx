// CONTRACT TEST for task card T-0109 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { ViewQuery } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null, smarts: [] });
});

function lastQuery(mock: MockTransport): ViewQuery | undefined {
  const opens = mock.calls.filter((c) => c.method === "view.open");
  return (opens.at(-1)?.params as { query: ViewQuery } | undefined)?.query;
}

describe("the More filters in the window", () => {
  it("narrow the list to mail to me", async () => {
    const mock = new MockTransport(mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    const bar = await screen.findByRole("toolbar", { name: "Filter messages" });
    fireEvent.click(within(bar).getByRole("button", { name: "More filters" }));
    fireEvent.click(await screen.findByRole("menuitemcheckbox", { name: /To Me/ }));
    await waitFor(() =>
      expect(lastQuery(mock)).toEqual({
        role: "inbox",
        threads: true,
        filter: { match: "all", conditions: [{ field: "tome", op: "is", value: "true" }] },
      }),
    );
    const list = screen.getByRole("listbox", { name: "Messages" });
    await waitFor(() => expect(within(list).queryAllByRole("option").length).toBeGreaterThan(0));
    expect(within(bar).getByRole("button", { name: "More filters" }).textContent).toBe("To Me");
  });
});
