// CONTRACT TEST for task card T-0143 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import {
  defaultRepeat,
  endOf,
  followStart,
  nthWeekday,
  presetOf,
  presetRule,
  type Repeat,
  repeatOf,
  ruleOf,
  withEnd,
} from "./repeat";

// 2026-10-15 is the third Thursday of October; 2026-10-29 the fifth, so
// the last; 2026-02-26 both the fourth and the last Thursday of February.
const thursday = "2026-10-15";

describe("presetOf", () => {
  it.each([
    ["", "never"],
    ["FREQ=DAILY", "daily"],
    ["FREQ=DAILY;COUNT=5", "daily"],
    ["FREQ=DAILY;INTERVAL=3", "custom"],
    ["FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "weekdays"],
    ["FREQ=WEEKLY;BYDAY=FR,TH,WE,TU,MO;UNTIL=20261231T235959Z", "weekdays"],
    ["FREQ=WEEKLY", "weekly"],
    ["FREQ=WEEKLY;BYDAY=TH", "weekly"],
    ["FREQ=WEEKLY;INTERVAL=1;WKST=SU", "weekly"],
    ["FREQ=WEEKLY;BYDAY=FR", "custom"],
    ["FREQ=WEEKLY;BYDAY=TU,TH", "custom"],
    ["FREQ=WEEKLY;INTERVAL=2", "biweekly"],
    ["FREQ=WEEKLY;INTERVAL=2;BYDAY=TH", "biweekly"],
    ["FREQ=WEEKLY;INTERVAL=3", "custom"],
    ["FREQ=MONTHLY", "monthly"],
    ["FREQ=MONTHLY;BYMONTHDAY=15", "monthly"],
    ["FREQ=MONTHLY;BYMONTHDAY=1", "custom"],
    ["FREQ=MONTHLY;BYDAY=3TH", "custom"],
    ["FREQ=YEARLY", "yearly"],
    ["FREQ=YEARLY;BYMONTH=10;BYMONTHDAY=15", "yearly"],
    ["FREQ=YEARLY;BYMONTH=3", "custom"],
    ["FREQ=HOURLY", "custom"],
    ["FREQ=WEEKLY;BYSETPOS=1;BYDAY=TH", "custom"],
    ["nonsense", "custom"],
  ])("%s is %s", (rule, preset) => {
    expect(presetOf(rule, thursday)).toBe(preset);
  });
});

describe("presetRule", () => {
  it.each([
    ["never", ""],
    ["daily", "FREQ=DAILY"],
    ["weekdays", "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"],
    ["weekly", "FREQ=WEEKLY"],
    ["biweekly", "FREQ=WEEKLY;INTERVAL=2"],
    ["monthly", "FREQ=MONTHLY"],
    ["yearly", "FREQ=YEARLY"],
  ] as const)("%s writes %s", (preset, rule) => {
    expect(presetRule(preset)).toBe(rule);
    expect(presetOf(rule, thursday)).toBe(preset);
  });
});

describe("nthWeekday", () => {
  it.each([
    ["2026-10-01", "1TH"],
    [thursday, "3TH"],
    ["2026-10-22", "4TH"],
    ["2026-10-29", "-1TH"],
    ["2026-02-26", "4TH"],
    ["2026-11-30", "-1MO"],
  ])("%s is %s", (date, byDay) => {
    expect(nthWeekday(date)).toBe(byDay);
  });
});

describe("repeatOf and ruleOf", () => {
  const weekly = (weekdays: Repeat["weekdays"], interval = 1): Repeat => ({
    frequency: "WEEKLY",
    interval,
    weekdays,
    byWeekday: false,
  });
  it.each<[string, string, Repeat, string]>([
    [
      "FREQ=DAILY;INTERVAL=2",
      thursday,
      { frequency: "DAILY", interval: 2, weekdays: [], byWeekday: false },
      "FREQ=DAILY;INTERVAL=2",
    ],
    ["FREQ=WEEKLY", thursday, weekly(["TH"]), "FREQ=WEEKLY"],
    ["FREQ=WEEKLY;BYDAY=TH", thursday, weekly(["TH"]), "FREQ=WEEKLY"],
    [
      "FREQ=WEEKLY;BYDAY=SU,TU,TH;INTERVAL=2",
      thursday,
      weekly(["TU", "TH", "SU"], 2),
      "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,TH,SU",
    ],
    ["FREQ=WEEKLY;BYDAY=FR", thursday, weekly(["FR"]), "FREQ=WEEKLY;BYDAY=FR"],
    [
      "FREQ=MONTHLY;BYMONTHDAY=15",
      thursday,
      { frequency: "MONTHLY", interval: 1, weekdays: [], byWeekday: false },
      "FREQ=MONTHLY",
    ],
    [
      "FREQ=MONTHLY;BYDAY=3TH;INTERVAL=3",
      thursday,
      { frequency: "MONTHLY", interval: 3, weekdays: [], byWeekday: true },
      "FREQ=MONTHLY;INTERVAL=3;BYDAY=3TH",
    ],
    [
      "FREQ=MONTHLY;BYDAY=-1TH",
      "2026-10-29",
      { frequency: "MONTHLY", interval: 1, weekdays: [], byWeekday: true },
      "FREQ=MONTHLY;BYDAY=-1TH",
    ],
    [
      "FREQ=MONTHLY;BYDAY=-1TH",
      "2026-02-26",
      { frequency: "MONTHLY", interval: 1, weekdays: [], byWeekday: true },
      "FREQ=MONTHLY;BYDAY=4TH",
    ],
    [
      "FREQ=YEARLY;BYMONTH=10;BYMONTHDAY=15;COUNT=3",
      thursday,
      { frequency: "YEARLY", interval: 1, weekdays: [], byWeekday: false },
      "FREQ=YEARLY",
    ],
  ])("%s from %s", (rule, start, repeat, written) => {
    expect(repeatOf(rule, start)).toEqual(repeat);
    expect(ruleOf(repeat, start)).toBe(written);
  });

  it.each([
    "FREQ=HOURLY",
    "FREQ=WEEKLY;BYDAY=1TH",
    "FREQ=MONTHLY;BYDAY=2TH",
    "FREQ=MONTHLY;BYMONTHDAY=1,15",
    "FREQ=MONTHLY;BYDAY=TH;BYSETPOS=-1",
    "FREQ=YEARLY;BYMONTH=3",
    "FREQ=DAILY;BYHOUR=9",
    "FREQ=DAILY;INTERVAL=0",
    "INTERVAL=2",
    "",
  ])("the sheet cannot show %s", (rule) => {
    expect(repeatOf(rule, thursday)).toBeUndefined();
  });

  it("starts the sheet on the start's day each week", () => {
    expect(defaultRepeat(thursday)).toEqual(weekly(["TH"]));
    expect(ruleOf(defaultRepeat("2026-10-18"), "2026-10-18")).toBe("FREQ=WEEKLY");
    expect(ruleOf(weekly([]), thursday)).toBe("FREQ=WEEKLY");
  });
});

describe("endOf and withEnd", () => {
  it.each<[string, string]>([
    ["FREQ=DAILY", "never"],
    ["FREQ=DAILY;COUNT=7", "count 7"],
    ["FREQ=DAILY;UNTIL=20261231", "date 2026-12-31"],
    ["FREQ=DAILY;UNTIL=20261218T045959Z", "date 2026-12-17"],
    ["FREQ=DAILY;UNTIL=20261217T235959", "date 2026-12-17"],
    ["FREQ=DAILY;UNTIL=soon", "never"],
  ])("%s ends %s in New York", (rule, want) => {
    const end = endOf(rule, "America/New_York");
    const got = end.kind === "count" ? `count ${end.count}` : end.kind === "date" ? `date ${end.date}` : "never";
    expect(got).toBe(want);
  });

  it("writes the end in place of the old one", () => {
    const zone = "America/New_York";
    expect(withEnd("FREQ=WEEKLY;COUNT=10;BYDAY=TH", { kind: "never" }, false, zone)).toBe("FREQ=WEEKLY;BYDAY=TH");
    expect(withEnd("FREQ=WEEKLY", { kind: "count", count: 4 }, false, zone)).toBe("FREQ=WEEKLY;COUNT=4");
    // The last moment of the day in the event's zone: EST in December,
    // EDT in October.
    expect(withEnd("FREQ=WEEKLY;COUNT=4", { kind: "date", date: "2026-12-17" }, false, zone)).toBe(
      "FREQ=WEEKLY;UNTIL=20261218T045959Z",
    );
    expect(withEnd("FREQ=DAILY", { kind: "date", date: "2026-10-20" }, false, zone)).toBe(
      "FREQ=DAILY;UNTIL=20261021T035959Z",
    );
    expect(withEnd("FREQ=DAILY", { kind: "date", date: "2026-10-20" }, false, "UTC")).toBe(
      "FREQ=DAILY;UNTIL=20261020T235959Z",
    );
    expect(withEnd("FREQ=YEARLY;UNTIL=20300101", { kind: "date", date: "2031-06-01" }, true, zone)).toBe(
      "FREQ=YEARLY;UNTIL=20310601",
    );
    expect(withEnd("", { kind: "count", count: 3 }, false, zone)).toBe("");
  });

  it("reads back what it writes", () => {
    const zone = "Europe/Berlin";
    for (const end of [
      { kind: "count", count: 12 },
      { kind: "date", date: "2027-03-28" },
      { kind: "never" },
    ] as const) {
      expect(endOf(withEnd("FREQ=DAILY", end, false, zone), zone)).toEqual(end);
      expect(endOf(withEnd("FREQ=DAILY", end, true, zone), zone)).toEqual(end);
    }
  });
});

describe("followStart", () => {
  it.each([
    ["FREQ=WEEKLY;BYDAY=TH", thursday, "2026-10-16", "FREQ=WEEKLY;BYDAY=FR"],
    ["FREQ=WEEKLY;BYDAY=TH;COUNT=10", thursday, "2026-10-20", "FREQ=WEEKLY;BYDAY=TU;COUNT=10"],
    ["FREQ=WEEKLY;BYDAY=TU,TH", thursday, "2026-10-16", "FREQ=WEEKLY;BYDAY=TU,TH"],
    ["FREQ=WEEKLY", thursday, "2026-10-16", "FREQ=WEEKLY"],
    ["FREQ=MONTHLY;BYDAY=3TH", thursday, "2026-10-05", "FREQ=MONTHLY;BYDAY=1MO"],
    ["FREQ=MONTHLY;BYDAY=-1TH", "2026-10-29", "2026-11-30", "FREQ=MONTHLY;BYDAY=-1MO"],
    ["FREQ=MONTHLY;BYDAY=-1TH", "2026-10-29", "2026-11-02", "FREQ=MONTHLY;BYDAY=1MO"],
    ["FREQ=MONTHLY;BYMONTHDAY=15", thursday, "2026-10-17", "FREQ=MONTHLY;BYMONTHDAY=17"],
    ["FREQ=MONTHLY;BYMONTHDAY=1", thursday, "2026-10-17", "FREQ=MONTHLY;BYMONTHDAY=1"],
    ["FREQ=YEARLY;BYMONTH=10;BYMONTHDAY=15", thursday, "2026-11-02", "FREQ=YEARLY;BYMONTH=11;BYMONTHDAY=2"],
    ["FREQ=DAILY", thursday, "2026-10-17", "FREQ=DAILY"],
    ["", thursday, "2026-10-17", ""],
  ])("%s from %s to %s", (rule, from, to, want) => {
    expect(followStart(rule, from, to)).toBe(want);
  });
});
