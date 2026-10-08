// CONTRACT TEST for task card T-0056 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ServerConfig } from "../../rpc/gen/api";
import { defaultPort, ServerFields } from "./ServerFields";

const imap: ServerConfig = { host: "imap.x.test", port: 993, tls: "tls", username: "ann" };

function fields(value: ServerConfig, id: "imap" | "smtp" = "imap", disabled?: boolean) {
  const onChange = vi.fn();
  render(<ServerFields id={id} legend="Incoming Mail (IMAP)" value={value} onChange={onChange} disabled={disabled} />);
  return onChange;
}

function input(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement;
}

describe("defaultPort", () => {
  it("knows IMAP's and SMTP's ports", () => {
    expect([defaultPort("imap", "tls"), defaultPort("imap", "starttls"), defaultPort("imap", "insecure")]).toEqual([
      993, 143, 143,
    ]);
    expect([defaultPort("smtp", "tls"), defaultPort("smtp", "starttls"), defaultPort("smtp", "insecure")]).toEqual([
      465, 587, 25,
    ]);
  });
});

describe("ServerFields", () => {
  it("shows the server in labeled controls", () => {
    fields(imap);
    const set = screen.getByRole("group", { name: "Incoming Mail (IMAP)" });
    expect(set.tagName).toBe("FIELDSET");
    expect(set.className).toContain("grid-cols-[128px_1fr]");
    expect(input("Server:").value).toBe("imap.x.test");
    expect(input("Server:").id).toBe("imap-host");
    expect(input("Port:").value).toBe("993");
    expect(input("Port:").type).toBe("number");
    expect(input("Port:").className).toContain("w-24");
    expect(input("User Name:").value).toBe("ann");
    expect(input("User Name:").id).toBe("imap-username");
    const tls = screen.getByLabelText("Security:") as HTMLSelectElement;
    expect(tls.id).toBe("imap-tls");
    expect(tls.value).toBe("tls");
    expect(Array.from(tls.options).map((o) => [o.value, o.text])).toEqual([
      ["tls", "TLS"],
      ["starttls", "STARTTLS"],
      ["insecure", "None (insecure)"],
    ]);
    expect(input("Server:").className).toContain("border-separator");
    expect(screen.getByText("Server:").className).toContain("text-secondary");
  });

  it("reports each change as a new value", () => {
    const value = { ...imap };
    const onChange = fields(value);
    fireEvent.change(input("Server:"), { target: { value: "mail.x.test" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...imap, host: "mail.x.test" });
    fireEvent.change(input("User Name:"), { target: { value: "ann@x.test" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...imap, username: "ann@x.test" });
    fireEvent.change(input("Port:"), { target: { value: "1993" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...imap, port: 1993 });
    fireEvent.change(input("Port:"), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...imap, port: 0 });
    expect(value).toEqual(imap);
  });

  it("moves a default port with the security mode and keeps a custom one", () => {
    const onChange = fields(imap);
    fireEvent.change(screen.getByLabelText("Security:"), { target: { value: "starttls" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...imap, tls: "starttls", port: 143 });
  });

  it("keeps a port that is not the default", () => {
    const onChange = fields({ ...imap, port: 1143 }, "smtp");
    fireEvent.change(screen.getByLabelText("Security:"), { target: { value: "starttls" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...imap, port: 1143, tls: "starttls" });
  });

  it("fills an empty port and uses SMTP's ports for smtp", () => {
    const onChange = fields({ host: "", port: 0, tls: "tls", username: "" }, "smtp");
    expect(input("Port:").value).toBe("");
    expect(input("Server:").id).toBe("smtp-host");
    fireEvent.change(screen.getByLabelText("Security:"), { target: { value: "starttls" } });
    expect(onChange).toHaveBeenLastCalledWith({ host: "", port: 587, tls: "starttls", username: "" });
  });

  it("can be disabled", () => {
    fields(imap, "imap", true);
    expect((screen.getByRole("group") as HTMLFieldSetElement).disabled).toBe(true);
  });
});
