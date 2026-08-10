import type { PageSnapshot } from './protocol';

/** How much visible text we keep. Enough for context, small enough to send. */
const MAX_TEXT_LENGTH = 5000;

/**
 * Reads the given document into a PageSnapshot.
 *
 * Pure over its Document argument so it can be tested without a browser.
 */
export function extractSnapshot(doc: Document): PageSnapshot {
  return {
    url: doc.location?.href ?? '',
    title: doc.title,
    headline: extractHeadline(doc),
    text: extractVisibleText(doc),
  };
}

/**
 * Best-effort primary heading of an arbitrary page.
 *
 * Heuristic, in priority order: an explicit og:title (what the site itself
 * calls the page), then the first <h1>, then the document title as a floor.
 * This is the natural seam to make smarter per job board later.
 */
export function extractHeadline(doc: Document): string {
  const ogTitle = doc
    .querySelector('meta[property="og:title"]')
    ?.getAttribute('content')
    ?.trim();
  if (ogTitle) return ogTitle;

  const h1 = doc.querySelector('h1')?.textContent?.trim();
  if (h1) return h1;

  return doc.title.trim();
}

function extractVisibleText(doc: Document): string {
  const body = doc.body;
  if (!body) return '';
  // innerText reflects what the user actually sees; textContent is the
  // fallback for engines (jsdom, older happy-dom) that don't implement it.
  const raw = body.innerText || body.textContent || '';
  return raw.replace(/\s+/g, ' ').trim().slice(0, MAX_TEXT_LENGTH);
}
