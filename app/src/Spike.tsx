// M0 go/no-go spike (docs/plans/0001-m0-foundations.md, step 6). Each check
// runs in the real WebKitGTK webview and reports pass/fail; the page is
// replaced by the real UI in M2.
import { invoke } from "@tauri-apps/api/core";
import { useEffect, useRef, useState } from "react";

import { Client, PROTOCOL, type Event } from "./rpc/gen/api";
import { connectTauri } from "./rpc/transport/tauri";

type Status = { ok: boolean | null; detail: string };
const pending: Status = { ok: null, detail: "running" };

// CSP violations seen by the page, for the report.
const violations: string[] = [];
document.addEventListener("securitypolicyviolation", (e) => {
  violations.push(`${e.violatedDirective} blocked ${e.blockedURI}`);
});

// A hostile message: a script, a remote image and a tracking pixel. In the
// sandboxed iframe, under the app CSP, none of them may run or load.
const HOSTILE_HTML = `<!doctype html><html><body style="font-family:sans-serif">
<h1 style="color:#c0392b">October deals</h1>
<p>Rendered in a sandboxed srcdoc iframe.</p>
<img src="https://shop.mailtest.test/hero.jpg" width="60" height="20" alt="remote">
<img src="https://track.mailtest.test/open.gif?u=1" width="1" height="1" alt="">
<script>parent.postMessage("script-ran", "*"); document.title = "pwned";</script>
<p style="background:url(https://track.mailtest.test/bg.gif)">CSS url()</p>
</body></html>`;

