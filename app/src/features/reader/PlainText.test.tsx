// CONTRACT TEST for task card T-0028 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { PlainText, parsePlainText } from "./PlainText";

describe("parsePlainText", () => {
  it("groups lines by quote level and strips markers", () => {
    const text = "Sounds good.\r\n\r\nOn Tue, Ann wrote:\r\n> First\r\n>> Older\r\n> > Also older\r\n>Back\r\nMe again";
    expect(parsePlainText(text)).toEqual([
      { kind: "text", quote: 0, lines: ["Sounds good.", "", "On Tue, Ann wrote:"] },
      { kind: "text", quote: 1, lines: ["First"] },
      { kind: "text", quote: 2, lines: ["Older", "Also older"] },
      { kind: "text", quote: 1, lines: ["Back"] },
      { kind: "text", quote: 0, lines: ["Me again"] },
    ]);
  });

  it("splits off the signature at the first unquoted '-- ' line", () => {
    const text = "Hi\n> -- \n> quoted sig\nBye\n-- \nAnn\n-- \nmore";
    expect(parsePlainText(text)).toEqual([
      { kind: "text", quote: 0, lines: ["Hi"] },
      { kind: "text", quote: 1, lines: ["-- ", "quoted sig"] },
      { kind: "text", quote: 0, lines: ["Bye"] },
      { kind: "signature", lines: ["Ann", "-- ", "more"] },
    ]);
  });

  it("trims empty lines at both ends and keeps inner ones", () => {
    expect(parsePlainText("\n\nA\n\nB\n\n\n")).toEqual([{ kind: "text", quote: 0, lines: ["A", "", "B"] }]);
    expect(parsePlainText("")).toEqual([]);
    expect(parsePlainText("\n \n")).toEqual([]);
  });

  it("treats '--' without the space as text", () => {
    expect(parsePlainText("A\n--\nB")).toEqual([{ kind: "text", quote: 0, lines: ["A", "--", "B"] }]);
  });
});

describe("PlainText", () => {
  it("renders quote blocks with their level and the signature apart", () => {
    const { container } = render(<PlainText text={"Hi\n> quoted\n>> deeper\n-- \nAnn"} onOpenLink={() => {}} />);
    const levels = [...container.querySelectorAll("[data-quote-level]")].map((e) => e.getAttribute("data-quote-level"));
    expect(levels).toEqual(["0", "1", "2"]);
    const sig = container.querySelector("[data-signature]");
    expect(sig?.textContent).toBe("Ann");
    expect(sig?.className).toContain("text-secondary");
    expect(container.querySelector('[data-quote-level="1"]')?.className).toContain("border-quote-1");
    expect(container.querySelector('[data-quote-level="2"]')?.className).toContain("border-quote-2");
    expect(screen.queryByText("> quoted")).toBeNull();
  });

  it("cycles the quote colors after level 3", () => {
    const { container } = render(<PlainText text={"> > > > four"} onOpenLink={() => {}} />);
    expect(container.querySelector('[data-quote-level="4"]')?.className).toContain("border-quote-1");
  });

  it("collapses quote blocks longer than 4 lines", () => {
    const text = ["Reply", "> one", "> two", "> three", "> four", "> five"].join("\n");
    const { container } = render(<PlainText text={text} onOpenLink={() => {}} />);
    const block = container.querySelector('[data-quote-level="1"]');
    expect(block?.textContent).toContain("one");
    expect(block?.textContent).toContain("two");
    expect(block?.textContent).not.toContain("three");
    const more = screen.getByRole("button", { name: "See More" });
    expect(more.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(more);
    expect(block?.textContent).toContain("five");
    const less = screen.getByRole("button", { name: "See Less" });
    expect(less.getAttribute("aria-expanded")).toBe("true");
  });

  it("keeps short quote blocks open", () => {
    render(<PlainText text={"> one\n> two\n> three\n> four"} onOpenLink={() => {}} />);
    expect(screen.queryByRole("button", { name: "See More" })).toBeNull();
    expect(screen.getByText(/four/)).toBeTruthy();
  });

  it("links URLs and addresses and hands clicks to onOpenLink", () => {
    const onOpenLink = vi.fn();
    render(
      <PlainText
        text={"See https://example.test/a?b=1, or www.example.test/x.\nMail ann.smith+x@mail.example.test."}
        onOpenLink={onOpenLink}
      />,
    );
    const links = screen.getAllByRole("link");
    expect(links.map((a) => [a.textContent, a.getAttribute("href")])).toEqual([
      ["https://example.test/a?b=1", "https://example.test/a?b=1"],
      ["www.example.test/x", "https://www.example.test/x"],
      ["ann.smith+x@mail.example.test", "mailto:ann.smith+x@mail.example.test"],
    ]);
    const first = links[0];
    if (!first) throw new Error("no link");
    const ev = fireEvent.click(first);
    expect(ev).toBe(false);
    expect(onOpenLink).toHaveBeenCalledWith("https://example.test/a?b=1");
  });

  it("is selectable reading text", () => {
    const { container } = render(<PlainText text="A" onOpenLink={() => {}} />);
    const root = container.firstElementChild;
    expect(root?.className).toContain("select-text");
    expect(root?.className).toContain("whitespace-pre-wrap");
    expect(root?.className).toContain("text-reader-body");
  });
});
