// CONTRACT TEST for task cards T-0066 and T-0093 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ServiceKind, ServiceSettings } from "../../rpc/gen/api";
import { ServicesSection, type ServicesSectionProps } from "./ServicesSection";

const now = new Date(2026, 9, 8, 15, 0);

function svc(service: ServiceKind, over: Partial<ServiceSettings> = {}): ServiceSettings {
  return { service, available: true, enabled: false, url: "", signedIn: true, ...over };
}

function section(over: Partial<ServicesSectionProps> = {}) {
  const props: ServicesSectionProps = {
    services: [svc("contacts"), svc("calendar"), svc("tasks")],
    busy: null,
    errors: {},
    now,
    onToggle: vi.fn(),
    onSignIn: vi.fn(),
    ...over,
  };
  render(<ServicesSection {...props} />);
  return props;
}

const box = (name: string) => screen.getByRole("checkbox", { name }) as HTMLInputElement;

describe("ServicesSection", () => {
  it("lists the available services under a heading", () => {
    section({ services: [svc("contacts"), svc("calendar"), svc("tasks", { available: false })] });
    const region = screen.getByRole("region", { name: "Contacts, Calendars and Tasks" });
    expect(within(region).getByRole("heading", { level: 3 }).textContent).toBe("Contacts, Calendars and Tasks");
    expect(
      within(region)
        .getAllByRole("checkbox")
        .map((c) => c.closest("label")?.textContent),
    ).toEqual(["Contacts", "Calendars"]);
  });

  it("toggles a service", () => {
    const props = section({ services: [svc("contacts", { enabled: true }), svc("calendar"), svc("tasks")] });
    expect(box("Contacts").checked).toBe(true);
    expect(box("Calendars").checked).toBe(false);
    fireEvent.click(box("Calendars"));
    expect(props.onToggle).toHaveBeenLastCalledWith("calendar", true);
    fireEvent.click(box("Contacts"));
    expect(props.onToggle).toHaveBeenLastCalledWith("contacts", false);
  });

  it("shows each enabled service's state", () => {
    const today = new Date(2026, 9, 8, 9, 41);
    const before = new Date(2026, 9, 6, 9, 41);
    section({
      services: [
        svc("contacts", { enabled: true, lastSyncAt: today.toISOString() }),
        svc("calendar", { enabled: true, lastSyncAt: before.toISOString() }),
        svc("tasks", { enabled: true }),
      ],
    });
    const time = today.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
    const date = before.toLocaleDateString(undefined, { year: "2-digit", month: "numeric", day: "numeric" });
    expect(screen.getByText(`Synced at ${time}`)).toBeTruthy();
    expect(screen.getByText(`Synced ${date}`)).toBeTruthy();
    expect(screen.getByText("Waiting for the first sync").className).toContain("text-secondary");
  });

  it("shows maild's error, a busy service, and a missing sign-in", () => {
    const props = section({
      services: [
        svc("contacts", { enabled: true, error: "offline" }),
        svc("calendar", { enabled: true, signedIn: false }),
        svc("tasks"),
      ],
      busy: "tasks",
    });
    expect(screen.getByText("offline").className).toContain("text-flag-1");
    expect(screen.getByText("Waiting for Google sign-in").className).toContain("text-secondary");
    expect(screen.getByText("Sign in to Google to allow Calendars.").className).toContain("text-flag-1");
    fireEvent.click(screen.getByRole("button", { name: "Sign In…" }));
    expect(props.onSignIn).toHaveBeenCalled();
    expect(screen.getByText("Connecting…")).toBeTruthy();
    expect(box("Tasks").disabled).toBe(true);
  });

  it("asks for the server when a service could not be found", () => {
    const props = section({ errors: { calendar: "found no calendar server for ann@example.com; enter its address" } });
    expect(screen.getByRole("alert").textContent).toBe(
      "found no calendar server for ann@example.com; enter its address",
    );
    const field = screen.getByLabelText("Server:") as HTMLInputElement;
    expect(field.id).toBe("service-url-calendar");
    expect(field.getAttribute("type")).toBe("url");
    const connect = screen.getByRole("button", { name: "Connect" }) as HTMLButtonElement;
    expect(connect.disabled).toBe(true);
    fireEvent.change(field, { target: { value: "  https://dav.example.com/  " } });
    expect(connect.disabled).toBe(false);
    fireEvent.click(connect);
    expect(props.onToggle).toHaveBeenLastCalledWith("calendar", true, "https://dav.example.com/");
  });

  it("says so when the account has no services", () => {
    section({
      services: [
        svc("contacts", { available: false }),
        svc("calendar", { available: false }),
        svc("tasks", { available: false }),
      ],
    });
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
    expect(screen.getByText("This account has no contacts, calendars or tasks Frostmail can reach.")).toBeTruthy();
  });
});
