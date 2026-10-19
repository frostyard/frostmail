import "./styles/app.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import type { Connect } from "./data/session";
import { IN_LODGE } from "./lodge/lodge";

// VITE_MOCK=1 serves the UI from MockTransport and VITE_MAILD_WS from maild's
// dev gateway (docs/design/app.md, Dev and test); the lodge build reaches
// maild through lodge's shell (docs/specs/lodge-app.md); otherwise the
// Tauri bridge connects to maild.
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
  return async () => (await import("./rpc/transport/tauri")).connectTauri();
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
