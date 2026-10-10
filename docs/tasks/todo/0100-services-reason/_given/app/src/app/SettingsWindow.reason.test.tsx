// CONTRACT TEST for task card T-0100 (docs/tasks). Do not edit.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

describe("an iCloud account's services", () => {
  it("say why Tasks is missing", async () => {
    const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-10T12:00:00Z") }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <SettingsWindow />
      </Session>,
    );
    await screen.findByRole("navigation", { name: "Accounts" });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Add Account" }));
    await user.type(screen.getByLabelText("Email Address:"), "ann@icloud.test");
    await user.selectOptions(screen.getByLabelText("Account Type:"), "icloud");
    await user.type(screen.getByLabelText("Password:"), "app-specific");
    const submit = screen
      .getAllByRole("button", { name: "Add Account" })
      .find((b) => b.getAttribute("type") === "submit");
    if (!submit) throw new Error("no submit button");
    await user.click(submit);
    await waitFor(() => expect((screen.getByLabelText("Email Address:") as HTMLInputElement).readOnly).toBe(true));
    const services = await screen.findByRole("region", { name: "Contacts, Calendars and Tasks" });
    expect(
      await within(services).findByText("Apple Reminders can't be reached by apps outside Apple's (since iOS 13)."),
    ).toBeTruthy();
  });
});
