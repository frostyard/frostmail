// The reader's find bar (ADR-0052; docs/specs/ui.md, Find in the message):
// its count, its moves, its keys and the marks it leaves behind.
import { fireEvent, render, screen } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { FindBar } from "./FindBar";
import { clearMarks, markRanges } from "./findInMessage";

// The marks are the CSS Custom Highlight API's, which this environment has
// not: the bar's part in them is what these tests see.
vi.mock("./findInMessage", async (original) => ({
  ...(await original<typeof import("./findInMessage")>()),
  markRanges: vi.fn(),
  clearMarks: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

/** Harness gives the bar the scroller it searches, as the reader lays out. */
function Harness({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  const [scroller, setScroller] = useState<HTMLElement | null>(null);
  return (
    <div>
      <section ref={(el) => setScroller(el)}>{children}</section>
      {scroller !== null && <FindBar scroller={scroller} onClose={onClose} />}
    </div>
  );
}

async function setup(content?: ReactNode) {
  const onClose = vi.fn();
  const view = render(
    <Harness onClose={onClose}>
      {content ?? (
        <>
          <p>red and red</p>
          <p>blue</p>
        </>
      )}
    </Harness>,
  );
  const field = (await screen.findByRole("searchbox", { name: "Find in Message" })) as HTMLInputElement;
  const type = (text: string) => fireEvent.change(field, { target: { value: text } });
  return { onClose, field, type, unmount: view.unmount };
}

describe("the find bar", () => {
  it("counts the matches and moves through them, wrapping both ways", async () => {
    const { field, type } = await setup();
    // Nothing while the field is empty.
    expect(screen.queryByText("No Matches")).toBeNull();
    type("red");
    expect(screen.getByText("1 of 2")).toBeTruthy();
    fireEvent.keyDown(field, { key: "Enter" });
    expect(screen.getByText("2 of 2")).toBeTruthy();
    fireEvent.keyDown(field, { key: "Enter" });
    expect(screen.getByText("1 of 2")).toBeTruthy();
    fireEvent.keyDown(field, { key: "Enter", shiftKey: true });
    expect(screen.getByText("2 of 2")).toBeTruthy();
  });

  it("says No Matches, and disables the moves, when nothing matches", async () => {
    const { field, type } = await setup();
    type("zzz");
    expect(screen.getByText("No Matches")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Previous Match" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Next Match" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.keyDown(field, { key: "Enter" });
    expect(screen.getByText("No Matches")).toBeTruthy();
  });

  it("enables the moves once the text finds a match", async () => {
    const { type } = await setup();
    type("red");
    expect((screen.getByRole("button", { name: "Previous Match" }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole("button", { name: "Next Match" }) as HTMLButtonElement).disabled).toBe(false);
    // The buttons move as Enter and Shift+Enter do.
    fireEvent.click(screen.getByRole("button", { name: "Next Match" }));
    expect(screen.getByText("2 of 2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Previous Match" }));
    expect(screen.getByText("1 of 2")).toBeTruthy();
  });

  it("closes on Escape and on Done", async () => {
    const { field, onClose } = await setup();
    fireEvent.keyDown(field, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("takes Ctrl+F in its field back, and selects the text it holds", async () => {
    const { field, type } = await setup();
    type("red");
    field.setSelectionRange(1, 2);
    // The window's keys leave plain Ctrl+F to a text field, so the bar takes
    // it: the field comes back with its text selected, ready for a new query.
    expect(fireEvent.keyDown(field, { key: "f", ctrlKey: true })).toBe(false);
    expect(field.selectionStart).toBe(0);
    expect(field.selectionEnd).toBe(3);
  });

  it("keeps Enter's own meaning on its buttons", async () => {
    const { onClose, type } = await setup(
      <>
        <p>red red</p>
        <p>red</p>
      </>,
    );
    type("red");
    expect(screen.getByText("1 of 3")).toBeTruthy();
    const previous = screen.getByRole("button", { name: "Previous Match" });
    // Enter on a button is the button's own: it never moves the match on.
    expect(fireEvent.keyDown(previous, { key: "Enter" })).toBe(true);
    expect(screen.queryByText("2 of 3")).toBeNull();
    const done = screen.getByRole("button", { name: "Done" });
    expect(fireEvent.keyDown(done, { key: "Enter" })).toBe(true);
    expect(screen.queryByText("2 of 3")).toBeNull();
    fireEvent.click(done);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("leaves an input method's Enter to the input method", async () => {
    const { field, type } = await setup();
    type("red");
    expect(screen.getByText("1 of 2")).toBeTruthy();
    const onWindow = vi.fn();
    window.addEventListener("keydown", onWindow);
    try {
      // The Enter that confirms a composition: by isComposing, and by
      // keyCode 229 where an engine does not set it.
      expect(fireEvent.keyDown(field, { key: "Enter", isComposing: true })).toBe(true);
      expect(fireEvent.keyDown(field, { key: "Enter", keyCode: 229 })).toBe(true);
    } finally {
      window.removeEventListener("keydown", onWindow);
    }
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(onWindow).not.toHaveBeenCalled();
  });

  it("takes its marks away when it goes", async () => {
    const { type, unmount } = await setup();
    type("red");
    expect(vi.mocked(markRanges)).toHaveBeenCalled();
    unmount();
    const calls = vi.mocked(clearMarks).mock.calls;
    expect(calls).toHaveLength(1);
    const windows = [...(calls[0]?.[0] ?? [])];
    expect(windows).toHaveLength(1);
    expect(windows[0]).toBe(document.defaultView);
  });
});
