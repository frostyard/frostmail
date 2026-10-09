// CONTRACT TEST for task card T-0063 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { PersonSummary } from "../../rpc/gen/api";
import { IndexHeader, PersonRow, type PersonRowProps } from "./PersonRow";

const ada: PersonSummary = {
  id: 7,
  displayName: "Ada Lovelace",
  organization: "Analytical Engines",
  email: "ada@example.com",
  hasPhoto: false,
  index: "L",
};

function row(over: Partial<PersonRowProps> = {}, person: Partial<PersonSummary> = {}) {
  const props: PersonRowProps = {
    person: { ...ada, ...person },
    selected: false,
    focused: false,
    onSelect: vi.fn(),
    ...over,
  };
  render(<PersonRow {...props} />);
  return { props, el: screen.getByRole("option") };
}

describe("PersonRow", () => {
  it("shows the avatar, the name and the organization in a 44px option", () => {
    const { el } = row();
    expect(el.getAttribute("data-person-id")).toBe("7");
    expect(el.getAttribute("tabindex")).toBe("-1");
    expect(el.className).toContain("h-[44px]");
    expect(screen.getByText("Ada Lovelace").className).toContain("font-semibold");
    expect(screen.getByText("Analytical Engines").className).toContain("text-secondary");
    expect(screen.getByText("AL")).toBeTruthy();
  });

  it("falls back to the email, and to the email for a nameless person", () => {
    row({}, { organization: "" });
    expect(screen.getByText("ada@example.com").className).toContain("text-secondary");
  });

  it("names a person without a name by their email", () => {
    row({}, { displayName: "", organization: "" });
    expect(screen.getAllByText("ada@example.com").length).toBeGreaterThan(0);
    expect(screen.getByText("A")).toBeTruthy();
  });

  it("shows the selection as the message list does", () => {
    const focused = row({ selected: true, focused: true }).el;
    expect(focused.getAttribute("aria-selected")).toBe("true");
    expect(focused.className).toContain("bg-accent");
    expect(focused.className).toContain("text-accent-contrast");
  });

  it("is gray when selected without focus, plain when not selected", () => {
    const { el } = row({ selected: true });
    expect(el.className).toContain("bg-selection-inactive");
    expect(el.className).not.toContain("bg-accent");
  });

  it("selects on click", () => {
    const { props, el } = row();
    fireEvent.click(el);
    expect(props.onSelect).toHaveBeenCalledWith(7);
  });
});

describe("IndexHeader", () => {
  it("is a 24px presentation header with the letter", () => {
    render(<IndexHeader letter="L" />);
    const h = screen.getByText("L");
    const header = h.closest("[role='presentation']");
    expect(header).not.toBeNull();
    expect(header?.className).toContain("h-[24px]");
    expect(header?.className).toContain("bg-sidebar");
    expect(header?.className).toContain("sticky");
  });
});
