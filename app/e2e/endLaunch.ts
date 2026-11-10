// Ending one launch before the next (a11y.e2e.ts). The app is
// single-instance (over D-Bus, as in CI), so a launch that does not end takes
// the next one. Every step has a deadline, and no error skips the steps after
// it: a WebDriver call that rejects or hangs cannot leave the app running.
// These tests check the views, not how the app ends (firstrun.e2e.ts does).

/** EndTarget is one launch, as endLaunch ends it. */
export interface EndTarget {
  /** closeOthers closes the windows a test opened (compose, settings, a reminder). */
  closeOthers(): Promise<void>;
  /** stop ends the session and the processes the harness started. */
  stop(): Promise<void>;
  /** running reports whether this launch's app process still runs. */
  running(): boolean;
  /** end sends the app SIGTERM and returns the pids it signalled. */
  end(): string[];
}

/** EndOptions are endLaunch's deadlines, short in its own test. */
export interface EndOptions {
  /** stepMs is the deadline of closeOthers and of stop (default 15 s). */
  stepMs?: number;
  /** waitMs is how long to wait for the app to end, before and after end (default 10 s). */
  waitMs?: number;
}

/** STEP_MS and WAIT_MS bound each step and each wait; POLL_MS is the poll interval. */
const STEP_MS = 15_000;
const WAIT_MS = 10_000;
const POLL_MS = 100;

/**
 * within settles as promise does, and rejects with an Error naming what after
 * ms; the timer is cleared as soon as the promise settles.
 */
function within<T>(ms: number, promise: Promise<T>, what: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_, fail) => {
    timer = setTimeout(() => fail(new Error(`${what} did not finish within ${ms} ms`)), ms);
  });
  return Promise.race([promise, deadline]).finally(() => clearTimeout(timer));
}

/**
 * gone reports whether the app ends within waitMs. It polls every POLL_MS, and
 * never past the deadline, so a short waitMs in a test stays short.
 */
async function gone(target: EndTarget, waitMs: number): Promise<boolean> {
  const deadline = Date.now() + waitMs;
  while (target.running()) {
    const left = deadline - Date.now();
    if (left <= 0) return false;
    await new Promise((resolve) => setTimeout(resolve, Math.min(POLL_MS, left)));
  }
  return true;
}

/**
 * endLaunch closes the launch's other windows, stops it, and waits for its app
 * to end. An app that outlives its session is ended, so it cannot take the
 * next launch, and log says so. A cleanup error is thrown at the end, alone or
 * as an AggregateError, but only after the process cleanup ran.
 */
export async function endLaunch(
  target: EndTarget,
  log: (line: string) => void,
  options: EndOptions = {},
): Promise<void> {
  const stepMs = options.stepMs ?? STEP_MS;
  const waitMs = options.waitMs ?? WAIT_MS;
  const errors: unknown[] = [];
  const steps: [string, () => Promise<void>][] = [
    ["closing the other windows", () => target.closeOthers()],
    ["stopping the launch", () => target.stop()],
  ];
  for (const [what, step] of steps) {
    await within(stepMs, step(), what).catch((err: unknown) => errors.push(err));
  }
  if (!(await gone(target, waitMs))) {
    const ended = target.end();
    log(`a11y: the app outlived its session; ended ${ended.join(", ") || "nothing"}`);
    if (!(await gone(target, waitMs))) errors.push(new Error("the app did not end"));
  }
  const [first, ...rest] = errors;
  if (first === undefined) return;
  if (rest.length > 0) throw new AggregateError(errors, "ending the launch failed");
  throw first;
}
