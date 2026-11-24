import { connectLodge } from "@frostyard/lodge-client";

async function openPreview() {
  const lodge = await connectLodge({ app: "mail" });
  Object.assign(window, {
    frostmailLodge: {
      contentURL: (path: string) => lodge.contentURL(path),
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
