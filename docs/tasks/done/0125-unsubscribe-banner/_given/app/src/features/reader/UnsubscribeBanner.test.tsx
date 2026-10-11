// CONTRACT TEST for task card T-0125 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Unsubscribe } from "../../rpc/gen/api";
import { UnsubscribeBanner, type UnsubscribeBannerProps } from "./UnsubscribeBanner";

const info: Unsubscribe = {
  methods: ["oneclick", "mail", "web"],
  list: "Weekly News",
  host: "list.example",
  address: "leave@list.example",
  url: "https://pages.example/u/1",
  done: false,
};

function banner(over: Partial<UnsubscribeBannerProps> = {}) {
  const props: UnsubscribeBannerProps = {
    info,
    method: "oneclick",
    busy: false,
    failed: false,
    onUnsubscribe: vi.fn(),
    ...over,
  };
  const view = render(<UnsubscribeBanner {...props} />);
  return { props, ...view };
}

const unsubscribeButton = () =>
  within(screen.getByRole("status")).getByRole("button", { name: "Unsubscribe" }) as HTMLButtonElement;

describe("UnsubscribeBanner", () => {
  it("names the list and offers Unsubscribe", () => {
    banner();
    const status = screen.getByRole("status");
    expect(status.className).toContain("bg-banner");
    expect(status.textContent).toContain("This message is from the mailing list Weekly News.");
    expect(unsubscribeButton().disabled).toBe(false);
  });

  it("asks first, saying what the first method does", () => {
    const sayings: [UnsubscribeBannerProps["method"], string][] = [
      ["oneclick", "Frostmail will ask list.example to take you off the list."],
      ["mail", "Frostmail will send a message to leave@list.example asking to take you off the list."],
      ["web", "The list's page on pages.example will open in your browser."],
    ];
    for (const [method, text] of sayings) {
      const { props, unmount } = banner({ method });
      fireEvent.click(unsubscribeButton());
      const dialog = screen.getByRole("dialog", { name: "Unsubscribe from Weekly News?" });
      expect(dialog.textContent).toContain(text);
      expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Unsubscribe" }));
      expect(props.onUnsubscribe).not.toHaveBeenCalled();
      unmount();
    }
  });

  it("unsubscribes on confirming, and not on Cancel or Escape", () => {
    const { props } = banner();
    fireEvent.click(unsubscribeButton());
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(unsubscribeButton());
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(props.onUnsubscribe).not.toHaveBeenCalled();
    fireEvent.click(unsubscribeButton());
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Unsubscribe" }));
    expect(props.onUnsubscribe).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps Escape in the dialog from reaching the window", () => {
    const onKey = vi.fn();
    window.addEventListener("keydown", onKey);
    banner();
    fireEvent.click(unsubscribeButton());
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    window.removeEventListener("keydown", onKey);
    expect(onKey).not.toHaveBeenCalled();
  });

  it("is disabled without a method and while busy, and says when it failed", () => {
    const { rerender, props } = banner({ method: undefined });
    expect(unsubscribeButton().disabled).toBe(true);
    rerender(<UnsubscribeBanner {...props} method="oneclick" busy />);
    const busy = within(screen.getByRole("status")).getByRole("button", {
      name: "Unsubscribing…",
    }) as HTMLButtonElement;
    expect(busy.disabled).toBe(true);
    rerender(<UnsubscribeBanner {...props} method="oneclick" failed />);
    expect(screen.getByRole("status").textContent).toContain(
      "This message is from the mailing list Weekly News. Couldn't unsubscribe.",
    );
  });

  it("says the list was left, without a button", () => {
    banner({ info: { ...info, done: true } });
    expect(screen.getByRole("status").textContent).toBe("You unsubscribed from Weekly News.");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("is absent for a message from no list", () => {
    const { container } = banner({ info: { methods: [], list: "Ann", done: false }, method: undefined });
    expect(container.textContent).toBe("");
  });
});
