/** Filter a mailto link before handing it to another app (ADR-0056).
 * Non-mailto URLs pass through. Retained fields stay byte for byte. */
export function filterMailto(url: string): string | null {
  // Recognize the scheme as the browser does, including ignored leading
  // controls and embedded tabs/newlines. Only inspect the protocol; keep
  // the original address and fields below, without decoding or serializing.
  try {
    if (new URL(url).protocol !== "mailto:") return url;
  } catch {
    return url;
  }
  if (!/^mailto:/i.test(url) || [...url].some((c) => c.charCodeAt(0) <= 0x20 || c.charCodeAt(0) === 0x7f)) return null;
  const fragment = url.indexOf("#");
  const bare = fragment < 0 ? url : url.slice(0, fragment);
  const question = bare.indexOf("?");
  const address = bare.slice(7, question < 0 ? undefined : question);
  const fields =
    question < 0
      ? []
      : bare
          .slice(question + 1)
          .split("&")
          .filter((field) => /^(to|cc|bcc|subject|body)=/i.test(field));
  if (!address && fields.length === 0) return null;
  return `mailto:${address}${fields.length ? `?${fields.join("&")}` : ""}`;
}
