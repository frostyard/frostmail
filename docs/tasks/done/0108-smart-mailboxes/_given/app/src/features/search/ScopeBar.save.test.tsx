// CONTRACT TEST for task card T-0108 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ScopeBar } from "./SearchField";

const scopes = [
  { key: "all", label: "All Mailboxes" },
  { key: "source", label: "Inbox" },
];

describe("the scope bar's Save", () => {
  it("saves the search as a smart mailbox", () => {
    const onSave = vi.fn();
    render(<ScopeBar scopes={scopes} selected="all" onSelect={vi.fn()} onSave={onSave} />);
    const save = screen.getByRole("button", { name: "Save as Smart Mailbox" });
    expect(save.textContent).toBe("Save");
    expect(save.className).toContain("text-accent");
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("is absent without onSave", () => {
    render(<ScopeBar scopes={scopes} selected="all" onSelect={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Save as Smart Mailbox" })).toBeNull();
  });
});
