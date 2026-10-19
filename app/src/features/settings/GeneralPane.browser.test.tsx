// The General pane's notifications in a browser (docs/specs/settings-ui.md).
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { GeneralPane } from "./GeneralPane";

describe("GeneralPane in a browser", () => {
  it("asks to allow notifications until the browser answers", () => {
    const onAllow = vi.fn();
    const { rerender } = render(
      <GeneralPane settings={null} onChange={vi.fn()} browser={{ permission: "default", onAllow }} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Allow Notifications…" }));
    expect(onAllow).toHaveBeenCalledOnce();
    rerender(<GeneralPane settings={null} onChange={vi.fn()} browser={{ permission: "granted", onAllow }} />);
    expect(screen.getByText("Notifications are on.")).toBeTruthy();
    rerender(<GeneralPane settings={null} onChange={vi.fn()} browser={{ permission: "denied", onAllow }} />);
    expect(screen.getByText("Notifications are blocked in the browser's settings.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Allow Notifications…" })).toBeNull();
  });

  it("says nothing of a browser in the desktop app", () => {
    render(<GeneralPane settings={null} onChange={vi.fn()} />);
    expect(screen.queryByText("In this browser:")).toBeNull();
  });
});
