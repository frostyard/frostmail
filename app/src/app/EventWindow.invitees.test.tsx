// CONTRACT TEST for task card T-0159 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { CALENDARS, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { EventWindow } from "./EventWindow";
import type { EditorTarget } from "./openEvent";

// The fixture's day is 2026-10-08 (UTC): 302 Design review is the user's
// in Work and invites Carmen Lindqvist (accepted) and Ines Kim (maybe,
// optional). With invites, Work and Home invite, as on Google or iCloud.
async function setup(target: EditorTarget, invites: boolean) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  for (const col of data.pim?.collections ?? []) col.invites = invites && col.kind === "calendar" && !col.readOnly;
  const mock = new MockTransport(data);
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <EventWindow target={target} />
    </Session>,
  );
  await screen.findByLabelText("Title");
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { calls, close };
}

const fresh: EditorTarget = {
  kind: "new",
  allDay: false,
  start: "2026-10-09T15:00:00.000Z",
  end: "2026-10-09T16:00:00.000Z",
  startDate: "",
  endDate: "",
  calendarId: CALENDARS.home,
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the event editor's invitees", () => {
  it("lists an event's invitees, keeps it in its calendar, and invites one more", async () => {
    const user = userEvent.setup();
    const { calls, close } = await setup({ kind: "edit", id: 302, recurrenceId: "" }, true);
    const list = within(screen.getByRole("list", { name: "Invitees" }));
    expect(list.getByText("Carmen Lindqvist")).toBeTruthy();
    expect(list.getByLabelText("Accepted")).toBeTruthy();
    expect(list.getByText("Ines Kim")).toBeTruthy();
    const calendar = screen.getByLabelText("Calendar") as HTMLSelectElement;
    expect([...calendar.options].map((o) => o.value)).toEqual([String(CALENDARS.work)]);

    const add = screen.getByRole("combobox", { name: "Add Invitee" });
    await user.click(add);
    await user.type(add, "erin@acme.test{Enter}");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const sheet = within(await screen.findByRole("dialog", { name: "Send Invitations?" }));
    expect(sheet.getByText("The invitee will get it by email.")).toBeTruthy();
    await user.click(sheet.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("calendar.update").at(-1)?.params).toEqual({
      id: 302,
      fields: {
        attendees: [
          { email: "carmen@vertex.test", name: "Carmen Lindqvist", role: "required" },
          { email: "ines.kim@vertex.test", name: "Ines Kim", role: "optional" },
          { email: "erin@acme.test", role: "required" },
        ],
      },
      notify: true,
    });
  });

  it("invites from a new event", async () => {
    const user = userEvent.setup();
    const { calls, close } = await setup(fresh, true);
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Planning" } });
    const add = screen.getByRole("combobox", { name: "Add Invitee" });
    await user.click(add);
    await user.type(add, "Carl Diaz <carl@example.test>{Enter}");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: "Send Invitations?" })).getByRole("button", { name: "Send" }),
    );
    await waitFor(() => expect(close).toHaveBeenCalled());
    const create = calls("calendar.create").at(-1)?.params as {
      calendarId: number;
      fields: { attendees: unknown };
      notify: boolean;
    };
    expect(create.calendarId).toBe(CALENDARS.home);
    expect(create.fields.attendees).toEqual([{ email: "carl@example.test", name: "Carl Diaz", role: "required" }]);
    expect(create.notify).toBe(true);
  });

  it("offers no invitees where the calendar does not invite", async () => {
    const { calls } = await setup(fresh, false);
    expect(screen.queryByRole("combobox", { name: "Add Invitee" })).toBeNull();
    expect(screen.getByText("Invitations aren't available in this calendar.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Focus" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls("calendar.create")).toHaveLength(1));
    const created = calls("calendar.create")[0]?.params as { fields: { attendees?: unknown } } | undefined;
    expect(created?.fields).toBeDefined();
    expect(created?.fields.attendees).toBeUndefined();
  });

  it("keeps Save off while an invitee's address is not one", async () => {
    const user = userEvent.setup();
    await setup(fresh, true);
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Planning" } });
    const add = screen.getByRole("combobox", { name: "Add Invitee" });
    await user.click(add);
    await user.type(add, "erin{Enter}");
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Check the invitees' addresses")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Remove erin" }));
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(false);
  });
});
