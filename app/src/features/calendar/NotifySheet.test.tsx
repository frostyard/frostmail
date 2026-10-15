// CONTRACT TEST for task card T-0156 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { InviteNotice } from "../../rpc/gen/api";
import { NotifySheet, useNotify } from "./NotifySheet";

const buttons = () => screen.getAllByRole("button").map((b) => b.textContent);

describe("NotifySheet", () => {
  it("asks before updates go out", () => {
    const onSend = vi.fn();
    render(<NotifySheet notice={{ kind: "updates", count: 3 }} onSend={onSend} onCancel={() => {}} />);
    const dialog = screen.getByRole("dialog", { name: "Send Updates?" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.textContent).toContain("3 invitees will get it by email.");
    expect(buttons()).toEqual(["Cancel", "Send"]);
    expect(document.activeElement?.textContent).toBe("Send");
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it("names invitations and cancellations, and one invitee", () => {
    const { unmount } = render(
      <NotifySheet notice={{ kind: "invitations", count: 1 }} onSend={() => {}} onCancel={() => {}} />,
    );
    expect(screen.getByRole("dialog", { name: "Send Invitations?" }).textContent).toContain(
      "The invitee will get it by email.",
    );
    unmount();
    render(<NotifySheet notice={{ kind: "cancellations", count: 2 }} onSend={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole("dialog", { name: "Send Cancellations?" })).toBeTruthy();
  });

  it("cancels with Cancel and Escape, and keeps keys from the window", () => {
    const onCancel = vi.fn();
    const windowKeys = vi.fn();
    window.addEventListener("keydown", windowKeys);
    render(<NotifySheet notice={{ kind: "updates", count: 1 }} onSend={() => {}} onCancel={onCancel} />);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(2);
    fireEvent.keyDown(screen.getByRole("button", { name: "Send" }), { key: "z", ctrlKey: true });
    expect(windowKeys).not.toHaveBeenCalled();
    window.removeEventListener("keydown", windowKeys);
  });

  it("keeps Tab inside, and gives focus back when it closes", () => {
    function Host() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Open
          </button>
          {open && (
            <NotifySheet notice={{ kind: "updates", count: 2 }} onSend={() => {}} onCancel={() => setOpen(false)} />
          )}
        </>
      );
    }
    render(<Host />);
    const opener = screen.getByRole("button", { name: "Open" });
    opener.focus();
    fireEvent.click(opener);
    expect(document.activeElement?.textContent).toBe("Send");
    fireEvent.keyDown(document.activeElement as Element, { key: "Tab" });
    expect(document.activeElement?.textContent).toBe("Cancel");
    fireEvent.keyDown(document.activeElement as Element, { key: "Tab" });
    expect(document.activeElement?.textContent).toBe("Send");
    fireEvent.keyDown(document.activeElement as Element, { key: "Tab", shiftKey: true });
    expect(document.activeElement?.textContent).toBe("Cancel");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
  });
});

describe("useNotify", () => {
  function Asker({ notice }: { notice: InviteNotice }) {
    const { ask, sheet } = useNotify();
    const [answer, setAnswer] = useState("none");
    return (
      <>
        <button type="button" onClick={() => ask(notice).then((yes) => setAnswer(String(yes)))}>
          Change
        </button>
        <output>{answer}</output>
        {sheet}
      </>
    );
  }

  it("shows the sheet while it asks, and answers with the choice", async () => {
    render(<Asker notice={{ kind: "updates", count: 2 }} />);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Change" }));
    expect(screen.getByRole("dialog", { name: "Send Updates?" }).textContent).toContain("2 invitees");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Send" }));
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Change" }));
    await act(async () => {
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("false");
  });
});
