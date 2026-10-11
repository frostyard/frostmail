// CONTRACT TEST for task card T-0112 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Rule } from "../../rpc/gen/api";
import { RulesPane, type RulesPaneProps } from "./RulesPane";

const rule = (id: number, name: string, position: number, over: Partial<Rule> = {}): Rule => ({
  id,
  name,
  position,
  enabled: true,
  conditions: { match: "any", conditions: [{ field: "from", op: "contains", value: "x" }] },
  actions: [{ kind: "read" }],
  ...over,
});

const RULES = [
  rule(1, "Receipts", 0),
  rule(2, "Newsletters", 1, { enabled: false, problem: "action 1: mailbox 9 is gone" }),
  rule(3, "From Ann", 2),
];

function pane(over: Partial<RulesPaneProps> = {}) {
  const props: RulesPaneProps = {
    rules: RULES,
    onToggle: vi.fn(),
    onAdd: vi.fn(),
    onEdit: vi.fn(),
    onDuplicate: vi.fn(),
    onRemove: vi.fn(),
    onMove: vi.fn(),
    ...over,
  };
  const view = render(<RulesPane {...props} />);
  return { props, view };
}

const button = (name: string) => screen.getByRole("button", { name }) as HTMLButtonElement;

describe("RulesPane", () => {
  it("lists the rules in order, on or off, with their problems", () => {
    pane();
    const list = screen.getByRole("list", { name: "Rules" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows.map((r) => within(r).getByRole("button").textContent)).toEqual(["Receipts", "Newsletters", "From Ann"]);
    expect((screen.getByRole("checkbox", { name: "Enable Receipts" }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole("checkbox", { name: "Enable Newsletters" }) as HTMLInputElement).checked).toBe(false);
    const problem = screen.getByRole("img", { name: "action 1: mailbox 9 is gone" });
    expect(problem.getAttribute("title")).toBe("action 1: mailbox 9 is gone");
    expect(within(rows[1] as HTMLElement).queryByRole("img")).not.toBeNull();
    expect(within(rows[0] as HTMLElement).queryByRole("img")).toBeNull();
  });

  it("says when there are none", () => {
    pane({ rules: [] });
    expect(within(screen.getByRole("list", { name: "Rules" })).getByRole("listitem").textContent).toBe("No Rules");
  });

  it("acts on the selected rule", () => {
    const { props } = pane();
    for (const name of ["Edit", "Duplicate", "Remove", "Move Up", "Move Down"])
      expect(button(name).disabled).toBe(true);
    expect(button("Add Rule").disabled).toBe(false);
    fireEvent.click(button("Add Rule"));
    expect(props.onAdd).toHaveBeenCalled();

    fireEvent.click(button("Newsletters"));
    expect(button("Newsletters").getAttribute("aria-pressed")).toBe("true");
    expect(button("Receipts").getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(button("Edit"));
    expect(props.onEdit).toHaveBeenCalledWith(RULES[1]);
    fireEvent.click(button("Duplicate"));
    expect(props.onDuplicate).toHaveBeenCalledWith(RULES[1]);
    fireEvent.click(button("Remove"));
    expect(props.onRemove).toHaveBeenCalledWith(RULES[1]);
    fireEvent.click(button("Move Up"));
    expect(props.onMove).toHaveBeenLastCalledWith(RULES[1], 0);
    fireEvent.click(button("Move Down"));
    expect(props.onMove).toHaveBeenLastCalledWith(RULES[1], 2);
  });

  it("cannot move the first rule up or the last down", () => {
    pane();
    fireEvent.click(button("Receipts"));
    expect([button("Move Up").disabled, button("Move Down").disabled]).toEqual([true, false]);
    fireEvent.click(button("From Ann"));
    expect([button("Move Up").disabled, button("Move Down").disabled]).toEqual([false, true]);
  });

  it("edits on a double-click and turns rules on and off without selecting", () => {
    const { props } = pane();
    fireEvent.doubleClick(button("From Ann"));
    expect(props.onEdit).toHaveBeenCalledWith(RULES[2]);
    fireEvent.click(screen.getByRole("checkbox", { name: "Enable Newsletters" }));
    expect(props.onToggle).toHaveBeenCalledWith(RULES[1], true);
    expect(button("Newsletters").getAttribute("aria-pressed")).toBe("false");
  });

  it("forgets a selection that is gone, and shows maild's error", () => {
    const { props, view } = pane();
    fireEvent.click(button("Newsletters"));
    expect(button("Edit").disabled).toBe(false);
    view.rerender(<RulesPane {...props} rules={[RULES[0] as Rule, RULES[2] as Rule]} />);
    expect(button("Edit").disabled).toBe(true);
    expect(screen.queryByRole("alert")).toBeNull();
    view.rerender(<RulesPane {...props} error="rule 2 does not exist" />);
    expect(screen.getByRole("alert").textContent).toContain("rule 2 does not exist");
  });
});
