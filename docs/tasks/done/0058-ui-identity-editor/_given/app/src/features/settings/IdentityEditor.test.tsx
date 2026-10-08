// CONTRACT TEST for task card T-0058 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Account, Identity } from "../../rpc/gen/api";
import { IdentityEditor } from "./IdentityEditor";

const server = { host: "h", port: 993, tls: "tls" as const, username: "u" };

function account(id: number, email: string): Account {
  return {
    id,
    kind: "imap",
    email,
    displayName: "",
    auth: "password",
    imap: server,
    smtp: server,
    createdAt: "2026-10-01T00:00:00Z",
    readOnly: false,
    notify: true,
    signedIn: true,
  };
}

function identity(over: Partial<Identity>): Identity {
  return { id: 1, accountId: 1, name: "", email: "", replyTo: "", signatureHtml: "", isDefault: true, ...over };
}

const accounts = [account(1, "ann@gmail.com"), account(2, "ann@icloud.com"), account(3, "empty@x.test")];
const identities = [
  identity({
    id: 10,
    accountId: 1,
    name: "Ann Example",
    email: "ann@gmail.com",
    signatureHtml: "<p>Ann</p><p>CEO</p>",
  }),
  identity({ id: 11, accountId: 1, name: " ", email: "support@gmail.com", isDefault: false }),
  identity({ id: 20, accountId: 2, name: "Ann", email: "ann@icloud.com", replyTo: "ann@gmail.com" }),
];

function editor(over: { busy?: boolean; error?: string | null; identities?: Identity[] } = {}) {
  const onSave = vi.fn();
  const view = render(
    <IdentityEditor
      accounts={accounts}
      identities={over.identities ?? identities}
      busy={over.busy ?? false}
      error={over.error ?? null}
      onSave={onSave}
    />,
  );
  return { onSave, ...view };
}

function field(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement;
}

function save(): HTMLButtonElement {
  return screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;
}

describe("IdentityEditor", () => {
  it("lists identities under their accounts", () => {
    editor();
    const list = screen.getByRole("list", { name: "Identities" });
    expect(list.className).toContain("w-[220px]");
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual([
      "ann@gmail.com",
      "Ann Exampleann@gmail.com",
      "support@gmail.comsupport@gmail.com",
      "ann@icloud.com",
      "Annann@icloud.com",
    ]);
    expect(within(list).getAllByRole("listitem")[0]?.className).toContain("text-sidebar-section");
  });

  it("starts on the first identity and shows its stored values", () => {
    editor();
    const current = screen.getAllByRole("button").filter((b) => b.getAttribute("aria-current") === "true");
    expect(current.map((b) => b.textContent)).toEqual(["Ann Exampleann@gmail.com"]);
    expect(current[0]?.className).toContain("bg-selection-sidebar");
    expect(field("Name:").value).toBe("Ann Example");
    expect(field("Reply-To:").value).toBe("");
    expect(field("Reply-To:").placeholder).toBe("None");
    expect(field("Signature:").value).toBe("Ann\nCEO");
    expect(field("Signature:").tagName).toBe("TEXTAREA");
    expect(save().disabled).toBe(true);
  });

  it("switches to another identity", () => {
    editor();
    fireEvent.click(screen.getByText("Ann", { selector: "span" }));
    expect(field("Name:").value).toBe("Ann");
    expect(field("Reply-To:").value).toBe("ann@gmail.com");
    expect(screen.getAllByText("ann@icloud.com").length).toBeGreaterThan(1);
  });

  it("saves what was typed, with the signature as HTML", () => {
    const { onSave } = editor();
    fireEvent.change(field("Name:"), { target: { value: "Ann E." } });
    expect(save().disabled).toBe(false);
    fireEvent.change(field("Signature:"), { target: { value: "Ann E.\n\n<CEO>" } });
    fireEvent.click(save());
    expect(onSave).toHaveBeenCalledWith(10, {
      name: "Ann E.",
      replyTo: "",
      signatureHtml: "<p>Ann E.</p><p><br></p><p>&lt;CEO&gt;</p>",
    });
  });

  it("forgets unsaved edits when another identity is selected", () => {
    editor();
    fireEvent.change(field("Name:"), { target: { value: "Changed" } });
    fireEvent.click(screen.getByText("support@gmail.com", { selector: "span.font-semibold" }));
    fireEvent.click(screen.getByText("Ann Example"));
    expect(field("Name:").value).toBe("Ann Example");
  });

  it("shows new stored values after a save", () => {
    const { rerender } = editor();
    const saved = identities.map((i) => (i.id === 10 ? { ...i, name: "Ann Saved" } : i));
    rerender(<IdentityEditor accounts={accounts} identities={saved} busy={false} error={null} onSave={() => {}} />);
    expect(field("Name:").value).toBe("Ann Saved");
    expect(save().disabled).toBe(true);
  });

  it("waits while busy and shows the last error", () => {
    editor({ busy: true, error: "invalid Reply-To" });
    fireEvent.change(field("Name:"), { target: { value: "Changed" } });
    expect(save().disabled).toBe(true);
    expect(screen.getByRole("alert").textContent).toBe("invalid Reply-To");
  });

  it("says what to do without identities", () => {
    editor({ identities: [] });
    expect(screen.getByText("Add an account to edit its signature.").className).toContain("text-secondary");
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });
});
