/**
 * Where the agent's reading stops.
 *
 * `read_page` serves whatever tab is active, and what comes back is persisted
 * verbatim into the conversation's transcript. This extension is the only side
 * that sees the url before the page is read — by the time hire could judge it, it
 * is already holding the page — so the boundary belongs here.
 *
 * In practice a content script declared on `<all_urls>` is already not injected
 * into `chrome://`, the Web Store or (absent an explicit opt-in) `file://`, so
 * those tabs fail today. They fail by accident, with a message that describes the
 * symptom. Deciding it here makes the refusal deliberate and explainable, and stops
 * the boundary depending on Chrome continuing to read `<all_urls>` that way.
 */

/** Schemes an ordinary web page is served over. Everything else — the browser's
 *  own pages, other extensions, the user's filesystem — is out of reach on
 *  purpose. */
const READABLE_PROTOCOLS = ['http:', 'https:'];

/** Whether the agent may read the page at this url. A url it cannot parse is not
 *  a page: a tab that has not committed one yet reports an empty string. */
export function isReadablePageUrl(url: string | undefined): boolean {
  if (!url) return false;
  try {
    return READABLE_PROTOCOLS.includes(new URL(url).protocol);
  } catch {
    return false;
  }
}

/** What the model is told when the boundary refuses. It states the rule rather
 *  than the symptom, because the model's only way to help is to explain it. */
export const NOT_A_WEB_PAGE =
  'I can only read ordinary web pages (http/https). This tab is a browser or local page, so there is nothing here I can read.';
