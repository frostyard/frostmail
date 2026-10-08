// CONTRACT TEST for task card T-0045 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ComposeToolbar, type ComposeToolbarProps, FormatBar } from "./ComposeToolbar";

function toolbar(over: Partial<ComposeToolbarProps> = {}) {
  const props: ComposeToolbarProps = {
    subject: "Lunch",
    canSend: true,
    formatBarShown: false,
    maximized: false,
    onCommand: vi.fn(),
    ...over,
  };
  render(<ComposeToolbar {...props} />);
  return props;
}

const button = (name: string) => screen.getByRole("button", { name });

describe("ComposeToolbar", () => {
  it("is a draggable toolbar titled by the subject", () => {
    toolbar();
    const bar = screen.getByRole("toolbar", { name: "Compose" });
    expect(bar.hasAttribute("data-tauri-drag-region")).toBe(true);
    expect(bar.className).toContain("h-[52px]");
    expect(screen.getByText("Lunch").hasAttribute("data-tauri-drag-region")).toBe(true);
  });

  it("titles a draft without a subject New Message", () => {
    toolbar({ subject: "   " });
    expect(screen.getByText("New Message")).toBeTruthy();
  });

  it("has its buttons in order with tooltips", () => {
    toolbar();
    const names = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(names).toEqual(["Send", "Attach Files", "Show Format Bar", "Delete Draft", "Minimize", "Maximize", "Close"]);
    expect(button("Send").getAttribute("title")).toBe("Send (Ctrl+Enter)");
    expect(button("Attach Files").getAttribute("title")).toBe("Attach Files (Ctrl+Shift+A)");
    expect(button("Show Format Bar").getAttribute("title")).toBe("Show Format Bar");
    expect(button("Delete Draft").getAttribute("title")).toBe("Delete Draft (Ctrl+Backspace)");
  });

  it("draws Send in the accent color and disables it when the draft cannot be sent", () => {
    toolbar({ canSend: false });
    const send = button("Send") as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    expect(send.className).toContain("text-accent");
    expect(send.className).not.toContain("text-secondary");
  });

  it("shows whether the format bar is shown", () => {
    toolbar({ formatBarShown: true });
    expect(button("Show Format Bar").getAttribute("aria-pressed")).toBe("true");
  });

  it("offers Restore while maximized", () => {
    toolbar({ maximized: true });
    expect(screen.queryByRole("button", { name: "Maximize" })).toBeNull();
    expect(button("Restore")).toBeTruthy();
  });

  it("sends each button's command", () => {
    const props = toolbar();
    const commands: [string, string][] = [
      ["Send", "send"],
      ["Attach Files", "attach"],
      ["Show Format Bar", "toggleFormatBar"],
      ["Delete Draft", "delete"],
      ["Minimize", "minimize"],
      ["Maximize", "toggleMaximize"],
      ["Close", "close"],
    ];
    for (const [name, command] of commands) {
      fireEvent.click(button(name));
      expect(props.onCommand).toHaveBeenLastCalledWith(command);
    }
    expect(props.onCommand).toHaveBeenCalledTimes(commands.length);
  });
});

describe("FormatBar", () => {
  const toggles: [string, string, string][] = [
    ["Bold", "bold", "Bold (Ctrl+B)"],
    ["Italic", "italic", "Italic (Ctrl+I)"],
    ["Underline", "underline", "Underline (Ctrl+U)"],
    ["Strikethrough", "strike", "Strikethrough"],
    ["Bulleted List", "bulletList", "Bulleted List"],
    ["Numbered List", "orderedList", "Numbered List"],
    ["Quote", "blockquote", "Quote"],
    ["Link", "link", "Link (Ctrl+K)"],
  ];

  it("is a toolbar of toggles in order, then Clear Formatting", () => {
    render(<FormatBar active={{}} onCommand={() => {}} />);
    expect(screen.getByRole("toolbar", { name: "Format" }).className).toContain("h-8");
    const names = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(names).toEqual([...toggles.map(([name]) => name), "Clear Formatting"]);
    for (const [name, , title] of toggles) {
      expect(button(name).getAttribute("title")).toBe(title);
    }
    expect(button("Clear Formatting").hasAttribute("aria-pressed")).toBe(false);
  });

  it("presses the toggles that are active", () => {
    render(<FormatBar active={{ bold: true, orderedList: true, link: false }} onCommand={() => {}} />);
    for (const [name, key] of toggles) {
      const want = key === "bold" || key === "orderedList" ? "true" : "false";
      expect(button(name).getAttribute("aria-pressed")).toBe(want);
    }
    expect(button("Bold").classList.contains("bg-selection-inactive")).toBe(true);
    expect(button("Bold").classList.contains("text-primary")).toBe(true);
    expect(button("Italic").classList.contains("bg-selection-inactive")).toBe(false);
    expect(button("Italic").classList.contains("text-primary")).toBe(false);
  });

  it("sends each control's command without taking focus from the editor", () => {
    const onCommand = vi.fn();
    render(<FormatBar active={{}} onCommand={onCommand} />);
    for (const [name, key] of [...toggles, ["Clear Formatting", "clear", ""] as const]) {
      const b = button(name);
      expect(fireEvent.mouseDown(b)).toBe(false);
      fireEvent.click(b);
      expect(onCommand).toHaveBeenLastCalledWith(key);
    }
  });

  it("splits the controls into three groups", () => {
    const { container } = render(<FormatBar active={{}} onCommand={() => {}} />);
    expect(container.querySelectorAll('[aria-hidden="true"].bg-separator')).toHaveLength(2);
  });
});
