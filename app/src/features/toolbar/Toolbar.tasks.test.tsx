// CONTRACT TEST for task card T-0084 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarProps } from "./Toolbar";

function toolbar(over: Partial<ToolbarProps> = {}) {
  const props: ToolbarProps = {
    mode: "tasks",
    tasks: { title: "Errands", showCompleted: false, paneWidth: 320 },
    sidebarWidth: 220,
    listWidth: 360,
    title: "",
    subtitle: "",
    syncing: false,
    selection: { count: 0, seen: true, flagColor: 0 },
    canArchive: false,
    moveTargets: [],
    maximized: false,
    search: <input aria-label="Search" />,
    onCommand: vi.fn(),
    ...over,
  };
  render(<Toolbar {...props} />);
  return props;
}

const button = (name: string) => screen.getByRole("button", { name });

describe("Toolbar in Tasks", () => {
  it("has the sidebar's, the list's and the pane's segments", () => {
    toolbar();
    const bar = screen.getByRole("toolbar", { name: "Toolbar" });
    const segments = [...bar.querySelectorAll<HTMLElement>(":scope > [data-segment]")];
    expect(segments.map((s) => s.getAttribute("data-segment"))).toEqual(["sidebar", "tasks", "pane"]);
    expect(segments[0]?.style.width).toBe("221px");
    expect(segments[2]?.style.width).toBe("321px");
    for (const s of segments) expect(s.hasAttribute("data-tauri-drag-region")).toBe(true);
  });

  it("shows New Task, the source's name and Show Completed", () => {
    const props = toolbar();
    expect(screen.getByText("Errands")).toBeTruthy();
    expect(button("New Task").getAttribute("title")).toBe("New Task (Ctrl+N)");
    fireEvent.click(button("New Task"));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "newTask" });
    const completed = button("Show Completed");
    expect(completed.textContent).toBe("Show Completed");
    expect(completed.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(completed);
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "toggleCompleted" });
  });

  it("shows Show Completed pressed, and hides it where it does not apply", () => {
    toolbar({ tasks: { title: "All Tasks", showCompleted: true, paneWidth: 320 } });
    expect(button("Show Completed").getAttribute("aria-pressed")).toBe("true");
  });

  it("hides Show Completed for Today and Flagged Mail", () => {
    toolbar({ tasks: { title: "Today", showCompleted: null, paneWidth: 320 } });
    expect(screen.queryByRole("button", { name: "Show Completed" })).toBeNull();
  });

  it("keeps the sidebar toggle, Settings and window controls, and nothing of Mail's or search", () => {
    toolbar();
    for (const name of ["Toggle Sidebar", "Settings", "Minimize", "Maximize", "Close"]) {
      expect(button(name)).toBeTruthy();
    }
    for (const name of ["Get Mail", "Reply", "Archive", "Delete", "To-Do Bar"]) {
      expect(screen.queryByRole("button", { name })).toBeNull();
    }
    expect(screen.queryByRole("textbox", { name: "Search" })).toBeNull();
  });
});

describe("Toolbar's To-Do Bar button", () => {
  it("shows in Mail before the search field, pressed while the bar shows", () => {
    const props = toolbar({ mode: "mail", tasks: undefined, todoBar: true });
    const todo = button("To-Do Bar");
    expect(todo.getAttribute("aria-pressed")).toBe("true");
    const search = screen.getByRole("textbox", { name: "Search" });
    expect(todo.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.click(todo);
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "toggleTodoBar" });
  });

  it("is not pressed while the bar is hidden, and absent outside Mail", () => {
    toolbar({ mode: "mail", tasks: undefined });
    expect(button("To-Do Bar").getAttribute("aria-pressed")).toBe("false");
  });

  it("is absent in People", () => {
    toolbar({ mode: "people", tasks: undefined, todoBar: true });
    expect(screen.queryByRole("button", { name: "To-Do Bar" })).toBeNull();
  });
});
