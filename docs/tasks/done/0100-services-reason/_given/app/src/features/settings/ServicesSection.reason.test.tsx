// CONTRACT TEST for task card T-0100 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ServiceKind, ServiceSettings } from "../../rpc/gen/api";
import { ServicesSection, type ServicesSectionProps } from "./ServicesSection";

const reminders = "Apple Reminders can't be reached by apps outside Apple's (since iOS 13).";

function svc(service: ServiceKind, over: Partial<ServiceSettings> = {}): ServiceSettings {
  return { service, available: true, enabled: false, url: "", signedIn: true, ...over };
}

function section(services: ServiceSettings[], over: Partial<ServicesSectionProps> = {}) {
  const props: ServicesSectionProps = {
    services,
    busy: null,
    errors: {},
    now: new Date(2026, 9, 10, 12, 0),
    onToggle: vi.fn(),
    onSignIn: vi.fn(),
    ...over,
  };
  render(<ServicesSection {...props} />);
}

describe("ServicesSection reasons", () => {
  it("says why a service is out of reach", () => {
    section([
      svc("contacts"),
      svc("calendar", { enabled: true }),
      svc("tasks", { available: false, reason: reminders }),
    ]);
    const p = screen.getByText(reminders);
    expect(p.tagName).toBe("P");
    expect(p.className).toContain("text-secondary");
    expect(p.className).toContain("mt-2");
    expect(screen.queryByRole("checkbox", { name: "Tasks" })).toBeNull();
    expect(screen.getByRole("region", { name: "Contacts, Calendars and Tasks" }).contains(p)).toBe(true);
  });

  it("says nothing for a service out of reach without a reason", () => {
    const { container } = render(
      <ServicesSection
        services={[svc("contacts"), svc("calendar"), svc("tasks", { available: false })]}
        busy={null}
        errors={{}}
        now={new Date(2026, 9, 10, 12, 0)}
        onToggle={vi.fn()}
        onSignIn={vi.fn()}
      />,
    );
    expect(container.querySelectorAll("p")).toHaveLength(0);
  });

  it("follows the no-service paragraph with the reasons it has", () => {
    section([
      svc("contacts", { available: false }),
      svc("calendar", { available: false }),
      svc("tasks", { available: false, reason: "No tasks here." }),
    ]);
    const paragraphs = Array.from(document.querySelectorAll("p")).map((p) => p.textContent);
    expect(paragraphs).toEqual([
      "This account has no contacts, calendars or tasks Frostmail can reach.",
      "No tasks here.",
    ]);
  });
});
