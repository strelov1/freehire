// schema.org JobPosting for a job page, and the private helpers only it needs.

import { COUNTRY_REGION_MAP } from './generated/contracts';
import { companyLogoUrl } from './logo';
import { isoLanguage } from './seo';
import type { Enrichment, Job } from './types';

// schema.org employmentType, from the enrich vocabulary (see internal/enrich).
const EMPLOYMENT_TYPE: Record<string, string> = {
  full_time: 'FULL_TIME',
  part_time: 'PART_TIME',
  contract: 'CONTRACTOR',
  internship: 'INTERN',
};

// schema.org QuantitativeValue.unitText, from the enrich salary_period vocab
// (year is the implicit default).
const SALARY_UNIT: Record<string, string> = {
  hour: 'HOUR',
  day: 'DAY',
  month: 'MONTH',
  year: 'YEAR',
};

// Countries of a region, inverted from the generated country→region map (the
// same grouping internal/location keys the facet on). Built once at module load.
const REGION_COUNTRIES: Record<string, string[]> = (() => {
  const out: Record<string, string[]> = {};
  for (const [code, region] of Object.entries(COUNTRY_REGION_MAP)) {
    (out[region] ??= []).push(code);
  }
  return out;
})();

// ISO 3166-1 alpha-2 → English country name. Intl carries the list, so there is
// no country table to keep in sync here; a code it doesn't know (e.g. the 'xk'
// user-assigned code for Kosovo) comes back unchanged and is dropped rather than
// emitted as a two-letter "name".
const COUNTRY_DISPLAY = new Intl.DisplayNames(['en'], { type: 'region' });

function countryName(iso: string): string | undefined {
  const code = iso.toUpperCase();
  try {
    const name = COUNTRY_DISPLAY.of(code);
    // `of` echoes an unknown code back rather than failing; that is not a name.
    return !name || name === code ? undefined : name;
  } catch {
    // Malformed input (not a region code at all) — Intl throws rather than echoes.
    return undefined;
  }
}

type CountryRef = { '@type': 'Country'; name: string };

// Google resolves applicantLocationRequirements at Country or State level and
// geocodes the name, so a supranational bloc ("North America", "APAC") is a value
// it cannot place — and TELECOMMUTE without a usable companion is an invalid
// combination. Emit countries instead: the posting's own `countries` when it has
// them (the precise fact), else the countries its region groups. A worldwide
// reach ('global') groups none and states no requirement, which is correct — it
// restricts nobody.
function applicantRegions(regions?: string[], countries?: string[]): CountryRef | CountryRef[] | undefined {
  const iso = countries?.length ? countries : (regions ?? []).flatMap((r) => REGION_COUNTRIES[r] ?? []);
  const named = [...new Set(iso.map(countryName).filter((n): n is string => Boolean(n)))].sort();
  if (named.length === 0) return undefined;
  const entries: CountryRef[] = named.map((name) => ({ '@type': 'Country', name }));
  // schema.org reads a lone object and a one-element array alike; emit the object
  // so the common single-country case stays readable in the page source.
  return entries.length === 1 ? entries[0] : entries;
}

function baseSalary(e: Enrichment): Record<string, unknown> {
  const value: Record<string, unknown> = { '@type': 'QuantitativeValue' };
  if (e.salary_min != null) value.minValue = e.salary_min;
  if (e.salary_max != null) value.maxValue = e.salary_max;
  value.unitText = (e.salary_period && SALARY_UNIT[e.salary_period]) || 'YEAR';
  return {
    '@type': 'MonetaryAmount',
    currency: e.salary_currency || 'USD',
    value,
  };
}

// Location strings that restate the work mode instead of naming a place. Sources
// put these in the location field constantly, and passing one through made the
// posting claim a city that does not exist: 8 of the 10 postings sampled from the
// Remote Worldwide collection told Google `addressLocality: "Remote"`, with no
// addressCountry to place it. Google geocodes jobLocation against real places, so
// that is not a harmless label — it is a location it cannot resolve.
//
// Matched on the whole (trimmed, lowercased) string, never as a substring:
// "Remote within the Continental United States" does name a real area, and
// dropping it would discard a genuine restriction.
const PLACEHOLDER_LOCATIONS = new Set([
  'remote',
  'fully remote',
  '100% remote',
  'remote worldwide',
  'remote - worldwide',
  'remote (worldwide)',
  'worldwide',
  'anywhere',
  'global',
  'work from home',
  'wfh',
]);

