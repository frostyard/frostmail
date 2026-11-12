import { expect, it } from "vitest";
import { moved, resized } from "./eventDrag";

it.each([
  ["2026-11-01T06:30:12.345Z", "2026-11-01T06:35:56.789Z"],
  ["2026-10-08T14:00:12.345Z", "2026-10-08T15:30:56.789Z"],
])("zero-delta drags preserve exact instants at %s", (start, end) => {
  const range = { start, end };
  expect(moved(range, 0, 0, "America/New_York")).toEqual(range);
  expect(resized(range, 0, "America/New_York")).toEqual(range);
});
