import { act, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("Invitation card", () => {
  // Two clicks before the card re-renders (a double click) send one
  // answer: the organizer gets one reply.
  it("answers once for a double click", async () => {
    const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), invitation: true });
    const mock = new MockTransport(data);
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("tree", { name: "Mailboxes" });
    const id = data.messages
      .filter((m) => m.summary.subject === "Invitation: Lunch with Ann" && m.summary.mailboxIds.includes(FIXTURE.inbox))
      .at(-1)?.summary.id;
    if (id === undefined) throw new Error("no invitation");
    act(() => useUI.getState().select([id], id));
    const card = await screen.findByRole("region", { name: "Invitation" });
    const accept = within(card).getByRole("button", { name: "Accept" });
    act(() => {
      accept.click();
      accept.click();
    });
    const responds = () => mock.calls.filter((c) => c.method === "calendar.respond");
    await waitFor(() => expect(responds()).toHaveLength(1));
    await waitFor(() => expect(accept.hasAttribute("disabled")).toBe(false));
    expect(responds()).toHaveLength(1);
  });
});
