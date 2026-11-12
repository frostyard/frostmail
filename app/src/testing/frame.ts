// Frame helpers shared by the window's integration tests.
import { onTestFinished, vi } from "vitest";

import { useFrameWindow, windowRoute } from "../lodge/frameWindow";

/**
 * frameRoute is the route of the window the frame shows, or "" when the
 * frame shows the main window (lodge/frameWindow.ts).
 */
export function frameRoute(): string {
  const w = useFrameWindow.getState().open;
  return w === null ? "" : windowRoute(w);
}

/**
 * closeWatch is a spy that counts the times the frame goes back to the main
 * window: what a window's Close does (lodge/frameWindow.ts). Put the window
 * in the frame first, or there is nothing to go back from.
 */
export function closeWatch() {
  const closes = vi.fn();
  const stop = useFrameWindow.subscribe((state, previous) => {
    if (state.open === null && previous.open !== null) closes();
  });
  onTestFinished(stop);
  return closes;
}
