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
  const ogTitle = doc.querySelector('meta[property="og:title"]')?.getAttribute('content')?.trim();
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

/** How deep `hasJobPostingMarkup` walks a JSON-LD tree. Real markup nests a posting at
 *  most a few levels down (`@graph`, `mainEntity`); the bound only stops a pathological
 *  document from costing more than a glance. */
const MAX_LD_DEPTH = 8;

/**
 * Whether the page declares itself a job posting: a schema.org `JobPosting` in any of
 * its JSON-LD blocks, at the top level, in a list, under `@graph` or nested inside
 * another node. That markup is what search engines' job features read, so an employer's
 * own careers page carries it even where its URL names no ATS we recognise. A block that
 * does not parse is skipped, never fatal. Pure over its Document argument.
 */
export function hasJobPostingMarkup(doc: Document): boolean {
  for (const script of doc.querySelectorAll('script[type="application/ld+json"]')) {
    let data: unknown;
    try {
      data = JSON.parse(script.textContent ?? '');
    } catch {
      continue;
    }
    if (declaresJobPosting(data, 0)) return true;
  }
  return false;
}

function declaresJobPosting(node: unknown, depth: number): boolean {
  if (depth > MAX_LD_DEPTH || node === null || typeof node !== 'object') return false;
  if (Array.isArray(node)) return node.some((item) => declaresJobPosting(item, depth + 1));
  const record = node as Record<string, unknown>;
  const types: unknown[] = Array.isArray(record['@type']) ? record['@type'] : [record['@type']];
  if (types.some(isJobPostingType)) return true;
  return Object.values(record).some((value) => declaresJobPosting(value, depth + 1));
}

/** `JobPosting`, also written as a compact IRI (`schema:JobPosting`) or a full one. */
function isJobPostingType(type: unknown): boolean {
  return typeof type === 'string' && /(^|[/:#])JobPosting$/.test(type.trim());
}
