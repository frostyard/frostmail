// What a window with no maild says (App.tsx).
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useMail } from "../data/stores";
import { Connecting } from "./App";

const button = () => screen.queryByRole("button", { name: "Server Settings…" });

describe("a window with no maild", () => {
  it("says it is connecting, and offers nothing, while the first connection is made", () => {
    useMail.setState({ connection: { state: "connecting" } });
    render(<Connecting />);
    expect(screen.getByText("Connecting to maild…")).toBeTruthy();
    expect(button()).toBeNull();
  });

  it("says why it is reconnecting, and offers nothing", () => {
    useMail.setState({ connection: { state: "lost", reason: "no maild" } });
    render(<Connecting />);
    expect(screen.getByText("Reconnecting to maild… (no maild)")).toBeTruthy();
    expect(button()).toBeNull();
  });
});
