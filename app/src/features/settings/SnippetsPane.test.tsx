import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Snippet } from "../../rpc/gen/api";
import { SnippetsPane } from "./SnippetsPane";

const snippets: Snippet[] = [
  { id: 1, name: "Greeting", html: "<p>Hello</p>", updatedAt: "2026-11-07T12:00:00Z" },
  { id: 2, name: "Sign-off", html: "<p>Thanks</p>", updatedAt: "2026-11-07T12:00:00Z" },
];

describe("SnippetsPane", () => {
  it("adds a trimmed name and signatureHtml text", () => {
    const onCreate = vi.fn();
    render(
      <SnippetsPane snippets={[]} busy={false} error={null} onCreate={onCreate} onSave={vi.fn()} onRemove={vi.fn()} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Add Snippet" }));
    fireEvent.change(screen.getByLabelText("Name:"), { target: { value: "  Closing  " } });
    fireEvent.change(screen.getByLabelText("Text:"), { target: { value: "One\n\nThree" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(onCreate).toHaveBeenCalledWith({ name: "Closing", html: "<p>One</p><p><br></p><p>Three</p>" });
  });

  it("selects another snippet and saves or removes the selected one", () => {
    const onSave = vi.fn();
    const onRemove = vi.fn();
    render(
      <SnippetsPane
        snippets={snippets}
        busy={false}
        error={null}
        onCreate={vi.fn()}
        onSave={onSave}
        onRemove={onRemove}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Sign-off" }));
    const save = screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Text:"), { target: { value: "Best" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledWith(2, { name: "Sign-off", html: "<p>Best</p>" });

    fireEvent.click(screen.getByRole("button", { name: "Remove Snippet" }));
    expect(onRemove).toHaveBeenCalledWith(2);
  });

  it("disables Save for a blank name and shows the empty guidance", () => {
    render(
      <SnippetsPane
        snippets={snippets.slice(0, 1)}
        busy={false}
        error={null}
        onCreate={vi.fn()}
        onSave={vi.fn()}
        onRemove={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("Name:"), { target: { value: "   " } });
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);

    render(
      <SnippetsPane snippets={[]} busy={false} error={null} onCreate={vi.fn()} onSave={vi.fn()} onRemove={vi.fn()} />,
    );
    expect(screen.getByText(/Snippets are text you insert/)).toBeTruthy();
  });

  it("has an empty text field in a new form", () => {
    render(
      <SnippetsPane snippets={[]} busy={false} error={null} onCreate={vi.fn()} onSave={vi.fn()} onRemove={vi.fn()} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Add Snippet" }));
    expect((screen.getByLabelText("Text:") as HTMLTextAreaElement).value).toBe("");
  });

  it("selects a snippet that appears with the name just added, and stays open on a taken name", () => {
    const props = { busy: false, onCreate: vi.fn(), onSave: vi.fn(), onRemove: vi.fn() };
    const { rerender } = render(<SnippetsPane snippets={snippets} error={null} {...props} />);
    // A name already listed in another case: maild refuses it, and the new
    // form stays open with the error, not on the existing snippet.
    fireEvent.click(screen.getByRole("button", { name: "Add Snippet" }));
    fireEvent.change(screen.getByLabelText("Name:"), { target: { value: "greeting" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    rerender(<SnippetsPane snippets={snippets} error={'a snippet named "greeting" already exists'} {...props} />);
    expect(screen.getByRole("heading", { name: "New Snippet" })).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toBe('a snippet named "greeting" already exists');

    fireEvent.change(screen.getByLabelText("Name:"), { target: { value: "closing" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    const added = { id: 3, name: "Closing", html: "", updatedAt: "2026-11-07T12:00:00Z" };
    rerender(<SnippetsPane snippets={[added, ...snippets]} error={null} {...props} />);
    expect(screen.queryByRole("heading", { name: "New Snippet" })).toBeNull();
    expect(screen.getByRole("button", { name: "Closing" }).getAttribute("aria-current")).toBe("true");
  });
});
