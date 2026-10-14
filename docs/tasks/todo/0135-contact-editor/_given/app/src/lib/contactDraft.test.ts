// CONTRACT TEST for task card T-0135 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Contact } from "../rpc/gen/api";
import { birthdayValue, changedFields, draftOf, nextLabel, validate } from "./contactDraft";

const ann: Contact = {
  id: 201,
  collectionId: 101,
  accountId: 1,
  displayName: "Ann Smith",
  givenName: "Ann",
  familyName: "Smith",
  nickname: "",
  organization: "Northwind",
  title: "Head of Operations",
  emails: [
    { label: "work", value: "ann.smith@northwind.test" },
    { label: "home", value: "ann@smith-family.test" },
  ],
  phones: [{ label: "mobile", value: "+1 555 0142" }],
  addresses: [
    { label: "work", street: "1 Harbour Way", locality: "Portland", region: "OR", postcode: "97201", country: "USA" },
  ],
  urls: [],
  birthday: "--04-12",
  note: "Runs the offsite.",
  readOnly: false,
};

describe("contact drafts", () => {
  it("start from the contact, or empty", () => {
    const d = draftOf(ann);
    expect(d.givenName).toBe("Ann");
    expect(d.emails).toEqual(ann.emails);
    expect(d.emails).not.toBe(ann.emails);
    expect(d.birthday).toEqual({ month: "04", day: "12", year: "" });
    expect(draftOf({ ...ann, birthday: "1990-05-01" }).birthday).toEqual({ month: "05", day: "01", year: "1990" });
    const empty = draftOf(null);
    expect(empty.givenName).toBe("");
    expect(empty.emails).toEqual([]);
    expect(empty.birthday).toEqual({ month: "", day: "", year: "" });
  });

  it("write birthdays as the API does", () => {
    expect(birthdayValue({ month: "05", day: "01", year: "1990" })).toBe("1990-05-01");
    expect(birthdayValue({ month: "04", day: "12", year: "" })).toBe("--04-12");
    expect(birthdayValue({ month: "", day: "", year: "" })).toBe("");
  });

  it("offer the first label not yet used", () => {
    expect(nextLabel("email", ["work", "home"])).toBe("other");
    expect(nextLabel("phone", [])).toBe("mobile");
    expect(nextLabel("url", [])).toBe("homepage");
    expect(nextLabel("address", ["home", "work", "other"])).toBe("other");
  });

  it("validate emails, birthdays and new contacts", () => {
    const d = draftOf(ann);
    expect(validate(d, false)).toEqual({});
    d.emails[1] = { label: "home", value: "ann at home" };
    d.birthday = { month: "02", day: "30", year: "" };
    const errors = validate(d, false);
    expect(Object.keys(errors).sort()).toEqual(["birthday", "emails.1"]);
    expect(validate({ ...draftOf(null), birthday: { month: "02", day: "29", year: "" } }, false)).toEqual({});
    expect(
      validate({ ...draftOf(null), birthday: { month: "02", day: "29", year: "2023" } }, false).birthday,
    ).toBeTruthy();
    expect(Object.keys(validate(draftOf(null), true))).toEqual(["name"]);
    expect(validate({ ...draftOf(null), organization: "Lumen" }, true)).toEqual({});
    expect(validate({ ...draftOf(null), emails: [{ label: "", value: "a@b.test" }] }, true)).toEqual({});
    expect(validate(draftOf(null), false)).toEqual({});
  });

  it("send only what changed, lists whole", () => {
    expect(changedFields(ann, draftOf(ann))).toEqual({});
    const d = draftOf(ann);
    d.organization = " Northwind Traders ";
    d.phones[0] = { label: "mobile", value: "+1 555 0199" };
    d.emails.push({ label: "other", value: "  " });
    expect(changedFields(ann, d)).toEqual({
      organization: "Northwind Traders",
      phones: [{ label: "mobile", value: "+1 555 0199" }],
    });
    const cleared = draftOf(ann);
    cleared.birthday = { month: "", day: "", year: "" };
    cleared.addresses = [];
    expect(changedFields(ann, cleared)).toEqual({ birthday: "", addresses: [] });
  });

  it("send a new contact's filled fields", () => {
    const d = draftOf(null);
    d.givenName = "Kai";
    d.emails = [
      { label: "home", value: " kai@berg.test " },
      { label: "work", value: "" },
    ];
    d.birthday = { month: "06", day: "23", year: "" };
    expect(changedFields(null, d)).toEqual({
      givenName: "Kai",
      emails: [{ label: "home", value: "kai@berg.test" }],
      birthday: "--06-23",
    });
  });
});
