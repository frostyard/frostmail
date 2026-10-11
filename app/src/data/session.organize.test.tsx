// The mail store's VIPs and settings (docs/specs/ui.md; M5): loaded when
// the window connects and kept current from vip.changed and
// settings.changed.
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { Session } from "./session";
import { useMail } from "./stores";

beforeEach(() => {
  useMail.setState({ vips: [], settings: null, smarts: [] });
});

describe("the mail store's preferences", () => {
  it("loads VIPs and settings and follows their changes", async () => {
    const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <div>ready</div>
      </Session>,
    );
    await screen.findByText("ready");
    await waitFor(() => expect(useMail.getState().settings?.undoDelay).toBe(10));
    expect(useMail.getState().vips).toEqual([]);

    await mock.call("vip.add", { addresses: ["kofi.okafor@acme.test"] });
    await waitFor(() => expect(useMail.getState().vips.map((v) => v.address)).toEqual(["kofi.okafor@acme.test"]));
    await mock.call("settings.set", { undoDelay: 0, notifyScope: "vips" });
    await waitFor(() => expect(useMail.getState().settings).toMatchObject({ undoDelay: 0, notifyScope: "vips" }));
    await mock.call("smart.create", { name: "Unread", conditions: { match: "all", conditions: [] } });
    await waitFor(() => expect(useMail.getState().smarts.map((s) => s.name)).toEqual(["Unread"]));
  });
});
