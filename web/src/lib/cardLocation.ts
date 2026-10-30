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

/**
 * The part of a posting's location line the browse card has not already said. The card
 * names the country beside its flag and the arrangement in a chip, so seven postings of
 * one employer in seven cities all read "Croatia" until opened (freehire#3894) — the city
 * is the only part missing. Strips the segments that repeat a shown country (by name or
 * code) or an arrangement word, and returns null when nothing is left, so a bare
 * "Remote" or "Germany" adds no caption at all.
 */
export function cardLocation(location: string | undefined, countries: string[] | undefined): string | null {
  const shown = new Set<string>();
  for (const code of countries ?? []) {
    shown.add(code.toLowerCase());
    shown.add(countryLabel(code).toLowerCase());
  }
  const parts = (location ?? '')
    .split(/\s*[,;|]\s*|\s+[-–]\s+/)
    .map((p) => p.trim())
    .filter((p) => {
      const key = p.toLowerCase();
      return key !== '' && !shown.has(key) && !ARRANGEMENT_WORDS.has(key);
    });
  return parts.length > 0 ? parts.join(', ') : null;
}
