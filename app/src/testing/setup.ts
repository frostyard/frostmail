// Vitest setup for component tests (happy-dom, Testing Library). Globals are
// off, so cleanup is registered here.
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(cleanup);
