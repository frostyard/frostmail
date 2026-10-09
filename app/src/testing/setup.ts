// Vitest setup for component tests (happy-dom, Testing Library). Globals are
// off, so cleanup is registered here.
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Testing Library prints the DOM with a failed query; keep it short enough
// that the failure itself stays in view.
process.env.DEBUG_PRINT_LIMIT ??= "1500";

// The main window's integration tests switch modules and wait for the
// next one's panes; with every worker busy that can take over the default
// second.
configure({ asyncUtilTimeout: 5000 });

afterEach(cleanup);
