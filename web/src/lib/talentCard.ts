import type { CatalogueMember } from '$lib/generated/contracts';
import { CATEGORY_LABELS, SENIORITY_LABELS, titleCase } from '$lib/labels';
import { countryOfTimezone } from '$lib/timezoneCountry';

// What a Talent Network card SAYS, derived once. The list card and the single-card page
// render different layouts over the same three derivations, and three copies of a rule
// about how somebody is described is how two of them end up disagreeing.

/** The heading for a card or a role: the two dictionary values joined, or the fallback
 *  when neither resolves.
 *
 *  Built rather than quoted — the payload carries no job title, only what `classify`
 *  resolved it to. `fallback` differs by surface: a whole candidate reads as "Candidate",
 *  one unlabelled row of their history as "Role".
 *
 *  Both label maps are EXCEPTION tables, not full dictionaries: SENIORITY_LABELS carries
 *  one entry (`c_level`), and everything else is meant to fall through to titleCase, the
 *  same fallback insights.ts and facets.ts use. Reading them as complete — `?? value` —
 *  is why headings first rendered as "senior backend". */
export function talentHeading(
  seniority: string | undefined,
  category: string | undefined,
  fallback: string,
): string {
  const grade = seniority ? (SENIORITY_LABELS[seniority] ?? titleCase(seniority)) : '';
  const discipline = category ? (CATEGORY_LABELS[category] ?? titleCase(category)) : '';
  return [grade, discipline].filter(Boolean).join(' ') || fallback;
}

/** Where a member is, in the three forms a card shows it. */
export interface TalentPlace {
  /** ISO country code from the timezone, or undefined when the zone names an offset. */
  country?: string;
  /** The city half of the IANA zone, underscores restored: 'New York'. */
  zone?: string;
  /** The normalised cities from the member's own record, joined. */
  place: string;
}

/** Read a member's location.
 *
 *  The country comes from the TIMEZONE, not from the city: the zone is machine-precise
 *  and generated from tzdata, whereas turning a city name into a country would need a
 *  gazetteer this repository does not have. The zone's region prefix is dropped, because
 *  the city already implies it. */
export function talentPlace(member: CatalogueMember): TalentPlace {
  return {
    country: countryOfTimezone(member.timezone),
    zone: member.timezone?.split('/').pop()?.replace(/_/g, ' '),
    place: member.cities.join(', '),
  };
}
