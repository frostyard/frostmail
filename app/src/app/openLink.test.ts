import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Lodge, setLodge } from "../lodge/lodge";
import { openLink } from "./ReaderContainer";

// Shared with maild's sanitizer; adapted from frostui v0.4.0's mailtoCases.
const cases: { name: string; input: string; want: string }[] = JSON.parse(
  readFileSync("../internal/render/testdata/mailto.json", "utf8"),
);

afterEach(() => {
  setLodge(null);
  vi.restoreAllMocks();
});

describe.each(["lodge", "browser"])("mailto links opened through %s", (through) => {
  function opener() {
    const browser = vi.spyOn(window, "open").mockReturnValue(null);
    const shell = vi.fn();
    setLodge(through === "lodge" ? ({ open: shell } as unknown as Lodge) : null);
    return { opened: through === "lodge" ? shell : browser, other: through === "lodge" ? browser : shell };
  }

  it.each(cases)("$name", ({ input, want }) => {
    const { opened, other } = opener();
    openLink(input);
    if (want) {
      expect(opened).toHaveBeenCalledExactlyOnceWith(
        ...(through === "lodge" ? [want] : [want, "_blank", "noopener,noreferrer"]),
      );
    } else {
      expect(opened).not.toHaveBeenCalled();
    }
    expect(other).not.toHaveBeenCalled();
  });

  it.each([
    " mailto:a@e.test?attach=x",
    "\u0000mailto:a@e.test?attach=x",
    "\u001fmailto:a@e.test?attach=x",
    "mail\nto:a@e.test?attach=x",
    "MAIL\tTO:a@e.test?attach=x",
    "mailto:a@e.test?body=a b",
    "mailto:a@e.test?body=a\u007fb",
  ])("refuses whitespace and controls in %j", (input) => {
    const { opened, other } = opener();
    openLink(input);
    expect(opened).not.toHaveBeenCalled();
    expect(other).not.toHaveBeenCalled();
  });

  it.each(["https://lodge.example/mail/message/a%40b", "http://example.test/?subject=x&attach=y#fragment"])(
    "leaves %s unchanged",
    (url) => {
      const { opened } = opener();
      openLink(url);
      expect(opened.mock.calls[0]?.[0]).toBe(url);
    },
  );
});
