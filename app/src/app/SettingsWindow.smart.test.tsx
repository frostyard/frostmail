// CONTRACT TEST for task card T-0110 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail } from "../data/stores";
import type { SmartMailbox } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

beforeEach(() => {
  useMail.setState({ vips: [], settings: null, smarts: [] });
});

describe("a smart mailbox as the notification scope", () => {
  it("is chosen in the General pane", async () => {
    const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
    const urgent = await mock.call<SmartMailbox>("smart.create", {
      name: "Urgent",
      conditions: { match: "all", conditions: [{ field: "flagged", op: "is", value: "true" }] },
    });
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <SettingsWindow />
      </Session>,
    );
    await screen.findByRole("navigation", { name: "Accounts" });
    fireEvent.click(screen.getByRole("button", { name: "General" }));
    const scope = (await screen.findByLabelText("New message notifications:")) as HTMLSelectElement;
    await waitFor(() => expect(Array.from(scope.options).some((o) => o.value === `smart:${urgent.id}`)).toBe(true));
    await waitFor(() => expect(scope.disabled).toBe(false));
    fireEvent.change(scope, { target: { value: `smart:${urgent.id}` } });
    await waitFor(() =>
      expect(mock.calls.filter((c) => c.method === "settings.set").map((c) => c.params)).toContainEqual({
        notifyScope: "smart",
        notifySmartId: urgent.id,
      }),
    );
    await waitFor(() => expect(useMail.getState().settings?.notifySmartId).toBe(urgent.id));
    expect((screen.getByLabelText("New message notifications:") as HTMLSelectElement).value).toBe(`smart:${urgent.id}`);
  });
});
