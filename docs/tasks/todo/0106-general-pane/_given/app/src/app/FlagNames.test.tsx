// CONTRACT TEST for task card T-0106 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null });
});

const label = (el: HTMLElement) => el.querySelector(".flex-1")?.textContent ?? el.textContent;

describe("flag names in the menus", () => {
  it("label the list's Flag Color menu and the toolbar's Flag menu", async () => {
    const mock = new MockTransport(mockData({ inbox: 10, now: new Date("2026-10-08T12:00:00Z") }));
    await mock.call("settings.set", { flagNames: ["Urgent", "", "", "", "", "", ""] });
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await waitFor(() => expect(useMail.getState().settings?.flagNames[0]).toBe("Urgent"));
    const list = await screen.findByRole("listbox", { name: "Messages" });
    const row = await waitFor(() => {
      const r = within(list).queryAllByRole("option")[0];
      if (!r) throw new Error("no rows yet");
      return r;
    });

    fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
    const menu = await screen.findByRole("menu");
    const flagColor = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
      (el) => label(el) === "Flag Color",
    );
    if (!flagColor) throw new Error("no Flag Color");
    fireEvent.click(flagColor);
    await waitFor(() => {
      const colors = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitemcheckbox"]')).map(label);
      expect(colors.slice(0, 2)).toEqual(["Urgent", "Orange"]);
    });
    fireEvent.mouseDown(document.body);
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());

    fireEvent.click(row);
    const flag = screen
      .getAllByRole("button", { name: "Flag" })
      .find((b) => b.getAttribute("aria-haspopup") === "menu");
    if (!flag) throw new Error("no toolbar Flag menu");
    fireEvent.click(flag);
    await waitFor(() => expect(screen.getAllByRole("menuitemcheckbox").map(label)[0]).toBe("Urgent"));
  });
});
