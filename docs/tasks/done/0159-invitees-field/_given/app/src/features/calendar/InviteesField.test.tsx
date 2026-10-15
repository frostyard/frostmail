// CONTRACT TEST for task card T-0159 (docs/tasks). Do not edit.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { DraftInvitee } from "../../lib/eventDraft";
import type { Address } from "../../rpc/gen/api";
import { InviteesField } from "./InviteesField";

const carmen: DraftInvitee = {
  email: "carmen@vertex.test",
  name: "Carmen Lindqvist",
  role: "required",
  answer: "accepted",
};
const ines: DraftInvitee = { email: "ines@vertex.test", name: "", role: "optional", answer: "tentative" };

function Harness(props: {
  initial?: DraftInvitee[];
  available?: boolean;
  disabled?: boolean;
  suggest?: (prefix: string) => Promise<Address[]>;
  onChange?: (next: DraftInvitee[]) => void;
}) {
  const [invitees, setInvitees] = useState(props.initial ?? []);
  return (
    <InviteesField
      invitees={invitees}
      onChange={(next) => {
        props.onChange?.(next);
        setInvitees(next);
      }}
      suggest={props.suggest ?? (async () => [])}
      available={props.available ?? true}
      disabled={props.disabled}
    />
  );
}

const rows = () => within(screen.getByRole("list", { name: "Invitees" })).getAllByRole("listitem");
const input = () => screen.getByRole("combobox", { name: "Add Invitee" }) as HTMLInputElement;

describe("InviteesField", () => {
  it("lists the invitees with their answers and optional marks", () => {
    render(<Harness initial={[carmen, ines]} />);
    const [first, second] = rows();
    if (!first || !second) throw new Error("two rows");
    expect(within(first).getByText("Carmen Lindqvist").getAttribute("title")).toBe(
      "Carmen Lindqvist <carmen@vertex.test>",
    );
    expect(within(first).getByLabelText("Accepted")).toBeTruthy();
    expect(within(first).getByRole("button", { name: "Optional" }).getAttribute("aria-pressed")).toBe("false");
    expect(within(second).getByText("ines@vertex.test")).toBeTruthy();
    expect(within(second).getByLabelText("Maybe")).toBeTruthy();
    expect(within(second).getByRole("button", { name: "Optional" }).getAttribute("aria-pressed")).toBe("true");
    expect(within(second).getByRole("button", { name: "Remove ines@vertex.test" })).toBeTruthy();
  });

  it("toggles the optional mark and removes an invitee", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness initial={[carmen, ines]} onChange={onChange} />);
    await user.click(within(rows()[0] as HTMLElement).getByRole("button", { name: "Optional" }));
    expect(onChange).toHaveBeenLastCalledWith([{ ...carmen, role: "optional" }, ines]);
    await user.click(screen.getByRole("button", { name: "Remove Carmen Lindqvist" }));
    expect(onChange).toHaveBeenLastCalledWith([ines]);
  });

  it("adds typed and suggested addresses, once each", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const suggest = vi.fn(async (prefix: string) =>
      [
        { name: "Erin Ito", address: "erin@acme.test" },
        { name: "Carmen Lindqvist", address: "carmen@vertex.test" },
      ].filter((a) => a.name.toLowerCase().includes(prefix.toLowerCase()) || a.address.startsWith(prefix)),
    );
    render(<Harness initial={[carmen]} onChange={onChange} suggest={suggest} />);
    await user.click(input());
    await user.type(input(), "dana@acme.test{Enter}");
    expect(onChange).toHaveBeenLastCalledWith([carmen, { email: "dana@acme.test", name: "", role: "required" }]);
    await user.type(input(), "CARMEN@vertex.test{Enter}");
    expect(onChange).toHaveBeenCalledTimes(1);

    await user.type(input(), "e");
    const list = await screen.findByRole("listbox", { name: "Add Invitee suggestions" });
    expect(within(list).queryByText("Carmen Lindqvist")).toBeNull();
    await user.click(within(list).getByText("Erin Ito"));
    expect(onChange).toHaveBeenLastCalledWith([
      carmen,
      { email: "dana@acme.test", name: "", role: "required" },
      { email: "erin@acme.test", name: "Erin Ito", role: "required" },
    ]);
  });

  it("marks an address that is not one", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(input());
    await user.type(input(), "erin{Enter}");
    const name = within(rows()[0] as HTMLElement).getByText("erin");
    expect(name.getAttribute("title")).toBe("erin is not a valid address");
    expect(name.className).toContain("text-flag-1");
  });

  it("says when the calendar does not invite, and shows its invitees only", () => {
    render(<Harness initial={[carmen]} available={false} />);
    expect(screen.queryByRole("combobox", { name: "Add Invitee" })).toBeNull();
    expect(screen.getByText("Invitations aren't available in this calendar.")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Remove Carmen Lindqvist" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("changes nothing when disabled", () => {
    render(<Harness initial={[carmen]} disabled />);
    expect(screen.queryByRole("combobox", { name: "Add Invitee" })).toBeNull();
    expect((screen.getByRole("button", { name: "Optional" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