function isPlaceholderLocation(location: string): boolean {
  return PLACEHOLDER_LOCATIONS.has(location.trim().toLowerCase());
}

// schema.org jobLocation from the raw location string: Google accepts a
// PostalAddress with just locality; add the ISO country code when the geo
// dictionary pinned exactly one (job.countries is alpha-2), never a made-up street/postal
// code — mismatched structured data is a ranking liability. Shared by a non-remote
// posting and a remote one whose region never resolved (see jobPostingJsonLd).
//
// Returns undefined for a string that names no place, so the caller omits
// jobLocation rather than emitting an unresolvable one.
function jobLocationFromText(location: string, countries?: string[]): Record<string, unknown> | undefined {
  if (isPlaceholderLocation(location)) return undefined;
  const address: Record<string, unknown> = {
    '@type': 'PostalAddress',
    addressLocality: location,
  };
  // Only an unambiguous country: a posting that stands for its reposts carries all their
  // countries, sorted, so the first is an alphabet's pick rather than this locality's.
  const only = countries?.length === 1 ? countries[0] : undefined;
  if (only) address.addressCountry = only.toUpperCase();
  return { '@type': 'Place', address };
}

// schema.org educationRequirements.credentialCategory, from the enrich
// education_level vocabulary ("none"/"bachelor"/"master"/"phd"). "none" and any
// unmapped value carry no requirement and are omitted.
const EDUCATION_CREDENTIAL: Record<string, string> = {
  bachelor: 'bachelor degree',
  master: 'postgraduate degree',
  phd: 'postgraduate degree',
};

// Days added on top of the freshest "still open" evidence to estimate how much
// longer an open posting is likely valid. Most sources carry no real listing-
// expiry date, and without validThrough Google assumes one itself (~30 days)
// and drops the posting from Google Jobs even while it's still open — this
// buffer matches that default, but keeps rolling forward on every recrawl
// instead of freezing at first sight. Comfortably above the 48h unseen-sweep
// grace (docs/agents/job-lifecycle.md) so an ordinary recrawl gap never reads
// as expired.
const VALID_THROUGH_BUFFER_DAYS = 30;

/** Estimated validThrough for an OPEN job: the freshest "still open" evidence
 *  (last_seen_at, falling back to the posting date for a job never re-crawled,
 *  e.g. a manual import) plus the buffer above. undefined when there's no date
 *  to estimate from at all. */
function estimatedValidThrough(job: Job): string | undefined {
  const evidence = job.last_seen_at ?? job.posted_at ?? job.created_at;
  if (!evidence) return undefined;
  const d = new Date(evidence);
  d.setUTCDate(d.getUTCDate() + VALID_THROUGH_BUFFER_DAYS);
  return d.toISOString();
}

/** schema.org JobPosting for a job-detail page, eligible for Google Jobs. A
 *  closed posting sets `validThrough` to its close time so it reads as expired,
 *  not open; an open one gets a rolling estimate (see estimatedValidThrough) so
 *  Google doesn't apply its own ~30-day default and drop it while still live.
 *  `origin` is the absolute site origin (e.g. https://freehire.me). */
