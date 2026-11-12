import "./styles/app.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import type { Connect } from "./data/session";

// The build reaches maild only through lodge's shell: main.tsx connects
// through @frostyard/lodge-client and never to maild's socket itself
// (docs/specs/lodge-app.md, The web build).
async function connector(): Promise<Connect> {
  const [{ connectLodge }, { startInLodge }] = await Promise.all([
    import("@frostyard/lodge-client"),
    import("./lodge/start"),
  ]);
  // Outside lodge's frame, connectLodge moves the page to its canonical URL
  // in the shell and never resolves.
  return startInLodge(() => connectLodge({ app: "mail" }));
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
