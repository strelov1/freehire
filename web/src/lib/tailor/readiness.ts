// Whether a tailored CV is ready to leave the workspace. The PDF is the thing the seeker
// actually sends, so the checks here guard the download rather than the editing.
import type { Document } from '#lib/generated/contracts.js';

/** A template slot still sitting in the CV, and the address of the field that holds it. */
export type Placeholder = { path: string; token: string };

// A slot is a short bracketed phrase with at least one letter in it: `[purpose]`,
// `[Your Name]`, `[X%]`. Digits-only pairs are citations or years, a pair followed by `(`
// is a markdown link, and anything longer than a few words is an aside, not a slot.
const SLOT = /\[([^[\]\n]{0,40})\](?!\()/g;
const HAS_LETTER = /\p{L}/u;

// Margins and typography hold numbers set through controls, never text the candidate or the
// agent wrote, so a slot cannot live there.
const LAYOUT_KEYS = new Set(['margins', 'style']);

/**
 * findPlaceholders lists every unfilled template slot in the document, in reading order.
 *
 * Paths use the addresses revisions use (`experience[1].bullets[0]`), so the preview's
 * existing highlighter can point at them without a second addressing scheme.
 */
export function findPlaceholders(doc: Document): Placeholder[] {
  const found: Placeholder[] = [];
  const walk = (value: unknown, path: string) => {
    if (typeof value === 'string') {
      for (const match of value.matchAll(SLOT)) {
        if (HAS_LETTER.test(match[1] ?? '')) found.push({ path, token: match[0] });
      }
    } else if (Array.isArray(value)) {
      value.forEach((item, i) => walk(item, `${path}[${i}]`));
    } else if (value && typeof value === 'object') {
      for (const [key, child] of Object.entries(value)) {
        if (path === '' && LAYOUT_KEYS.has(key)) continue;
        walk(child, path === '' ? key : `${path}.${key}`);
      }
    }
  };
  walk(doc, '');
  return found;
}
