// CONTRACT TEST for task card T-0126 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Address } from "../../rpc/gen/api";
import { RedirectSheet, type RedirectSheetProps } from "./RedirectSheet";

function sheet(over: Partial<RedirectSheetProps> = {}) {
  const props: RedirectSheetProps = {
    subject: "Quarterly numbers",
    suggest: vi.fn(() => Promise.resolve<Address[]>([])),
    onRedirect: vi.fn(() => Promise.resolve()),
    onCancel: vi.fn(),
    ...over,
  };
  render(<RedirectSheet {...props} />);
  return props;
}

const dialog = () => screen.getByRole("dialog", { name: "Redirect" });
const to = () => within(dialog()).getByRole("combobox", { name: "To" }) as HTMLInputElement;
const redirectButton = () => within(dialog()).getByRole("button", { name: "Redirect" }) as HTMLButtonElement;

function type(text: string) {
  fireEvent.change(to(), { target: { value: text } });
  fireEvent.keyDown(to(), { key: "Enter" });
}

describe("RedirectSheet", () => {
  it("shows the subject and starts in To, with Redirect disabled", () => {
    sheet();
    expect(dialog().className).toContain("w-[420px]");
    expect(dialog().textContent).toContain("Quarterly numbers");
    expect(document.activeElement).toBe(to());
    expect(redirectButton().disabled).toBe(true);
  });

  it("says (no subject) for a blank subject", () => {
    sheet({ subject: "  " });
    expect(dialog().textContent).toContain("(no subject)");
  });

  it("redirects to the addresses once they are all valid", async () => {
    const props = sheet();
    type("bob");
    expect(redirectButton().disabled).toBe(true);
    fireEvent.click(within(dialog()).getByRole("button", { name: "bob" }));
    fireEvent.keyDown(to(), { key: "Backspace" });
    expect(within(dialog()).queryByRole("button", { name: "bob" })).toBeNull();
    type("Bob <bob@x.test>");
    type("carol@y.test");
    await waitFor(() => expect(redirectButton().disabled).toBe(false));
    fireEvent.click(redirectButton());
    expect(props.onRedirect).toHaveBeenCalledWith([
      { name: "Bob", address: "bob@x.test" },
      { name: "", address: "carol@y.test" },
    ]);
  });

  it("shows why it failed and stays open", async () => {
    let fail: (err: Error) => void = () => {};
    const props = sheet({ onRedirect: vi.fn(() => new Promise<void>((_, reject) => (fail = reject))) });
    type("bob@x.test");
    fireEvent.click(redirectButton());
    expect(redirectButton().disabled).toBe(true);
    await act(async () => fail(new Error("account 1 is read-only")));
    expect(within(dialog()).getByRole("alert").textContent).toBe("account 1 is read-only");
    expect(redirectButton().disabled).toBe(false);
    expect(props.onCancel).not.toHaveBeenCalled();
  });

  it("cancels with Cancel or Escape, keeping Escape from the window", () => {
    const onKey = vi.fn();
    window.addEventListener("keydown", onKey);
    const props = sheet();
    fireEvent.keyDown(to(), { key: "Escape" });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    window.removeEventListener("keydown", onKey);
    expect(props.onCancel).toHaveBeenCalledTimes(2);
    expect(onKey).not.toHaveBeenCalled();
  });
});
