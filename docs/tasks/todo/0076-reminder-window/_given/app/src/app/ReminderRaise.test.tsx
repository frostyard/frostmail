// CONTRACT TEST for task card T-0076 (docs/tasks). Do not edit.
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { openOccurrenceInMain } from "./reminders";

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("raising the reminder window", () => {
  it("opens it when the main window connects with reminders due, and on each announcement", async () => {
    const opened = vi.spyOn(window, "open").mockImplementation(() => null);
    const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), reminders: true }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("tree", { name: "Mailboxes" });
    await waitFor(() => expect(opened).toHaveBeenCalledWith(expect.stringContaining("#/reminders"), "reminders"));
    opened.mockClear();
    // A snooze that already ended keeps the reminder due and announces it.
    const c = new Client(mock);
    const [first] = await c.calendar.reminders({});
    await c.calendar.snooze({ ids: [first?.id ?? ""], until: "2026-10-08T11:00:00Z" });
    await waitFor(() => expect(opened).toHaveBeenCalledWith(expect.stringContaining("#/reminders"), "reminders"));
  });

  it("opens an occurrence the reminder window asks for", async () => {
    vi.spyOn(window, "open").mockImplementation(() => null);
    const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), reminders: true }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("tree", { name: "Mailboxes" });
    await openOccurrenceInMain({ eventId: 302, recurrenceId: "", date: "2026-10-08" });
    await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
    expect(useUI.getState().calendarSelected).toEqual({ eventId: 302, recurrenceId: "" });
    expect(useUI.getState().calendarDate).toBe("2026-10-08");
  });
});
