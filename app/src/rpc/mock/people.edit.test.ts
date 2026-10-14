// CONTRACT TEST for task card T-0134 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client, ErrorCode } from "../gen/api";
import { BOOKS, mockData } from "./fixture";
import { MockTransport } from "./mock";

// Contact IDs in the fixture: Ann Smith 201, Bob Okafor 202, Carmen
// Lindqvist 203, Elif Tanaka 205 (and her read-only Shared card 206),
// Vertex News 212.
function client() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  const events: string[] = [];
  mock.onEvent((e) => events.push(e.event));
  return { mock, c: new Client(mock), events };
}

async function code(p: Promise<unknown>): Promise<number | undefined> {
  try {
    await p;
    return undefined;
  } catch (err) {
    return (err as { code?: number }).code;
  }
}

const names = async (c: Client) => (await c.people.list({})).map((p) => p.displayName);

describe("MockTransport contact edits", () => {
  it("creates a contact in the default address book and announces it", async () => {
    const { c, events } = client();
    const w = await c.people.create({
      fields: { givenName: "Kai", familyName: "Berg", emails: [{ label: "home", value: "kai@berg.test" }] },
    });
    expect(w.person?.displayName).toBe("Kai Berg");
    expect(w.person?.contacts[0]?.collectionId).toBe(BOOKS.contacts);
    expect(w.person?.contacts[0]?.id).toBe(w.contactId);
    expect(typeof w.restoreId).toBe("string");
    expect(await names(c)).toContain("Kai Berg");
    await Promise.resolve();
    expect(events).toContain("people.changed");

    const company = await c.people.create({ collectionId: BOOKS.contacts, fields: { organization: "Lumen" } });
    expect(company.person?.displayName).toBe("Lumen");
  });

  it("refuses contacts it cannot make", async () => {
    const { c } = client();
    expect(await code(c.people.create({ fields: { note: "nobody" } }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.people.create({ fields: { emails: [{ label: "", value: "not-an-address" }] } }))).toBe(
      ErrorCode.invalidParams,
    );
    expect(await code(c.people.create({ fields: { givenName: "A", birthday: "1990-02-30" } }))).toBe(
      ErrorCode.invalidParams,
    );
    expect(await code(c.people.create({ collectionId: BOOKS.shared, fields: { givenName: "A" } }))).toBe(
      ErrorCode.conflict,
    );
    expect(await code(c.people.create({ collectionId: 999, fields: { givenName: "A" } }))).toBe(ErrorCode.notFound);
  });

  it("changes a contact's fields and keeps the rest", async () => {
    const { c } = client();
    const w = await c.people.update({
      id: 201,
      fields: { organization: "Northwind Traders", phones: [{ label: "mobile", value: "+1 555 0199" }] },
    });
    const ann = w.person?.contacts[0];
    expect(w.person?.organization).toBe("Northwind Traders");
    expect(ann?.phones).toEqual([{ label: "mobile", value: "+1 555 0199" }]);
    expect(ann?.emails.length).toBe(2);
    expect(w.person?.displayName).toBe("Ann Smith");
    expect((await c.people.update({ id: 201, fields: { givenName: "Anne" } })).person?.displayName).toBe("Anne Smith");
    expect((await c.people.get({ id: 201 })).contacts[0]?.birthday).toBe("--04-12");
  });

  it("keeps a display name the user chose", async () => {
    const { c } = client();
    expect((await c.people.update({ id: 212, fields: { organization: "Vertex Media" } })).person?.displayName).toBe(
      "Vertex News",
    );
  });

  it("refuses read-only and unknown contacts", async () => {
    const { c } = client();
    expect(await code(c.people.update({ id: 206, fields: { nickname: "E" } }))).toBe(ErrorCode.conflict);
    expect(await code(c.people.delete({ id: 206 }))).toBe(ErrorCode.conflict);
    expect(await code(c.people.update({ id: 999, fields: { nickname: "E" } }))).toBe(ErrorCode.notFound);
    expect(await code(c.people.update({ id: 201, fields: { emails: [{ label: "", value: "x" }] } }))).toBe(
      ErrorCode.invalidParams,
    );
  });

  it("deletes a contact, and the person with their last one", async () => {
    const { c } = client();
    const bob = await c.people.delete({ id: 202 });
    expect(bob.person).toBeUndefined();
    expect(await names(c)).not.toContain("Bob Okafor");
    const elif = await c.people.delete({ id: 205 });
    expect(elif.person?.contacts.map((x) => x.id)).toEqual([206]);
    expect(await names(c)).toContain("Elif T.");
  });

  it("sets and removes a photo", async () => {
    const { c } = client();
    const photo = { contentType: "image/jpeg", data: btoa("jpeg bytes") };
    expect((await c.people.setPhoto({ id: 203, photo })).person?.hasPhoto).toBe(true);
    expect(await c.people.photo({ id: 203 })).toEqual(photo);
    expect((await c.people.setPhoto({ id: 203 })).person?.hasPhoto).toBe(false);
    expect(await code(c.people.photo({ id: 203 }))).toBe(ErrorCode.notFound);
    expect(await code(c.people.setPhoto({ id: 203, photo: { contentType: "image/gif", data: btoa("GIF89a") } }))).toBe(
      ErrorCode.invalidParams,
    );
    const big = btoa("x".repeat(300 * 1024));
    expect(await code(c.people.setPhoto({ id: 203, photo: { contentType: "image/png", data: big } }))).toBe(
      ErrorCode.invalidParams,
    );
  });

  it("restores what each change replaced, once", async () => {
    const { c } = client();
    const made = await c.people.create({ fields: { givenName: "Kai" } });
    await c.people.restore({ restoreId: made.restoreId });
    expect(await names(c)).not.toContain("Kai");

    const changed = await c.people.update({ id: 201, fields: { title: "COO", givenName: "Anne" } });
    await c.people.restore({ restoreId: changed.restoreId });
    const ann = await c.people.get({ id: 201 });
    expect(ann.displayName).toBe("Ann Smith");
    expect(ann.contacts[0]?.title).toBe("Head of Operations");

    const gone = await c.people.delete({ id: 202 });
    await c.people.restore({ restoreId: gone.restoreId });
    expect(await names(c)).toContain("Bob Okafor");

    const half = await c.people.delete({ id: 205 });
    await c.people.restore({ restoreId: half.restoreId });
    expect((await names(c)).filter((n) => n.startsWith("Elif"))).toEqual(["Elif Tanaka"]);
    expect((await c.people.get({ id: 205 })).contacts.map((x) => x.id).sort()).toEqual([205, 206]);

    const pictured = await c.people.setPhoto({ id: 203, photo: { contentType: "image/png", data: btoa("png") } });
    await c.people.restore({ restoreId: pictured.restoreId });
    expect((await c.people.get({ id: 203 })).hasPhoto).toBe(false);

    expect(await code(c.people.restore({ restoreId: pictured.restoreId }))).toBe(ErrorCode.notFound);
  });
});
