// CONTRACT TEST for task card T-0089 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup(invitation = true) {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), invitation }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, calls };
}

async function openMessage(subject: string) {
  const row = await waitFor(() => {
    const found = screen.getAllByRole("option").find((o) => o.textContent?.includes(subject));
    if (!found) throw new Error(`no ${subject}`);
    return found;
  });
  fireEvent.click(row);
  return row;
}

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("Invitation card", () => {
  it("shows an invitation above the message, without its .ics attachment", async () => {
    const { calls } = await setup();
    await openMessage("Invitation: Lunch with Ann");
    const card = await screen.findByRole("region", { name: "Invitation" });
    expect(within(card).getByText("Lunch with Ann")).toBeTruthy();
    expect(within(card).getByText("Cafe Nord · Ann Smith (organizer)")).toBeTruthy();
    expect(within(card).getByText("Before: Standup")).toBeTruthy();
    const id = useUI.getState().selected[0];
    expect(calls("calendar.invitation")).toContainEqual({ messageId: id });
    await waitFor(() => expect(screen.queryByRole("button", { name: /invite\.ics/ })).toBeNull());
    expect(screen.queryByText("invite.ics")).toBeNull();
  });

  it("answers, and shows the answer", async () => {
    const { calls } = await setup();
    await openMessage("Invitation: Lunch with Ann");
    const card = await screen.findByRole("region", { name: "Invitation" });
    const id = useUI.getState().selected[0];
    fireEvent.click(within(card).getByRole("button", { name: "Accept" }));
    await waitFor(() => expect(calls("calendar.respond")).toContainEqual({ messageId: id, answer: "accepted" }));
    await waitFor(() =>
      expect(
        within(screen.getByRole("region", { name: "Invitation" }))
          .getByRole("button", { name: "Accept" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    const before = calls("calendar.invitation").length;
    expect(before).toBeGreaterThanOrEqual(2);
  });

  it("opens the event in Calendar", async () => {
    await setup();
    await openMessage("Invitation: Lunch with Ann");
    const card = await screen.findByRole("region", { name: "Invitation" });
    fireEvent.click(within(card).getByRole("button", { name: "Show in Calendar" }));
    await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
    expect(useUI.getState().calendarDate).toBe("2026-10-09");
    expect(useUI.getState().calendarSelected).toEqual({ eventId: 303, recurrenceId: "" });
  });

  it("asks nothing for a message without an invitation", async () => {
    const { calls } = await setup(false);
    await openMessage("Re: Offsite plan");
    await screen.findAllByRole("article", { name: "Re: Offsite plan" });
    expect(screen.queryByRole("region", { name: "Invitation" })).toBeNull();
    expect(calls("calendar.invitation")).toEqual([]);
  });
});
