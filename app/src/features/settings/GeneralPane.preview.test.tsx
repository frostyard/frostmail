// P-211: the General pane's List preview.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Settings } from "../../rpc/gen/api";
import { GeneralPane } from "./GeneralPane";

const settings: Settings = { undoDelay: 10, notifyScope: "inbox", flagNames: ["", "", "", "", "", "", ""] };

describe("GeneralPane: List preview", () => {
  it("offers No Preview to 5 lines, shows 2 until chosen, and sends the choice", () => {
    const onChange = vi.fn();
    render(<GeneralPane settings={settings} onChange={onChange} />);
    const select = screen.getByLabelText("List preview:") as HTMLSelectElement;
    expect(select.value).toBe("2");
    expect(Array.from(select.options).map((o) => [o.value, o.textContent])).toEqual([
      ["0", "No Preview"],
      ["1", "1 Line"],
      ["2", "2 Lines"],
      ["3", "3 Lines"],
      ["4", "4 Lines"],
      ["5", "5 Lines"],
    ]);
    fireEvent.change(select, { target: { value: "0" } });
    expect(onChange).toHaveBeenLastCalledWith({ previewLines: 0 });
  });

  it("shows the stored choice, and waits for the settings", () => {
    const { unmount } = render(<GeneralPane settings={{ ...settings, previewLines: 4 }} onChange={vi.fn()} />);
    expect((screen.getByLabelText("List preview:") as HTMLSelectElement).value).toBe("4");
    unmount();
    render(<GeneralPane settings={null} onChange={vi.fn()} />);
    expect((screen.getByLabelText("List preview:") as HTMLSelectElement).disabled).toBe(true);
  });
});
