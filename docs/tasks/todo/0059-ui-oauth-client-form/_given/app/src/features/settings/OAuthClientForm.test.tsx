// CONTRACT TEST for task card T-0059 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OAuthClient } from "../../rpc/gen/api";
import { OAuthClientForm } from "./OAuthClientForm";

const stored: OAuthClient = { provider: "google", clientId: "123.apps.googleusercontent.com", hasSecret: true };

function clientForm(client: OAuthClient | null, over: { busy?: boolean; error?: string | null } = {}) {
  const onSave = vi.fn();
  const view = render(
    <OAuthClientForm client={client} busy={over.busy ?? false} error={over.error ?? null} onSave={onSave} />,
  );
  return { onSave, ...view };
}

function field(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement;
}

function save(): HTMLButtonElement {
  return screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;
}

describe("OAuthClientForm", () => {
  it("explains the client and shows that none is stored", () => {
    clientForm(null);
    expect(screen.getByRole("form", { name: "Google client" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Google Client");
    expect(screen.getByText(/Create a Desktop app client in Google Cloud/).className).toContain("text-secondary");
    expect(screen.getByText("No client stored")).toBeTruthy();
    expect(field("Client ID:").value).toBe("");
    expect(field("Client Secret:").type).toBe("password");
    expect(field("Client Secret:").placeholder).toBe("");
    expect(save().disabled).toBe(true);
  });

  it("shows the stored client without its secret", () => {
    clientForm(stored);
    expect(field("Client ID:").value).toBe("123.apps.googleusercontent.com");
    expect(field("Client Secret:").value).toBe("");
    expect(field("Client Secret:").placeholder).toBe("Stored");
    expect(screen.getByText("Client and secret stored")).toBeTruthy();
    expect(save().disabled).toBe(true);
  });

  it("names a client stored without a secret", () => {
    clientForm({ ...stored, hasSecret: false });
    expect(screen.getByText("Client stored without a secret")).toBeTruthy();
  });

  it("saves a new client with its secret and clears the secret field", () => {
    const { onSave } = clientForm(null);
    fireEvent.change(field("Client ID:"), { target: { value: "  456.apps.googleusercontent.com " } });
    fireEvent.change(field("Client Secret:"), { target: { value: "GOCSPX-abc" } });
    expect(save().disabled).toBe(false);
    fireEvent.click(save());
    expect(onSave).toHaveBeenCalledWith("456.apps.googleusercontent.com", "GOCSPX-abc");
    expect(field("Client Secret:").value).toBe("");
  });

  it("saves a new secret for the stored client", () => {
    const { onSave } = clientForm(stored);
    fireEvent.change(field("Client Secret:"), { target: { value: "GOCSPX-new" } });
    fireEvent.click(save());
    expect(onSave).toHaveBeenCalledWith("123.apps.googleusercontent.com", "GOCSPX-new");
  });

  it("saves a changed ID and keeps the stored secret", () => {
    const { onSave } = clientForm(stored);
    fireEvent.change(field("Client ID:"), { target: { value: "789.apps.googleusercontent.com" } });
    fireEvent.click(save());
    expect(onSave).toHaveBeenCalledWith("789.apps.googleusercontent.com", "");
  });

  it("needs an ID and waits while busy", () => {
    clientForm(null);
    fireEvent.change(field("Client ID:"), { target: { value: "   " } });
    fireEvent.change(field("Client Secret:"), { target: { value: "x" } });
    expect(save().disabled).toBe(true);
  });

  it("shows the last error and disables Save while busy", () => {
    clientForm(stored, { busy: true, error: "clientId is not a client ID" });
    fireEvent.change(field("Client Secret:"), { target: { value: "x" } });
    expect(save().disabled).toBe(true);
    expect(screen.getByRole("alert").textContent).toBe("clientId is not a client ID");
  });

  it("follows a newly stored client", () => {
    const { rerender } = clientForm(null);
    rerender(<OAuthClientForm client={stored} busy={false} error={null} onSave={() => {}} />);
    expect(field("Client ID:").value).toBe("123.apps.googleusercontent.com");
  });
});
