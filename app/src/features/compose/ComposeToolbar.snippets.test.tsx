import { readFileSync } from "node:fs";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ComposeToolbar, type ComposeToolbarProps } from "./ComposeToolbar";

afterEach(() => vi.restoreAllMocks());

const props = (snippets: { id: number; name: string }[]): ComposeToolbarProps => ({
  subject: "Lunch",
  canSend: true,
  formatBarShown: false,
  maximized: false,
  snippets,
  onInsertSnippet: vi.fn(),
  onCommand: vi.fn(),
});

describe("ComposeToolbar snippets menu", () => {
  it("keeps the highlighted row in view", async () => {
    const snippets = Array.from({ length: 40 }, (_, index) => ({ id: index + 1, name: `Snippet ${index + 1}` }));
    render(<ComposeToolbar {...props(snippets)} />);

    const revealed: HTMLElement[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (this: HTMLElement) {
      revealed.push(this);
    });

    fireEvent.click(screen.getByRole("button", { name: "Insert Snippet" }));
    const menu = await screen.findByRole("menu", { name: "Snippets" });
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.keyDown(menu, { key: "ArrowDown" });

    await waitFor(() => expect(revealed.length).toBeGreaterThan(0));
    const row = menu.querySelector('[data-highlighted="true"]');
    expect(row).toBeInstanceOf(HTMLElement);
    expect(revealed.at(-1)).toBe(row);
  });

  it("bounds the labelled Snippets menu in app.css", () => {
    const css = readFileSync("src/styles/app.css", "utf8");
    expect(css).toContain('[role="menu"][aria-label="Snippets"]');
    expect(css).toContain("max-height");
  });
});
