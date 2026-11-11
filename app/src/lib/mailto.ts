/** Filter a mailto link before handing it to another app (ADR-0056).
 * Non-mailto URLs pass through. Retained fields stay byte for byte. */
export function filterMailto(url: string): string | null {
  // A browser ignores tabs/newlines and leading whitespace in schemes.
  // Recognize those forms too, then refuse them rather than bypass filtering.
  const colon = url.indexOf(":");
  if (colon < 0) return url;
  const scheme = url
    .slice(0, colon)
    .replace(/[\t\n\r]/g, "")
    .trimStart()
    .toLowerCase();
  if (scheme !== "mailto") return url;
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
