// CONTRACT TEST for task card T-0093 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ServiceKind, ServiceSettings } from "../../rpc/gen/api";
import { ServicesSection, type ServicesSectionProps } from "./ServicesSection";

function svc(service: ServiceKind, over: Partial<ServiceSettings> = {}): ServiceSettings {
  return { service, available: true, enabled: false, url: "", signedIn: true, ...over };
}

function section(services: ServiceSettings[]) {
  const props: ServicesSectionProps = {
    services,
    busy: null,
    errors: {},
    now: new Date(2026, 9, 8, 15, 0),
    onToggle: vi.fn(),
    onSignIn: vi.fn(),
  };
  render(<ServicesSection {...props} />);
  return props;
}

describe("ServicesSection's one sign-in", () => {
  it("asks once for every service waiting for Google", () => {
    const props = section([
      svc("contacts", { enabled: true, signedIn: false }),
      svc("calendar", { enabled: true, signedIn: false }),
      svc("tasks", { enabled: true, signedIn: false }),
    ]);
    expect(screen.getAllByText("Waiting for Google sign-in")).toHaveLength(3);
    expect(screen.getByText("Sign in to Google to allow Contacts, Calendars and Tasks.")).toBeTruthy();
    const buttons = screen.getAllByRole("button", { name: "Sign In…" });
    expect(buttons).toHaveLength(1);
    fireEvent.click(buttons[0] as HTMLElement);
    expect(props.onSignIn).toHaveBeenCalledTimes(1);
  });

  it("names two services with and", () => {
    section([
      svc("contacts", { enabled: true, signedIn: false }),
      svc("calendar"),
      svc("tasks", { enabled: true, signedIn: false }),
    ]);
    expect(screen.getByText("Sign in to Google to allow Contacts and Tasks.")).toBeTruthy();
  });

  it("asks nothing for a service that is off or signed in", () => {
    section([svc("contacts", { signedIn: false }), svc("calendar", { enabled: true }), svc("tasks")]);
    expect(screen.queryByText(/^Sign in to Google/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Sign In…" })).toBeNull();
  });
});
