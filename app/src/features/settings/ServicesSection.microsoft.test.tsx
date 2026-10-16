// A Microsoft account's services wait for a Microsoft sign-in (plan 0010,
// Phase 3): Graph's contacts and calendars, and To Do's reason.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ServiceSettings } from "../../rpc/gen/api";
import { ServicesSection } from "./ServicesSection";

const services: ServiceSettings[] = [
  { service: "contacts", available: true, enabled: true, url: "https://graph.microsoft.com/v1.0/", signedIn: false },
  { service: "calendar", available: true, enabled: false, url: "", signedIn: false },
  {
    service: "tasks",
    available: false,
    reason: "Microsoft To Do isn't supported yet.",
    enabled: false,
    url: "",
    signedIn: true,
  },
];

describe("ServicesSection for Microsoft", () => {
  it("asks for a Microsoft sign-in", () => {
    render(
      <ServicesSection
        services={services}
        busy={null}
        errors={{}}
        now={new Date()}
        onToggle={vi.fn()}
        onSignIn={vi.fn()}
        provider="Microsoft"
      />,
    );
    expect(screen.getByText("Waiting for Microsoft sign-in")).toBeTruthy();
    expect(screen.getByText("Sign in to Microsoft to allow Contacts.")).toBeTruthy();
    expect(screen.getByText("Microsoft To Do isn't supported yet.")).toBeTruthy();
  });
});
