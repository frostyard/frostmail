// New accounts start writable since M5 (docs/specs/parity.md, P-952).
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

describe("a new account", () => {
  it("starts with Read only clear", async () => {
    const user = userEvent.setup();
    const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <SettingsWindow />
      </Session>,
    );
    await screen.findByRole("navigation", { name: "Accounts" });
    await user.click(screen.getByRole("button", { name: "Add Account" }));
    const readOnly = screen.getByRole("checkbox", { name: /Read only/ }) as HTMLInputElement;
    expect(readOnly.checked).toBe(false);
  });
});
