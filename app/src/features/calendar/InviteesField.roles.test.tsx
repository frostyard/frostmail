import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { DraftInvitee } from "../../lib/eventDraft";
import { InviteesField } from "./InviteesField";

it("shows special roles and preserves them while editing other guests", () => {
  const invitees: DraftInvitee[] = [
    { email: "chair@example.com", name: "Leader", role: "chair" },
    { email: "info@example.com", name: "Observer", role: "none" },
    { email: "guest@example.com", name: "Guest", role: "required" },
  ];
  const onChange = vi.fn();
  render(<InviteesField invitees={invitees} available onChange={onChange} suggest={async () => []} />);
  expect(screen.getByText("Chair")).toBeTruthy();
  expect(screen.getByText("For information")).toBeTruthy();
  expect(screen.getByRole("combobox", { name: "Add Invitee" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Remove Guest" }));
  expect(onChange).toHaveBeenCalledWith(invitees.slice(0, 2));
});
