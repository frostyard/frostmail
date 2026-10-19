import { describe, expect, it } from "vitest";

import type { Source } from "../data/stores";
import { parseRoute, permalink, routeOf } from "./routes";

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

  it("names a message by its Message-ID, which maild resolves", () => {
    const all = { kind: "allInboxes" } as const;
    expect(permalink("gen-1-200@mailgen.test")).toBe("/message/gen-1-200%40mailgen.test");
    for (const messageId of ["gen-1-200@mailgen.test", "a/b+c@x.test", "Ünï code@x", "12ab@x"]) {
      expect(parseRoute(permalink(messageId))).toEqual({ source: all, messageId });
    }
    expect(parseRoute("/message/plan@x.test")).toEqual({ source: all, messageId: "plan@x.test" });
    expect(parseRoute("/message/plan%40x.test?from=notes#top")).toEqual({ source: all, messageId: "plan@x.test" });
    // A maild ID stays one; a malformed escape or a blank names nothing.
    expect(parseRoute("/message/42")).toEqual({ source: all, message: 42 });
    expect(parseRoute("/message/%E0%A4")).toEqual({ source: all });
    expect(parseRoute("/message/%20")).toEqual({ source: all });
    // Only /message/<Message-ID> is permanent: under a source it names no message.
    expect(parseRoute("/mailbox/3/message/plan@x")).toEqual({ source: { kind: "mailbox", mailboxId: 3 } });
  });
});
