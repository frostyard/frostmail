import { connectLodge } from "@frostyard/lodge-client";

async function openPreview() {
  const lodge = await connectLodge({ app: "mail" });
  let receive: ((route: string) => void) | undefined;
  const location = lodge.history.location;
  let current = location.pathname + location.search + location.hash;
  lodge.on("navigate", ({ route }) => {
    current = route;
    receive?.(route);
  });
  Object.assign(window, {
    frostmailLodge: {
      onRoute(callback?: (route: string) => void) {
        receive = callback;
        return current;
      },
      report(route: string, title: string, replace: boolean) {
        if (route !== current) lodge.route(route, title, { replace });
        current = route;
      },
      printMessage(subject: string, headers: string, text: string) {
        document.getElementById("mail-print")?.remove();
        const printable = document.createElement("article");
        printable.id = "mail-print";
        for (const [tag, value] of [
          ["h1", subject],
          ["pre", headers],
          ["pre", text],
        ] as const) {
          const element = document.createElement(tag);
          element.textContent = value;
          printable.append(element);
        }
        document.body.append(printable);
        window.addEventListener("afterprint", () => printable.remove(), { once: true });
        window.print();
      },
      contentURL: (path: string) => lodge.contentURL(path),
      openPart: (path: string) => lodge.open(lodge.contentURL(path)),
      download: (path: string) => lodge.open(`${lodge.contentURL(path)}?download=1`),
    },
  });
  const base = new URL("/app/mail/", window.location.origin);
  const assets = new URL("assets/", base);
  const response = await fetch(new URL("nsgo.json", base));
  if (!response.ok) throw new Error(`Could not load the mail configuration (HTTP ${response.status})`);
  const config = await response.json();
  const loader = new URL(config.loader, assets);
  const { mount } = await import(/* @vite-ignore */ loader.href);
  const player = await mount(null, assets.href, { config, appearance: lodge.theme });
  player.setAppearance(lodge.theme);
  lodge.on("theme", ({ scheme }) => player.setAppearance(scheme));
}

openPreview().catch((error: unknown) => {
  const message = document.createElement("p");
  message.setAttribute("role", "alert");
  message.textContent = `Could not open FrostMail: ${error instanceof Error ? error.message : String(error)}`;
  document.body.append(message);
});
