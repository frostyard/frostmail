// The a11y test's afterEach hook (endLaunch): each step has a deadline, and an
// error or a hang in one step never leaves the single-instance app running for
// the next launch. The targets here are fakes: no app and no display are
// needed, which is why it runs fast in the same suite.
import { describe, expect, it } from "vitest";

import { type EndOptions, type EndTarget, endLaunch } from "./endLaunch";

/** FAST keeps the deadline tests short; the real ones are 15 s and 10 s. */
const FAST: EndOptions = { stepMs: 50, waitMs: 50 };

/** Fake is one launch and what endLaunch did to it. */
interface Fake {
  target: EndTarget;
  /** calls records the steps that ran, in order. */
  calls: string[];
  /** lines collects what endLaunch logged. */
  lines: string[];
}

/** fake makes a launch that closes its windows, stops and has ended, unless over says otherwise. */
function fake(over: Partial<EndTarget> = {}): Fake {
  const calls: string[] = [];
  const target: EndTarget = {
    closeOthers: () => {
      calls.push("closeOthers");
      return over.closeOthers?.() ?? Promise.resolve();
    },
    stop: () => {
      calls.push("stop");
      return over.stop?.() ?? Promise.resolve();
    },
    running: () => over.running?.() ?? false,
    end: () => {
      calls.push("end");
      return over.end?.() ?? [];
    },
  };
  return { target, calls, lines: [] };
}

/** run calls endLaunch on f and returns the error it threw, if any. */
async function run(f: Fake, options: EndOptions = FAST): Promise<unknown> {
  return endLaunch(f.target, (line) => f.lines.push(line), options).then(
    () => undefined,
    (err: unknown) => err,
  );
}

describe("endLaunch", () => {
  it("still stops the app and ends it when a window goes away", async () => {
    const gone = new Error("no such window: the window is closed");
    let running = true;
    const f = fake({
      closeOthers: () => Promise.reject(gone),
      running: () => running,
      end: () => {
        running = false;
        return ["4711"];
      },
    });
    expect(await run(f)).toBe(gone);
    expect(f.calls).toEqual(["closeOthers", "stop", "end"]);
    expect(f.lines).toEqual(["a11y: the app outlived its session; ended 4711"]);
  });

  it("stops the app when closing a window never settles", async () => {
    const f = fake({ closeOthers: () => new Promise<void>(() => {}), running: () => false });
    const error = await run(f);
    expect(f.calls).toEqual(["closeOthers", "stop"]);
    expect(String((error as Error).message)).toContain("closing the other windows");
    expect(f.lines).toEqual([]);
  });

  it("does not end an app that ends by itself", async () => {
    const f = fake();
    expect(await run(f)).toBeUndefined();
    expect(f.calls).toEqual(["closeOthers", "stop"]);
    expect(f.lines).toEqual([]);
  });

  it("ends an app that outlives its session, and names the pids", async () => {
    let running = true;
    const f = fake({
      running: () => running,
      end: () => {
        running = false;
        return ["1201", "1202"];
      },
    });
    expect(await run(f)).toBeUndefined();
    expect(f.calls).toEqual(["closeOthers", "stop", "end"]);
    expect(f.lines).toEqual(["a11y: the app outlived its session; ended 1201, 1202"]);
  });

  it("says nothing was ended when no process matches, and fails if the app stays", async () => {
    const f = fake({ running: () => true, end: () => [] });
    const error = await run(f);
    expect(String((error as Error).message)).toBe("the app did not end");
    expect(f.calls).toEqual(["closeOthers", "stop", "end"]);
    expect(f.lines).toEqual(["a11y: the app outlived its session; ended nothing"]);
  });

  it("collects the errors it meets and reports them together", async () => {
    const closed = new Error("no such window");
    const f = fake({
      closeOthers: () => Promise.reject(closed),
      stop: () => Promise.reject(new Error("the driver is gone")),
      running: () => true,
      end: () => [],
    });
    const error = await run(f);
    expect(error).toBeInstanceOf(AggregateError);
    expect((error as AggregateError).message).toBe("ending the launch failed");
    expect((error as AggregateError).errors).toHaveLength(3);
    expect((error as AggregateError).errors[0]).toBe(closed);
  });
});
