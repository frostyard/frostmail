// CONTRACT TEST for task card T-0026 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import {
  avatarTone,
  displayName,
  formatAddressList,
  formatCount,
  formatHeaderDate,
  formatListDate,
  formatSize,
  initials,
} from "./format";

const addr = (name: string, address: string) => ({ name, address });

describe("formatListDate", () => {
  // Wednesday, October 7, 2026, 15:00 local time.
  const now = new Date(2026, 9, 7, 15, 0);
  const time = (d: Date) => d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" });

  it("shows the time for today, including later today", () => {
    const morning = new Date(2026, 9, 7, 9, 41);
    expect(formatListDate(morning, now, "en-US")).toBe(time(morning));
    expect(formatListDate(new Date(2026, 9, 7, 0, 0), now, "en-US")).toBe(time(new Date(2026, 9, 7, 0, 0)));
    expect(formatListDate(new Date(2026, 9, 7, 23, 59), now, "en-US")).toBe(time(new Date(2026, 9, 7, 23, 59)));
  });

  it("shows Yesterday for the previous calendar day", () => {
    expect(formatListDate(new Date(2026, 9, 6, 23, 59), now, "en-US")).toBe("Yesterday");
    expect(formatListDate(new Date(2026, 9, 6, 0, 1), now, "en-US")).toBe("Yesterday");
  });

  it("shows the weekday for 2 to 6 days back", () => {
    expect(formatListDate(new Date(2026, 9, 5, 12, 0), now, "en-US")).toBe("Monday");
    expect(formatListDate(new Date(2026, 9, 1, 0, 0), now, "en-US")).toBe("Thursday");
  });

  it("shows the short date from 7 days back, and for future days", () => {
    expect(formatListDate(new Date(2026, 8, 30, 23, 0), now, "en-US")).toBe("9/30/26");
    expect(formatListDate(new Date(2025, 0, 2, 8, 0), now, "en-US")).toBe("1/2/25");
    expect(formatListDate(new Date(2026, 9, 8, 8, 0), now, "en-US")).toBe("10/8/26");
  });

  it("counts calendar days, not 24-hour periods", () => {
    const earlyNow = new Date(2026, 9, 7, 0, 30);
    expect(formatListDate(new Date(2026, 9, 6, 23, 30), earlyNow, "en-US")).toBe("Yesterday");
  });
});

describe("formatHeaderDate", () => {
  it("uses the long date and short time", () => {
    const d = new Date(2026, 9, 7, 9, 41);
    const want = new Intl.DateTimeFormat("en-US", { dateStyle: "long", timeStyle: "short" }).format(d);
    expect(formatHeaderDate(d, "en-US")).toBe(want);
    expect(want.startsWith("October 7, 2026")).toBe(true);
  });
});

describe("formatSize", () => {
  it.each([
    [0, "0 bytes"],
    [1, "1 byte"],
    [999, "999 bytes"],
    [1000, "1 KB"],
    [12_345, "12 KB"],
    [999_499, "999 KB"],
    [999_500, "1 MB"],
    [1_400_000, "1.4 MB"],
    [2_000_000, "2 MB"],
    [12_500_000, "12.5 MB"],
    [3_210_000_000, "3.2 GB"],
  ])("formats %d as %s", (bytes, want) => {
    expect(formatSize(bytes)).toBe(want);
  });
});

describe("formatCount", () => {
  it("groups digits", () => {
    expect(formatCount(1234, "en-US")).toBe("1,234");
    expect(formatCount(0, "en-US")).toBe("0");
  });
});

describe("displayName", () => {
  it("prefers the trimmed name, then the address", () => {
    expect(displayName(addr("  Ann Smith ", "ann@mailtest.test"))).toBe("Ann Smith");
    expect(displayName(addr("", "ann@mailtest.test"))).toBe("ann@mailtest.test");
    expect(displayName(addr(" ", ""))).toBe("Unknown Sender");
  });
});

describe("formatAddressList", () => {
  const list = [
    addr("Ann", "a@x.test"),
    addr("", "b@x.test"),
    addr("Cat", "c@x.test"),
    addr("Dan", "d@x.test"),
    addr("Eve", "e@x.test"),
  ];
  it("joins up to max display names", () => {
    expect(formatAddressList(list.slice(0, 3))).toBe("Ann, b@x.test, Cat");
    expect(formatAddressList(list.slice(0, 1))).toBe("Ann");
    expect(formatAddressList([])).toBe("");
  });
  it("summarizes the rest", () => {
    expect(formatAddressList(list)).toBe("Ann, b@x.test, Cat & 2 more");
    expect(formatAddressList(list, 4)).toBe("Ann, b@x.test, Cat, Dan & 1 more");
  });
});

describe("initials", () => {
  it.each([
    ["Ann Smith", "ann@x.test", "AS"],
    ["ann", "ann@x.test", "A"],
    ["Dr. John Q. Public", "jqp@x.test", "DP"],
    ["'Bob' (Work)", "bob@x.test", "BW"],
    ["Ünal Çelik", "u@x.test", "ÜÇ"],
    ["", "zoe@x.test", "Z"],
    ["", "", "?"],
    ["123", "4@x.test", "?"],
  ])("%s <%s> is %s", (name, address, want) => {
    expect(initials(addr(name, address))).toBe(want);
  });
});

describe("avatarTone", () => {
  it("is FNV-1a over the lowercased address, modulo 8", () => {
    expect(avatarTone("ann@mailtest.test")).toBe(3);
    expect(avatarTone("ANN@mailtest.test")).toBe(3);
    expect(avatarTone("bob@example.test")).toBe(0);
    expect(avatarTone("carol@example.test")).toBe(2);
    expect(avatarTone("dave@example.test")).toBe(5);
    expect(avatarTone("frank@example.test")).toBe(1);
    expect(avatarTone("zoë@example.test")).toBe(0);
    expect(avatarTone("")).toBe(5);
  });
});
