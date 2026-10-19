import { describe, expect, it } from "vitest";

import type { Source } from "../data/stores";
import { parseRoute, routeOf } from "./routes";

describe("lodge routes", () => {
  it("round-trips every source, with and without a message", () => {
    const sources: Source[] = [
      { kind: "allInboxes" },
      { kind: "mailbox", mailboxId: 12 },
      { kind: "smart", id: 3 },
      { kind: "role", role: "sent" },
      { kind: "flagged" },
      { kind: "flagColor", color: 2 },
      { kind: "vips" },
    ];
    for (const source of sources) {
      expect(parseRoute(routeOf(source, []))).toEqual({ source });
      expect(parseRoute(routeOf(source, [42]))).toEqual({ source, message: 42 });
    }
  });

  it("names the routes the spec lists", () => {
    expect(routeOf({ kind: "allInboxes" }, [])).toBe("/");
    expect(routeOf({ kind: "mailbox", mailboxId: 7 }, [])).toBe("/mailbox/7");
    expect(routeOf({ kind: "mailbox", mailboxId: 7 }, [9])).toBe("/mailbox/7/message/9");
    expect(routeOf({ kind: "allInboxes" }, [9])).toBe("/message/9");
    expect(routeOf({ kind: "mailbox", mailboxId: 7 }, [9, 10])).toBe("/mailbox/7");
    expect(routeOf({ kind: "vip", key: "vip:a", addresses: ["a@x"] }, [])).toBe("/vips");
  });

  it("reads anything else as All Inboxes", () => {
    for (const bad of [
      "",
      "/nope",
      "/mailbox/x",
      "/mailbox/0",
      "/mailbox/1/2",
      "/role/bogus",
      "/flag/9",
      "/message/abc",
      "/x/message/3",
    ]) {
      expect(parseRoute(bad).source).toEqual({ kind: "allInboxes" });
    }
    expect(parseRoute("/mailbox/5?q=1#top")).toEqual({ source: { kind: "mailbox", mailboxId: 5 } });
    expect(parseRoute("/x/message/3")).toEqual({ source: { kind: "allInboxes" }, message: 3 });
  });
});
