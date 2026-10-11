// CONTRACT TEST for task card T-0106 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar } from "./Toolbar";

describe("the toolbar's Flag menu", () => {
  it("names the colors as the user named the flags", () => {
    render(
      <Toolbar
        sidebarWidth={220}
        listWidth={360}
        title="Inbox"
        subtitle=""
        syncing={false}
        selection={{ count: 1, seen: true, flagColor: 1 }}
        canArchive
        moveTargets={[]}
        maximized={false}
        search={<input aria-label="Search" />}
        onCommand={vi.fn()}
        flagNames={["Urgent", "", "", "", "", "", "Someday"]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Flag" }));
    expect(screen.getAllByRole("menuitemcheckbox").map((m) => m.textContent)).toEqual([
      "Urgent",
      "Orange",
      "Yellow",
      "Green",
      "Blue",
      "Purple",
      "Someday",
    ]);
  });
});
