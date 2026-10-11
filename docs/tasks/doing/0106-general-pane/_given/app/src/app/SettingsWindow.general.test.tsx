// CONTRACT TEST for task card T-0106 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

beforeEach(() => {
  useMail.setState({ vips: [], settings: null });
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("navigation", { name: "Accounts" });
  return mock;
}

const sets = (mock: MockTransport) => mock.calls.filter((c) => c.method === "settings.set").map((c) => c.params);

describe("the settings window's General pane", () => {
  it("comes first among the tabs, and the window opens on Accounts", async () => {
    await setup();
    const tabs = screen.getAllByRole("button").filter((b) => b.hasAttribute("aria-pressed"));
    expect(tabs.map((b) => b.textContent)).toEqual(["General", "Accounts", "Signatures", "Sign-In"]);
    expect(tabs[1]?.getAttribute("aria-pressed")).toBe("true");
  });

  it("changes maild's settings", async () => {
    const mock = await setup();
    fireEvent.click(screen.getByRole("button", { name: "General" }));
    const delay = (await screen.findByLabelText("Undo send delay:")) as HTMLSelectElement;
    await waitFor(() => expect(delay.disabled).toBe(false));
    fireEvent.change(delay, { target: { value: "30" } });
    await waitFor(() => expect(sets(mock)).toContainEqual({ undoDelay: 30 }));
    await waitFor(() => expect((screen.getByLabelText("Undo send delay:") as HTMLSelectElement).value).toBe("30"));

    fireEvent.change(screen.getByLabelText("New message notifications:"), { target: { value: "all" } });
    await waitFor(() => expect(sets(mock)).toContainEqual({ notifyScope: "all" }));

    const red = screen.getByLabelText("Flag 1 name");
    fireEvent.change(red, { target: { value: "Urgent" } });
    fireEvent.blur(red);
    await waitFor(() => expect(sets(mock)).toContainEqual({ flagNames: ["Urgent", "", "", "", "", "", ""] }));
    await waitFor(() => expect(useMail.getState().settings?.flagNames[0]).toBe("Urgent"));
  });
});
