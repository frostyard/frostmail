import "./styles/app.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import type { Connect } from "./data/session";
import { IN_LODGE } from "./lodge/lodge";

// The lodge build reaches maild through lodge's shell (docs/specs/lodge-app.md).
// For development, VITE_MAILD_WS connects to maild's dev gateway and VITE_MOCK=1
// serves the UI from MockTransport (docs/design/app.md, Dev and test).
const gateway = import.meta.env.VITE_MAILD_WS as string | undefined;

async function connector(): Promise<Connect> {
  if (IN_LODGE) {
    const [{ connectLodge }, { startInLodge }] = await Promise.all([
      import("@frostyard/lodge-client"),
      import("./lodge/start"),
    ]);
    // Outside lodge's frame, connectLodge moves the page to its canonical
    // URL in the shell and never resolves.
    return startInLodge(() => connectLodge({ app: "mail" }));
  }
  if (gateway) return async () => (await import("./rpc/transport/ws")).connectWS(gateway);
  if (import.meta.env.VITE_MOCK === "1") {
    return async () => {
      const [{ MockTransport }, { mockData }] = await Promise.all([
        import("./rpc/mock/mock"),
        import("./rpc/mock/fixture"),
      ]);
      const rows = Number(import.meta.env.VITE_MOCK_ROWS ?? 60);
      return new MockTransport(mockData({ inbox: rows, invitation: true }), { latency: 15 });
    };
  }
  return async () => {
    throw new Error("no maild: build with --mode lodge, or set VITE_MAILD_WS or VITE_MOCK");
  };
}

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
void connector().then((connect) =>
  createRoot(root).render(
    <StrictMode>
      <App connect={connect} />
    </StrictMode>,
  ),
);
