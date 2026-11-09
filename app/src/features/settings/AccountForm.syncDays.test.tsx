// The account form's "Keep mail from" select (docs/specs/settings-ui.md,
// AccountForm; ADR-0016, the sync window).
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AccountForm, type AccountFormProps, type AccountFormValue } from "./AccountForm";

const value: AccountFormValue = {
  kind: "imap",
  email: "ann@x.test",
  displayName: "Ann Example",
  auth: "password",
  imap: { host: "imap.x.test", port: 993, tls: "tls", username: "ann" },
  smtp: { host: "smtp.x.test", port: 465, tls: "tls", username: "ann" },
  readOnly: false,
  notify: true,
  syncDays: 0,
  password: "",
};

function form(over: Partial<AccountFormProps> = {}) {
  const props: AccountFormProps = {
    mode: "edit",
    value,
    onChange: vi.fn(),
    discovery: { kind: "idle" },
    onDiscover: vi.fn(),
    signedIn: true,
    onSignIn: vi.fn(),
    busy: false,
    error: null,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<AccountForm {...props} />);
  return props;
}

function syncDays(): HTMLSelectElement {
  return screen.getByLabelText("Keep mail from:") as HTMLSelectElement;
}

function chosen(select: HTMLSelectElement): string {
  return select.options[select.selectedIndex]?.text ?? "";
}

describe("the account form's sync window", () => {
  it("shows every window it offers, keeping every message first", () => {
    form();
    expect(Array.from(syncDays().options).map((o) => [o.value, o.text])).toEqual([
      ["0", "All mail"],
      ["30", "Last 30 days"],
      ["90", "Last 90 days"],
      ["183", "Last 6 months"],
      ["365", "Last year"],
      ["730", "Last 2 years"],
    ]);
    expect(chosen(syncDays())).toBe("All mail");
    expect(syncDays().className).toContain("border-separator");
    expect(syncDays().disabled).toBe(false);
    expect(
      screen.getByText(
        "A longer time downloads older mail from the server. A shorter time removes older mail from this computer only; it stays on the server.",
      ),
    ).toBeTruthy();
  });

  it("shows the stored window", () => {
    form({ value: { ...value, syncDays: 365 } });
    expect(chosen(syncDays())).toBe("Last year");
  });

  it("keeps a window set outside the list as an option of its own", () => {
    form({ value: { ...value, syncDays: 45 } });
    const select = syncDays();
    expect(Array.from(select.options).map((o) => [o.value, o.text])).toEqual([
      ["0", "All mail"],
      ["30", "Last 30 days"],
      ["90", "Last 90 days"],
      ["183", "Last 6 months"],
      ["365", "Last year"],
      ["730", "Last 2 years"],
      ["45", "Last 45 days"],
    ]);
    expect(chosen(select)).toBe("Last 45 days");
  });

  it("reports a chosen window", () => {
    const { onChange } = form();
    fireEvent.change(syncDays(), { target: { value: "365" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, syncDays: 365 });
  });

  it("waits while busy", () => {
    form({ busy: true });
    expect(syncDays().disabled).toBe(true);
  });

  it("is absent when adding an account, which keeps every message", () => {
    form({ mode: "add" });
    expect(screen.queryByLabelText("Keep mail from:")).toBeNull();
  });
});
