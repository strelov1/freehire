import { ISO_COUNTRY_CODES, countryLabel } from './facets';

/** Turn an English display name into a URL segment: strip diacritics, lowercase, and
 *  collapse every run of non-alphanumerics into one hyphen. Local to the country
 *  index — a general slugifier would invite callers whose input is not a name Intl
 *  produced, and the guarantees below (injective, round-tripping) only hold for that. */
function slugifyName(name: string): string {
  return name
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

/** A country that has earned a URL segment: its ISO code and the slug it resolves by. */
export interface SlugifiedCountry {
  code: string;
  slug: string;
}

// The bidirectional country↔slug index, built once from ISO_COUNTRY_CODES. The
// landing routes address a country by its slugified English name ('germany'), never
// by ISO code — the code is not a phrase anyone searches. Intl gives the forward
// direction and no reverse one, so the reverse is derived here rather than in a
// hand-kept table that could disagree with countryLabel.
//
// Two codes are refused a slug:
//   - one countryLabel only ECHOES (it returns the uppercased code when Intl has no
//     name — a runtime whose ICU predates a code); an echo is not a name, and indexing it
//     would mint a URL out of a two-letter non-word.
//   - both members of any pair that slugify alike. Handing the slug to whichever
//     came first in the list would be an arbitrary choice made silently, so the
//     collision drops the whole group and the ambiguity stays visible as an absence.
const COUNTRY_SLUGS: { byCode: Map<string, string>; bySlug: Map<string, string>; list: SlugifiedCountry[] } = (() => {
  const claims = new Map<string, string[]>();
  for (const code of ISO_COUNTRY_CODES) {
    const label = countryLabel(code);
    if (label === code.toUpperCase()) continue;
    const slug = slugifyName(label);
    if (!slug) continue;
    claims.set(slug, [...(claims.get(slug) ?? []), code]);
  }

  const byCode = new Map<string, string>();
  const bySlug = new Map<string, string>();
  const list: SlugifiedCountry[] = [];
  for (const [slug, codes] of claims) {
    const [code, ...rest] = codes;
    if (!code || rest.length > 0) continue;
    byCode.set(code, slug);
    bySlug.set(slug, code);
    list.push({ code, slug });
  }
  list.sort((a, b) => a.slug.localeCompare(b.slug));
  return { byCode, bySlug, list };
})();

/** ISO alpha-2 → its URL slug, or undefined when the code earns no slug (see above). */
export function countrySlug(code: string): string | undefined {
  return COUNTRY_SLUGS.byCode.get(code.toLowerCase());
}

/** A URL slug → its ISO alpha-2, or undefined when the slug names no country. */
export function countryFromSlug(slug: string): string | undefined {
  return COUNTRY_SLUGS.bySlug.get(slug.toLowerCase());
}

/** Every country carrying a slug, sorted by slug. The generator's country axis. */
export function slugifiedCountries(): SlugifiedCountry[] {
  return COUNTRY_SLUGS.list;
}
