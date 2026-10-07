import "./styles/app.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import type { Connect } from "./data/session";

// VITE_MOCK=1 serves the UI from MockTransport (docs/design/app.md, Dev and
// test); otherwise the Tauri bridge connects to maild.
const connect: Connect =
  import.meta.env.VITE_MOCK === "1"
    ? async () => {
        const [{ MockTransport }, { mockData }] = await Promise.all([
          import("./rpc/mock/mock"),
          import("./rpc/mock/fixture"),
        ]);
        const rows = Number(import.meta.env.VITE_MOCK_ROWS ?? 60);
        return new MockTransport(mockData({ inbox: rows }), { latency: 15 });
      }
    : async () => (await import("./rpc/transport/tauri")).connectTauri();

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
createRoot(root).render(
  <StrictMode>
    <App connect={connect} />
  </StrictMode>,
);