export function Spike() {
  const [hello, setHello] = useState<Status>(pending);
  const [subscribe, setSubscribe] = useState<Status>(pending);
  const [delivery, setDelivery] = useState<Status>(pending);
  const [events, setEvents] = useState<Event[]>([]);
  const [iframe, setIframe] = useState<Status>(pending);
  const [mailpart, setMailpart] = useState<Status>(pending);
  const [client, setClient] = useState<Client | null>(null);
  const [actionResult, setActionResult] = useState("");
  const frameRef = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const transport = await connectTauri();
        const seen: Event[] = [];
        transport.onEvent((e) => {
          seen.push(e);
          setEvents((prev) => [...prev, e]);
        });
        const c = new Client(transport);
        const h = await c.rpc.hello({ protocol: PROTOCOL, client: "frostmail-spike" });
        if (cancelled) return;
        setClient(c);
        setHello({ ok: true, detail: `${h.server}, protocol ${h.protocol}` });
        const sub = await c.events.subscribe({ sinceSeq: 0 });
        setSubscribe({ ok: true, detail: `seq ${sub.seq}, resync ${sub.resync}` });
        // A write that emits a durable event, which must come back over the
        // bridge's channel (fails until task T-0002 implements the store).
        try {
          const a = await c.account.create(spikeAccount());
          const deadline = Date.now() + 3000;
          while (!seen.some((e) => e.event === "account.changed" && e.data.id === a.id) && Date.now() < deadline) {
            await new Promise((r) => setTimeout(r, 50));
          }
          const got = seen.find((e) => e.event === "account.changed" && e.data.id === a.id);
          setDelivery(got ? { ok: true, detail: `account.changed seq ${got.seq} for account ${a.id}` } : { ok: false, detail: "no event within 3s" });
        } catch (err) {
          setDelivery({ ok: false, detail: String(err) });
        }
      } catch (err) {
        setHello({ ok: false, detail: String(err) });
        setSubscribe({ ok: false, detail: "no connection" });
        setDelivery({ ok: false, detail: "no connection" });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let scriptRan = false;
    const onMessage = (e: MessageEvent) => {
      if (e.data === "script-ran") scriptRan = true;
    };
    window.addEventListener("message", onMessage);
    const t = setTimeout(() => {
      const win = frameRef.current?.contentWindow;
      const requests = win ? win.performance.getEntriesByType("resource").map((r) => r.name) : ["no access"];
      const titled = frameRef.current?.contentDocument?.title === "pwned";
      const ok = !scriptRan && !titled && requests.length === 0;
      setIframe({
        ok,
        detail: `script ran: ${scriptRan || titled}; network requests: ${requests.length}${requests.length ? ` (${requests.join(", ")})` : ""}`,
      });
    }, 1500);
    return () => {
      clearTimeout(t);
      window.removeEventListener("message", onMessage);
    };
  }, []);

  // Report once every automatic check has settled, with renderer facts.
  const reported = useRef(false);
  useEffect(() => {
    const checks = { hello, subscribe, delivery, iframe, mailpart };
    if (reported.current || Object.values(checks).some((c) => c.ok === null)) return;
    reported.current = true;
    void (async () => {
      const report = { checks, violations, origin: location.origin, fps: await measureFps(), dpr: window.devicePixelRatio, ua: navigator.userAgent };
      await invoke("spike_report", { report });
    })();
  }, [hello, subscribe, delivery, iframe, mailpart]);

  const createAccount = async () => {
    if (!client) return;
    try {
      const a = await client.account.create(spikeAccount());
      setActionResult(`created account ${a.id}`);
    } catch (err) {
      setActionResult(String(err));
    }
  };

  return (
    <main style={{ font: "14px system-ui, sans-serif", padding: 24, maxWidth: 900 }}>
      <h1 style={{ fontSize: 20 }}>Frostmail M0 spike</h1>
      <p style={{ color: "#666" }}>{navigator.userAgent}</p>
      <Check name="rpc.hello over the Tauri bridge" s={hello} />
      <Check name="events.subscribe" s={subscribe} />
      <Check name="durable event delivered over the channel" s={delivery} />
      <Check name="sandboxed srcdoc iframe: no scripts, no network" s={iframe} />
      <Check name="mailpart:// protocol" s={mailpart} />
      <section style={{ margin: "16px 0" }}>
        <button onClick={createAccount} disabled={!client}>
          account.create (emits account.changed)
        </button>{" "}
        <span>{actionResult}</span>
        <pre style={{ background: "#f4f4f4", padding: 8, minHeight: 40 }}>
          {events.map((e) => JSON.stringify(e)).join("\n") || "no events yet"}
        </pre>
      </section>
      <img
        src="mailpart://localhost/spike/frost.png"
        width={48}
        height={48}
        alt="mailpart test"
        onLoad={() => setMailpart({ ok: true, detail: "served spike/frost.png from the parts cache" })}
        onError={() => setMailpart({ ok: false, detail: "spike/frost.png did not load; run make spike-assets" })}
      />
      <iframe
        ref={frameRef}
        title="message"
        sandbox="allow-same-origin"
        srcDoc={HOSTILE_HTML}
        style={{ width: "100%", height: 220, border: "1px solid #ccc", marginTop: 12 }}
      />
    </main>
  );
}

function spikeAccount() {
  const email = `spike${Date.now()}${Math.floor(Math.random() * 1000)}@mailtest.test`;
  return {
    kind: "imap" as const,
    email,
    displayName: "Spike",
    auth: "password" as const,
    imap: { host: "mailtest.test", port: 993, tls: "tls" as const, username: email },
    smtp: { host: "mailtest.test", port: 587, tls: "starttls" as const, username: email },
  };
}

function measureFps(): Promise<number> {
  return new Promise((resolve) => {
    let frames = 0;
    const start = performance.now();
    const tick = (now: number) => {
      frames++;
      if (now - start < 1000) requestAnimationFrame(tick);
      else resolve(Math.round((frames * 1000) / (now - start)));
    };
    requestAnimationFrame(tick);
  });
}

function Check({ name, s }: { name: string; s: Status }) {
  const mark = s.ok === null ? "…" : s.ok ? "PASS" : "FAIL";
  const color = s.ok === null ? "#888" : s.ok ? "#1e7d32" : "#c62828";
  return (
    <div data-check={name} data-ok={String(s.ok)} style={{ margin: "6px 0" }}>
      <strong style={{ color, display: "inline-block", width: 48 }}>{mark}</strong> {name}
      <span style={{ color: "#555" }}> — {s.detail}</span>
    </div>
  );
}
