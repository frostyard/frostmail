import { describe, expect, it } from "vitest";

import { Client } from "../gen/api";
import { BOOKS, mockData } from "./fixture";
import { MockTransport } from "./mock";

function client() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  return { mock, c: new Client(mock) };
}

describe("MockTransport people", () => {
  it("lists people by family name, filed by letter", async () => {
    const { c } = client();
    const list = await c.people.list({});
    expect(list.map((p) => p.index).join("")).toBe("#GHKLMNORSTV");
    expect(list[1]?.displayName).toBe("Farah Garcia");
    expect((await c.people.list({ query: "north" })).map((p) => p.displayName)).toEqual(["Farah Garcia", "Ann Smith"]);
    expect((await c.people.list({ collectionId: BOOKS.shared })).map((p) => p.displayName)).toEqual(["Elif Tanaka"]);
  });

  it("answers contact cards for people and strangers", async () => {
    const { c } = client();
    const ann = await c.people.card({ email: "Ann.Smith@northwind.test" });
    expect(ann.person?.displayName).toBe("Ann Smith");
    expect(ann.recent.length).toBeGreaterThan(0);
    expect(ann.canAdd).toBe(false);
    const stranger = await c.people.card({ email: "nobody@example.test" });
    expect(stranger.person).toBeUndefined();
    expect(stranger.canAdd).toBe(true);
  });

  it("adds a contact and announces it", async () => {
    const { mock, c } = client();
    const events: string[] = [];
    mock.onEvent((e) => events.push(e.event));
    const p = await c.people.add({ email: "New@Example.test", name: "New Person" });
    expect(p.contacts[0]?.familyName).toBe("Person");
    await Promise.resolve();
    expect(events).toContain("people.changed");
    await expect(c.people.add({ email: "new@example.test" })).rejects.toThrow();
  });
});
