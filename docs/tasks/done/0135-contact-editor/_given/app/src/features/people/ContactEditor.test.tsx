// CONTRACT TEST for task card T-0135 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Contact } from "../../rpc/gen/api";
import { ContactEditor } from "./ContactEditor";

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
  phones: [
    { label: "mobile", value: "+1 555 0142" },
    { label: "work", value: "+1 555 0100" },
  ],
  addresses: [
    { label: "work", street: "1 Harbour Way", locality: "Portland", region: "OR", postcode: "97201", country: "USA" },
  ],
  urls: [],
  birthday: "--04-12",
  note: "Runs the offsite.",
  readOnly: false,
};

function editor(contact: Contact | null = ann, extra: Partial<Parameters<typeof ContactEditor>[0]> = {}) {
  const onSave = vi.fn();
  const onCancel = vi.fn();
  render(<ContactEditor contact={contact} onSave={onSave} onCancel={onCancel} {...extra} />);
  return { onSave, onCancel };
}

const field = (name: string) => screen.getByRole("textbox", { name }) as HTMLInputElement;
const select = (name: string) => screen.getByRole("combobox", { name }) as HTMLSelectElement;
const type = (name: string, value: string) => fireEvent.change(field(name), { target: { value } });
const done = () => fireEvent.click(screen.getByRole("button", { name: "Done" }));

describe("ContactEditor", () => {
  it("shows the contact's fields", () => {
    editor();
    expect(field("First").value).toBe("Ann");
    expect(field("Last").value).toBe("Smith");
    expect(field("Company").value).toBe("Northwind");
    expect(field("Job Title").value).toBe("Head of Operations");
    expect(field("Email 2").value).toBe("ann@smith-family.test");
    expect(select("Label for email 1").value).toBe("work");
    expect(field("Street 1").value).toBe("1 Harbour Way");
    expect(field("City 1").value).toBe("Portland");
    expect(select("Birthday month").value).toBe("04");
    expect(select("Birthday day").value).toBe("12");
    expect(field("Birthday year").value).toBe("");
    expect(field("Note").value).toBe("Runs the offsite.");
  });

  it("saves only what changed", () => {
    const { onSave } = editor();
    type("Company", "Northwind Traders");
    done();
    expect(onSave).toHaveBeenCalledWith({ organization: "Northwind Traders" }, undefined);
  });

  it("adds, removes and relabels rows", () => {
    const { onSave } = editor();
    fireEvent.click(screen.getByRole("button", { name: "add email" }));
    expect(select("Label for email 3").value).toBe("other");
    type("Email 3", "ann@chess.test");
    fireEvent.click(screen.getByRole("button", { name: "Remove phone 2" }));
    fireEvent.change(select("Label for phone 1"), { target: { value: "\u0000custom" } });
    type("Custom label for phone 1", "Lab");
    done();
    expect(onSave).toHaveBeenCalledWith(
      {
        emails: [...ann.emails, { label: "other", value: "ann@chess.test" }],
        phones: [{ label: "Lab", value: "+1 555 0142" }],
      },
      undefined,
    );
  });

  it("refuses an invalid email until it is fixed", () => {
    const { onSave } = editor();
    type("Email 1", "ann at northwind");
    done();
    expect(onSave).not.toHaveBeenCalled();
    expect(field("Email 1").getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText("Enter an address like name@example.com.")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Done" }) as HTMLButtonElement).disabled).toBe(true);
    type("Email 1", "ann@northwind.test");
    expect((screen.getByRole("button", { name: "Done" }) as HTMLButtonElement).disabled).toBe(false);
    done();
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("writes and clears birthdays", () => {
    const { onSave } = editor();
    fireEvent.change(select("Birthday month"), { target: { value: "05" } });
    fireEvent.change(select("Birthday day"), { target: { value: "01" } });
    type("Birthday year", "1990");
    done();
    expect(onSave).toHaveBeenLastCalledWith({ birthday: "1990-05-01" }, undefined);
  });

  it("makes a new contact in the chosen address book", () => {
    const books = [
      { id: 101, label: "Contacts — test1@mailtest.test" },
      { id: 103, label: "Work — test1@mailtest.test" },
    ];
    const { onSave } = editor(null, { books });
    expect(select("Address Book").value).toBe("101");
    done();
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByText("Add a name, a company or an email address.")).toBeTruthy();
    type("First", "Kai");
    fireEvent.change(select("Address Book"), { target: { value: "103" } });
    done();
    expect(onSave).toHaveBeenCalledWith({ givenName: "Kai" }, 103);
  });

  it("hides the address book with only one", () => {
    editor(null, { books: [{ id: 101, label: "Contacts" }] });
    expect(screen.queryByRole("combobox", { name: "Address Book" })).toBeNull();
  });

  it("cancels with Cancel and Escape", () => {
    const { onCancel } = editor();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(field("First"), { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(2);
  });

  it("shows maild's error, the photo well and Delete Contact", () => {
    const onDelete = vi.fn();
    editor(ann, { error: "contact 201 is read-only", onDelete, photo: <div>the well</div> });
    expect(screen.getByRole("alert").textContent).toBe("contact 201 is read-only");
    expect(screen.getByText("the well")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Delete Contact" }));
    expect(onDelete).toHaveBeenCalled();
  });

  it("offers no Delete Contact without onDelete, and disables Done while busy", () => {
    editor(ann, { busy: true });
    expect(screen.queryByRole("button", { name: "Delete Contact" })).toBeNull();
    expect((screen.getByRole("button", { name: "Done" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
