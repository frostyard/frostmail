import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow } from "./ComposeWindow";

async function setup(
  html = "<p>Old</p>",
  snippets = [
    { id: 2, name: "b", html: "<p>Bee</p>", updatedAt: "2026-11-07T12:00:00Z" },
    { id: 1, name: "A", html: "<p>Hello</p>", updatedAt: "2026-11-07T12:00:00Z" },
  ],
) {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-11-07T12:00:00Z"), snippets }), {
    undoMs: 50,
  });
  const client = new Client(mock);
  const draft = await client.draft.create({ kind: "new" });
  await client.draft.update({
    id: draft.id,
    content: { ...draft.content, subject: "Hi", html, to: [{ name: "Bob", address: "bob@x.test" }] },
  });
  vi.spyOn(window, "close").mockImplementation(() => {});
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
  await waitFor(() => expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(false));
  return { mock, draft };
}

afterEach(() => vi.restoreAllMocks());

describe("snippets in compose", () => {
  it("lists snippets in maild order and Edit Snippets", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: "Insert Snippet" }));
    const menu = await screen.findByRole("menu");
    // maild's order (by name without case), not the order they were made.
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["A", "b", "Edit Snippets…"]);
  });

  it("shows a disabled No Snippets item", async () => {
    await setup("<p>Old</p>", []);
    fireEvent.click(screen.getByRole("button", { name: "Insert Snippet" }));
    const item = await screen.findByRole("menuitem", { name: "No Snippets" });
    expect(item.getAttribute("aria-disabled")).toBe("true");
  });

  it("inserts at the cursor and replaces a selection", async () => {
    const user = userEvent.setup();
    const { mock } = await setup();
    fireEvent.click(screen.getByRole("button", { name: "Insert Snippet" }));
    await user.click(within(await screen.findByRole("menu")).getByText("A"));
    const body = await screen.findByRole("textbox", { name: "Message body" });
    await waitFor(() => expect(body.textContent).toBe("HelloOld"));
    // Focus returns to the editor.
    expect(body.contains(document.activeElement)).toBe(true);

    await user.click(body);
    await user.keyboard("{Control>}a{/Control}");
    fireEvent.click(screen.getByRole("button", { name: "Insert Snippet" }));
    await user.click(within(await screen.findByRole("menu")).getByText("b"));
    await waitFor(() => {
      expect(body.textContent).toBe("Bee");
      const updates = mock.calls.filter((call) => call.method === "draft.update");
      expect(JSON.stringify(updates.at(-1)?.params)).toContain("Bee");
    });
  });

  it("drops a remote image and reaches draft.update", async () => {
    const user = userEvent.setup();
    const { mock } = await setup("<p>Old</p>", [
      {
        id: 1,
        name: "Remote",
        html: '<p>Safe</p><img src="https://example.test/pixel.png">',
        updatedAt: "2026-11-07T12:00:00Z",
      },
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Insert Snippet" }));
    await user.click(within(await screen.findByRole("menu")).getByText("Remote"));
    const body = await screen.findByRole("textbox", { name: "Message body" });
    await waitFor(() => expect(body.textContent).toContain("Safe"));
    expect(body.querySelector('img[src="https://example.test/pixel.png"]')).toBeNull();
    await waitFor(() => expect(mock.calls.some((call) => call.method === "draft.update")).toBe(true));
  });
});
