// CONTRACT TEST for task card T-0035 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ScopeBar, SearchField } from "./SearchField";

function Harness(props: { onSearch: (t: string) => void; onClear: () => void; initial?: string }) {
  const [value, setValue] = useState(props.initial ?? "");
  return (
    <SearchField
      value={value}
      onChange={setValue}
      onSearch={props.onSearch}
      onClear={() => {
        setValue("");
        props.onClear();
      }}
    />
  );
}

describe("SearchField", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("is a labeled search box with a placeholder", () => {
    render(<Harness onSearch={() => {}} onClear={() => {}} />);
    const box = screen.getByRole("searchbox", { name: "Search" });
    expect(box.getAttribute("placeholder")).toBe("Search");
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
  });

  it("reports edits at once and searches 250 ms after the last one", () => {
    const onSearch = vi.fn();
    render(<Harness onSearch={onSearch} onClear={() => {}} />);
    const box = screen.getByRole("searchbox", { name: "Search" });
    fireEvent.change(box, { target: { value: "inv" } });
    act(() => vi.advanceTimersByTime(200));
    fireEvent.change(box, { target: { value: " invoice " } });
    expect((box as HTMLInputElement).value).toBe(" invoice ");
    act(() => vi.advanceTimersByTime(249));
    expect(onSearch).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(onSearch).toHaveBeenCalledTimes(1);
    expect(onSearch).toHaveBeenCalledWith("invoice");
  });

  it("searches at once on Enter, without a second search later", () => {
    const onSearch = vi.fn();
    render(<Harness onSearch={onSearch} onClear={() => {}} />);
    const box = screen.getByRole("searchbox", { name: "Search" });
    fireEvent.change(box, { target: { value: "lunch" } });
    fireEvent.keyDown(box, { key: "Enter" });
    expect(onSearch).toHaveBeenCalledWith("lunch");
    act(() => vi.advanceTimersByTime(1000));
    expect(onSearch).toHaveBeenCalledTimes(1);
  });

  it("clears on Escape and cancels the pending search", () => {
    const onSearch = vi.fn();
    const onClear = vi.fn();
    render(<Harness onSearch={onSearch} onClear={onClear} />);
    const box = screen.getByRole("searchbox", { name: "Search" });
    fireEvent.change(box, { target: { value: "draft" } });
    fireEvent.keyDown(box, { key: "Escape" });
    expect(onClear).toHaveBeenCalledTimes(1);
    act(() => vi.advanceTimersByTime(1000));
    expect(onSearch).not.toHaveBeenCalled();
    expect((box as HTMLInputElement).value).toBe("");
  });

  it("shows a clear button while there is text", () => {
    const onClear = vi.fn();
    const onSearch = vi.fn();
    render(<Harness onSearch={onSearch} onClear={onClear} initial="x" />);
    const clear = screen.getByRole("button", { name: "Clear search" });
    fireEvent.change(screen.getByRole("searchbox", { name: "Search" }), { target: { value: "xy" } });
    fireEvent.click(clear);
    expect(onClear).toHaveBeenCalledTimes(1);
    act(() => vi.advanceTimersByTime(1000));
    expect(onSearch).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
  });

  it("forwards inputRef to the input", () => {
    const ref = { current: null as HTMLInputElement | null };
    render(<SearchField value="" onChange={() => {}} onSearch={() => {}} onClear={() => {}} inputRef={ref} />);
    expect(ref.current).toBe(screen.getByRole("searchbox", { name: "Search" }));
  });
});

describe("ScopeBar", () => {
  const scopes = [
    { key: "all", label: "All Mailboxes" },
    { key: "mailbox:3", label: "Inbox" },
  ];

  it("labels the bar and marks the selected scope", () => {
    render(<ScopeBar scopes={scopes} selected="mailbox:3" onSelect={() => {}} />);
    expect(screen.getByText("Search:")).toBeTruthy();
    expect(screen.getByRole("button", { name: "All Mailboxes" }).getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByRole("button", { name: "Inbox" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("reports the clicked scope", () => {
    const onSelect = vi.fn();
    render(<ScopeBar scopes={scopes} selected="all" onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("button", { name: "Inbox" }));
    expect(onSelect).toHaveBeenCalledWith("mailbox:3");
  });
});
