// CONTRACT TEST for task card T-0066 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MOCK_UNREACHABLE } from "../rpc/mock/people";
import { SettingsWindow } from "./SettingsWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  const region = await screen.findByRole("region", { name: "Contacts, Calendars and Tasks" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, calls, region };
}

const box = (name: string) => screen.getByRole("checkbox", { name }) as HTMLInputElement;

describe("SettingsWindow services", () => {
  it("shows the selected account's services", async () => {
    const { calls } = await setup();
    await waitFor(() => expect(box("Contacts").checked).toBe(true));
    expect(box("Calendars").checked).toBe(false);
    expect(calls("account.services")[0]?.params).toEqual({ id: 1 });
    expect(screen.getByText(/^Synced at /)).toBeTruthy();
  });

  it("turns a service off and on", async () => {
    const { calls } = await setup();
    await waitFor(() => expect(box("Contacts").checked).toBe(true));
    fireEvent.click(box("Contacts"));
    await waitFor(() => expect(box("Contacts").checked).toBe(false));
    expect(calls("account.setService").at(-1)?.params).toEqual({ id: 1, service: "contacts", enabled: false });
  });

  it("asks for the server when discovery finds none, and connects to the one given", async () => {
    const { calls, region } = await setup();
    await waitFor(() => expect(box("Calendars").checked).toBe(false));
    fireEvent.click(box("Calendars"));
    const alert = await within(region).findByRole("alert");
    expect(alert.textContent).toContain("enter its address");
    expect(box("Calendars").checked).toBe(false);
    fireEvent.change(within(region).getByLabelText("Server:"), { target: { value: MOCK_UNREACHABLE } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(calls("account.setService")).toHaveLength(2));
    expect(within(region).getByRole("alert")).toBeTruthy();
    fireEvent.change(within(region).getByLabelText("Server:"), { target: { value: "https://dav.example.com/" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(box("Calendars").checked).toBe(true));
    expect(calls("account.setService").at(-1)?.params).toEqual({
      id: 1,
      service: "calendar",
      enabled: true,
      url: "https://dav.example.com/",
    });
    expect(within(region).queryByRole("alert")).toBeNull();
  });
});
