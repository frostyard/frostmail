// The settings window saves the account's sync window (docs/specs/settings-ui.md,
// Behavior (container): Edit; ADR-0016).
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Session } from "../data/session";
import type { AccountUpdateParams } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

describe("an existing account's sync window", () => {
  it("starts at All mail and saves Last year", async () => {
    const user = userEvent.setup();
    const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <SettingsWindow />
      </Session>,
    );
    await screen.findByRole("navigation", { name: "Accounts" });
    const select = (await screen.findByLabelText("Keep mail from:")) as HTMLSelectElement;
    expect(select.options[select.selectedIndex]?.text).toBe("All mail");

    await user.selectOptions(select, "Last year");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      const update = mock.calls.find(
        (call) => call.method === "account.update" && (call.params as AccountUpdateParams).syncDays === 365,
      );
      expect(update).toBeDefined();
    });
    await waitFor(() => expect(select.options[select.selectedIndex]?.text).toBe("Last year"));
  });
});
