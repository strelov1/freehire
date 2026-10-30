import { countryLabel } from '#lib/facets.js';

// Words a location line uses to state the work arrangement rather than a place. The card
// already says that through its work-mode chip and its flags, so they never earn the
// location caption a second time.
const ARRANGEMENT_WORDS = new Set([
  'remote',
  'anywhere',
  'worldwide',
  'global',
  'hybrid',
  'onsite',
  'on-site',
  'on site',
  'fully remote',
  '100% remote',
]);

// Spellings of a country that are neither its ISO code nor the name the flag shows, but
// that location lines use constantly: "New York, USA" must lose its "USA" the way
// "Berlin, Germany" loses "Germany".
const COUNTRY_ALIASES: Record<string, string[]> = {
  us: ['usa', 'u.s.', 'u.s.a.', 'united states of america'],
  gb: ['uk', 'u.k.'],
};

/**
 * The part of a posting's location line the browse card has not already said. The card
 * names the country beside its flag and the arrangement in a chip, so seven postings of
 * one employer in seven cities all read "Croatia" until opened (freehire#3894) — the city
 * is the only part missing. Strips the segments that repeat a shown country (by name,
 * code or common alias) or an arrangement word — including inside a trailing
 * parenthetical, so "Remote (US)" says nothing new while "Zagreb (Sesvete)" stays whole —
 * and returns null when nothing is left, so a bare "Remote" or "Germany" adds no caption.
 */
export function cardLocation(location: string | undefined, countries: string[] | undefined): string | null {
  const shown = new Set<string>(ARRANGEMENT_WORDS);
  for (const code of countries ?? []) {
    const lower = code.toLowerCase();
    shown.add(lower);
    shown.add(countryLabel(code).toLowerCase());
    for (const alias of COUNTRY_ALIASES[lower] ?? []) shown.add(alias);
  }
  const adds = (text: string) => text !== '' && !shown.has(text.toLowerCase());

  const parts: string[] = [];
  for (const segment of (location ?? '').split(/\s*[,;|]\s*|\s+[-–]\s+/)) {
    const m = /^(.*?)\s*\(([^)]*)\)$/.exec(segment.trim());
    const head = (m?.[1] ?? segment).trim();
    const qualifier = (m?.[2] ?? '').trim();
    if (adds(head) && adds(qualifier)) parts.push(`${head} (${qualifier})`);
    else if (adds(head)) parts.push(head);
    else if (adds(qualifier)) parts.push(qualifier);
  }
  return parts.length > 0 ? parts.join(', ') : null;
}
