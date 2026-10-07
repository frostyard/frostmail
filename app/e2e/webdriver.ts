// A minimal W3C WebDriver client for driving the built app through
// WebKitWebDriver (docs/design/app.md, Dev and test). Only the endpoints the
// app tests use.

const ELEMENT = "element-6066-11e4-a52e-4f735466cecf";

/** Element is a handle to an element in the session's page. */
export interface Element {
  readonly id: string;
}

interface Reply<T> {
  value: T & { error?: string; message?: string };
}

/** WebDriverError is an error reply from the driver. */
export class WebDriverError extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(`${code}: ${message}`);
  }
}

async function call<T>(base: string, method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(base + path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const reply = (await res.json()) as Reply<T>;
  if (!res.ok || reply.value?.error) {
    throw new WebDriverError(reply.value?.error ?? String(res.status), reply.value?.message ?? res.statusText);
  }
  return reply.value;
}

/** Session is one WebDriver session: one running app. */
export class Session {
  private constructor(
    private readonly base: string,
    readonly id: string,
  ) {}

  /** start creates a session; capabilities go into alwaysMatch. */
  static async start(driverURL: string, capabilities: Record<string, unknown>): Promise<Session> {
    const v = await call<{ sessionId: string }>(driverURL, "POST", "/session", {
      capabilities: { alwaysMatch: capabilities },
    });
    return new Session(driverURL, v.sessionId);
  }

  private do<T>(method: string, path: string, body?: unknown): Promise<T> {
    return call<T>(this.base, method, `/session/${this.id}${path}`, body);
  }

  /** quit ends the session and closes the app. */
  async quit(): Promise<void> {
    await this.do("DELETE", "");
  }

  /** execute runs a synchronous script in the page and returns its result. */
  execute<T>(script: string, ...args: unknown[]): Promise<T> {
    return this.do<T>("POST", "/execute/sync", { script, args });
  }

  /** executeAsync runs a script whose last argument is a done callback. */
  executeAsync<T>(script: string, ...args: unknown[]): Promise<T> {
    return this.do<T>("POST", "/execute/async", { script, args });
  }

  /** find returns the first element matching a CSS selector. */
  async find(css: string): Promise<Element> {
    const v = await this.do<Record<string, string>>("POST", "/element", { using: "css selector", value: css });
    return { id: v[ELEMENT] ?? "" };
  }

  /** findAll returns every element matching a CSS selector. */
  async findAll(css: string): Promise<Element[]> {
    const v = await this.do<Record<string, string>[]>("POST", "/elements", { using: "css selector", value: css });
    return v.map((e) => ({ id: e[ELEMENT] ?? "" }));
  }

  /** click clicks an element. */
  async click(el: Element): Promise<void> {
    await this.do("POST", `/element/${el.id}/click`, {});
  }

  /** text returns an element's rendered text. */
  text(el: Element): Promise<string> {
    return this.do<string>("GET", `/element/${el.id}/text`);
  }

  /** screenshot returns the window as base64 PNG. */
  screenshot(): Promise<string> {
    return this.do<string>("GET", "/screenshot");
  }

  /** waitFor polls fn until it returns a truthy value or timeoutMs passes. */
  async waitFor<T>(what: string, fn: () => Promise<T>, timeoutMs = 10_000): Promise<NonNullable<T>> {
    const deadline = Date.now() + timeoutMs;
    let last: unknown;
    for (;;) {
      try {
        const v = await fn();
        if (v) return v as NonNullable<T>;
      } catch (err) {
        last = err;
      }
      if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}${last ? `: ${String(last)}` : ""}`);
      await new Promise((r) => setTimeout(r, 100));
    }
  }
}