export function jobPostingJsonLd(job: Job, origin: string): Record<string, unknown> {
  const e = job.enrichment ?? {};
  // Our logo proxy resolves a logo from the company name (404s for unknown
  // companies, which Google silently ignores); same source the SPA and OG cards use.
  const logo = companyLogoUrl(job.company);
  const ld: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'JobPosting',
    title: job.title,
    description: job.description || job.title,
    url: `${origin}/jobs/${job.public_slug}`,
    hiringOrganization: {
      '@type': 'Organization',
      name: job.company || 'Unknown',
      ...(job.company_slug ? { sameAs: `${origin}/companies/${job.company_slug}` } : {}),
      ...(logo ? { logo } : {}),
    },
  };

  // datePosted is required for Google Jobs. Many sources carry no publish date,
  // so fall back to created_at (the ingest time; always set) rather than omit it.
  const datePosted = job.posted_at ?? job.created_at;
  if (datePosted) ld.datePosted = datePosted;
  // A closed posting is no longer accepting applications: mark it expired. An
  // open one gets a rolling estimate instead of nothing, so Google doesn't
  // apply its own default expiry assumption to a posting that's still live.
  if (job.closed_at) {
    ld.validThrough = job.closed_at;
  } else if (job.application_deadline) {
    // The source's own last day, inclusive, beats any estimate.
    ld.validThrough = `${job.application_deadline}T23:59:59Z`;
  } else {
    const validThrough = estimatedValidThrough(job);
    if (validThrough) ld.validThrough = validThrough;
  }

  // identifier is the hiring org's own posting id (Google-recommended): external_id
  // is the source's stable job id, so Google dedupes the vacancy across boards
  // rather than reading us as a scraped copy. external_id is the namespaced dedup
  // key "<board>:<id>" (sources.NamespaceExternalID); a boardless source yields a
  // bare ":<id>", so drop a leading colon for a clean identifier value.
  if (job.external_id) {
    ld.identifier = {
      '@type': 'PropertyValue',
      name: job.company || 'Unknown',
      value: job.external_id.replace(/^:/, ''),
    };
  }

  // Google's own question: can a candidate apply without leaving this page? True only
  // for a vacancy its employer publishes here and takes applications for here; every
  // other posting sends the candidate to the employer's site, so it says nothing.
  if (job.direct_apply) ld.directApply = true;

  const empType = e.employment_type ? EMPLOYMENT_TYPE[e.employment_type] : undefined;
  if (empType) ld.employmentType = empType;

  if (job.work_mode === 'remote') {
    const regions = applicantRegions(job.regions, job.countries);
    if (regions) {
      // Google requires applicantLocationRequirements whenever jobLocationType is
      // TELECOMMUTE — set them together, never TELECOMMUTE alone.
      ld.jobLocationType = 'TELECOMMUTE';
      ld.applicantLocationRequirements = regions;
    } else if (job.location) {
      // No resolved region to state a location *requirement* from (the geo
      // dictionary and the LLM fallback both came up empty — a real, if raw,
      // location string still reached the posting). Asserting TELECOMMUTE without
      // its required companion would be worse than not asserting it: fall back to
      // the same plain jobLocation a non-remote posting gets.
      //
      // Unless that string is a placeholder like "Remote", which is the work mode
      // written into the location field. Then there is still nothing to state and
      // this joins the omit-entirely case below.
      const place = jobLocationFromText(job.location, job.countries);
      if (place) ld.jobLocation = place;
    }
    // Neither a resolved region nor any location text at all: nothing honest to
    // state, so location is omitted rather than guessed.
  } else if (job.location) {
    const place = jobLocationFromText(job.location, job.countries);
    if (place) ld.jobLocation = place;
  }

  if (e.salary_min != null || e.salary_max != null) {
    ld.baseSalary = baseSalary(e);
  }

  // skills is the dictionary facet (canonical names), served as Google Text.
  if (job.skills?.length) ld.skills = job.skills.join(', ');

  // Every known language, English included: a JSON-LD node inherits nothing, so
  // unlike the `lang` attribute there is no default here worth staying silent
  // about (see foreignContentLang).
  const inLanguage = isoLanguage(e.posting_language);
  if (inLanguage) ld.inLanguage = inLanguage;

  // A zero minimum (explicit entry-level) carries no SEO signal, so omit it.
  if (e.experience_years_min != null && e.experience_years_min > 0) {
    ld.experienceRequirements = {
      '@type': 'OccupationalExperienceRequirements',
      monthsOfExperience: e.experience_years_min * 12,
    };
  }

  const credential = e.education_level ? EDUCATION_CREDENTIAL[e.education_level] : undefined;
  if (credential) {
    ld.educationRequirements = {
      '@type': 'EducationalOccupationalCredential',
      credentialCategory: credential,
    };
  }

  return ld;
}
