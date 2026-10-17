// The account form's error shows where it can be seen: below the services,
// a short window would hide it.
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AccountForm, type AccountFormProps } from "./AccountForm";

const props: AccountFormProps = {
  mode: "edit",
  value: {
    kind: "gmail",
    email: "ann@gmail.test",
    displayName: "",
    auth: "oauth2",
    imap: { host: "imap.gmail.com", port: 993, tls: "tls", username: "ann@gmail.test" },
    smtp: { host: "smtp.gmail.com", port: 465, tls: "tls", username: "ann@gmail.test" },
    readOnly: false,
    notify: true,
    password: "",
  },
  onChange: vi.fn(),
  discovery: { kind: "idle" },
  onDiscover: vi.fn(),
  signedIn: false,
  onSignIn: vi.fn(),
  busy: false,
  error: null,
  onSubmit: vi.fn(),
  onCancel: vi.fn(),
};

afterEach(() => vi.restoreAllMocks());

describe("the account form's error", () => {
  it("scrolls into view when it appears and when it changes", () => {
    const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
    const { rerender } = render(<AccountForm {...props} />);
    expect(scroll).not.toHaveBeenCalled();
    rerender(<AccountForm {...props} error="Set up your Google client under Sign-In first." />);
    expect(screen.getByRole("alert").textContent).toBe("Set up your Google client under Sign-In first.");
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(scroll.mock.contexts[0]).toBe(screen.getByRole("alert"));
    rerender(<AccountForm {...props} error="Could not turn on Calendars: offline" />);
    expect(scroll).toHaveBeenCalledTimes(2);
  });
});
