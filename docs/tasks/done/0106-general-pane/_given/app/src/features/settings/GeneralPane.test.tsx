// CONTRACT TEST for task card T-0106 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Settings } from "../../rpc/gen/api";
import { GeneralPane } from "./GeneralPane";

const settings: Settings = { undoDelay: 10, notifyScope: "inbox", flagNames: ["", "Soon", "", "", "", "", ""] };

const options = (el: HTMLSelectElement) => Array.from(el.options).map((o) => [o.value, o.textContent]);

describe("GeneralPane", () => {
  it("sets the notification scope", () => {
    const onChange = vi.fn();
    render(<GeneralPane settings={settings} onChange={onChange} />);
    const scope = screen.getByLabelText("New message notifications:") as HTMLSelectElement;
    expect(scope.value).toBe("inbox");
    expect(options(scope)).toEqual([
      ["inbox", "Inbox Only"],
      ["vips", "VIPs"],
      ["contacts", "Contacts"],
      ["all", "All Mailboxes"],
    ]);
    fireEvent.change(scope, { target: { value: "vips" } });
    expect(onChange).toHaveBeenLastCalledWith({ notifyScope: "vips" });
  });

  it("sets the undo send delay", () => {
    const onChange = vi.fn();
    render(<GeneralPane settings={settings} onChange={onChange} />);
    const delay = screen.getByLabelText("Undo send delay:") as HTMLSelectElement;
    expect(delay.value).toBe("10");
    expect(options(delay)).toEqual([
      ["0", "Off"],
      ["10", "10 Seconds"],
      ["20", "20 Seconds"],
      ["30", "30 Seconds"],
    ]);
    fireEvent.change(delay, { target: { value: "0" } });
    expect(onChange).toHaveBeenLastCalledWith({ undoDelay: 0 });
  });

  it("names the flags when a field is left or Enter is pressed", () => {
    const onChange = vi.fn();
    render(<GeneralPane settings={settings} onChange={onChange} />);
    const red = screen.getByLabelText("Flag 1 name") as HTMLInputElement;
    const orange = screen.getByLabelText("Flag 2 name") as HTMLInputElement;
    expect(red.placeholder).toBe("Red");
    expect(red.value).toBe("");
    expect(red.maxLength).toBe(40);
    expect(orange.value).toBe("Soon");
    expect(screen.getByLabelText("Flag 7 name").getAttribute("placeholder")).toBe("Gray");

    fireEvent.blur(red);
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.change(red, { target: { value: " Urgent " } });
    fireEvent.blur(red);
    expect(onChange).toHaveBeenLastCalledWith({ flagNames: ["Urgent", "Soon", "", "", "", "", ""] });
    fireEvent.change(orange, { target: { value: "Later" } });
    fireEvent.keyDown(orange, { key: "Enter" });
    expect(onChange).toHaveBeenLastCalledWith({ flagNames: ["Urgent", "Later", "", "", "", "", ""] });
  });

  it("shows maild's error, and waits for the settings", () => {
    const { unmount } = render(<GeneralPane settings={settings} error="undoDelay must be 0" onChange={vi.fn()} />);
    expect(screen.getByRole("alert").textContent).toContain("undoDelay must be 0");
    unmount();
    render(<GeneralPane settings={null} onChange={vi.fn()} />);
    expect((screen.getByLabelText("Undo send delay:") as HTMLSelectElement).disabled).toBe(true);
    expect((screen.getByLabelText("Flag 1 name") as HTMLInputElement).disabled).toBe(true);
  });
});
