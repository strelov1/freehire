// Display labels for the dynamic job facets (posting language, company, city,
// skill), split out of facets.ts, which re-exports the public ones.
import { SKILL_LABELS } from './generated/contracts';
import { titleCase } from './labels';
import { countryLabel } from './locationOptions';
import { metroLabel } from './metro';
import { sourceLabel } from './sourceLabels';

// Resolve an ISO 639-1 code (en, pt, ru, …) to an English language name via
// platform Intl data (no hand-maintained table); fall back to the upper-cased
// code. Mirrors countryLabel — the posting-language facet is dynamic, so any
// code the language detector emits gets a readable label.
const languageNames = (() => {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' });
  } catch {
    return null;
  }
})();

/** A representative country flag code for an ISO 639-1 language (en → gb), or
 *  undefined when none is clear. A flag only decorates a language; it never decides
 *  which languages exist. */
export function languageFlag(code: string): string | undefined {
  let region: string | undefined;
  try {
    region = code === 'en' ? 'GB' : code === 'pt' ? 'PT' : new Intl.Locale(code).maximize().region;
  } catch {
    return undefined;
  }
  return region && /^[A-Z]{2}$/.test(region) ? region.toLowerCase() : undefined;
}

function languageLabel(code: string): string {
  if (!code) return code;
  try {
    return languageNames?.of(code) ?? code.toUpperCase();
  } catch {
    return code.toUpperCase();
  }
}

// Company facet values are slugs (group-ib, epam); the live distribution carries
// no display name, so title-case the slug for a readable label. Imperfect for
// acronyms (group-ib → "Group Ib") but consistent with the other facets — a real
// slug→name lookup would need a per-company fetch, which this facet forgoes.
function companyLabel(slug: string): string {
  return titleCase(slug.replace(/-/g, '_'));
}

// Skill facet values are canonical slugs (postgresql, ci-cd, nodejs); the live
// distribution carries no display name, so map them through the generated SKILL_LABELS
// catalog (the skilltag dictionary is the source of truth). The fallback title-cases the
// slug the way companyLabel does, so a value that outlived a dictionary edit still reads
// as words rather than as data.
/** Display label for a normalised city slug: `new-york` reads as `New York`.
 *
 *  Title-casing the slug rather than looking the city up: `users.resume_cities` stores
 *  the normalisation, and the dictionary's own spellings (internal/dict/location's
 *  CityMatch.Name) are reachable only through a search API keyed by query text, not by
 *  slug. THE SEAM: a city whose real name is not its title-cased slug — 's-Hertogenbosch,
 *  Rio de Janeiro — is spelled wrong here, and fixing it means exposing a slug lookup
 *  from that dictionary rather than adding a second list of exceptions.
 *
 *  Worth having anyway: without it a card reads "berlin" beside the timezone's own
 *  "Berlin", which looks like a bug in the half that is correct. */
export function cityLabel(slug: string): string {
  return slug
    .split('-')
    .map((part) => (part ? part.charAt(0).toUpperCase() + part.slice(1) : part))
    .join(' ');
}

export function skillLabel(slug: string): string {
  return (SKILL_LABELS as Record<string, string>)[slug] ?? titleCase(slug.replace(/-/g, '_'));
}

/** Display label for a dynamic facet value: country code → name, company slug →
 *  title-cased name, else the value. */
export function dynamicLabel(param: string, value: string): string {
  if (param === 'countries') return countryLabel(value);
  if (param === 'metros') return metroLabel(value);
  if (param === 'posting_language') return languageLabel(value);
  if (param === 'company_slug') return companyLabel(value);
  if (param === 'source') return sourceLabel(value);
  // Skills have no static option list, so every surface naming a SELECTED skill — the
  // filter summary chips above all — arrives here. Without this it fell through to the
  // raw slug, and one skill was spelled two ways on one screen: "ci-cd" in the summary,
  // "CI/CD" in the panel beside it.
  if (param === 'skills') return skillLabel(value);
  return value;
}

// Role slugs carry an optional seniority grade prefix (senior_backend); the
// related-role map is keyed by the ungraded base (backend), so one entry serves
// every grade. Longest prefix first is unnecessary — the grades share no
// prefix among themselves — but drawing them from SENIORITY_VALUES keeps this in
