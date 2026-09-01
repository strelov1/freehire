// Pure SEO helpers: per-page metadata and schema.org JSON-LD built from the
// public wire shapes. Emitted server-side (see the route +page.svelte files) so
// crawlers and Google Jobs see structured data in the initial HTML.

import type { PostMeta } from './blog';
import { countryLabel } from './facets';
import { COUNTRY_REGION_MAP } from './generated/contracts';
import { companyLogoUrl } from './logo';
import type { Company, Enrichment, Job, JobCard } from './types';

const SITE = 'freehire';
// Site-level facts reused across the homepage WebSite/Organization schema.
const SITE_DESCRIPTION =
  'Freehire is an open-source search engine for tech jobs: it indexes millions of openings straight from company career boards, deduplicates them, and tags each with stack, seniority and location. Free and open source.';
const SITE_GITHUB = 'https://github.com/strelov1/freehire';

/** One question/answer pair; the shape shared by the visible FAQ and its schema. */
export type FaqItem = { question: string; answer: string };

/** Plain-text, length-capped description for `<meta name="description">` and OG,
 *  derived from the job's sanitized HTML body. Cuts at the last whole word within
 *  the budget rather than mid-word, and defaults to ~155-160 chars — Google's own
 *  typical search-snippet width, beyond which it truncates the snippet itself and
 *  we lose control of where the cut lands. */
export function metaDescription(html: string, max = 160): string {
  const text = html
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (text.length <= max) return text;
  const cut = text.slice(0, max - 1);
  const lastSpace = cut.lastIndexOf(' ');
  const truncated = lastSpace > 0 ? cut.slice(0, lastSpace) : cut;
  return `${truncated.trimEnd()}…`;
}

/** Plain-text, length-capped `<meta name="description">` for a company page.
 *  Prefers the curated `tagline`, then the imported `company_info.description`,
 *  appending whatever facts (industries, headcount, HQ) are present; falls back
 *  to the generic "Open jobs at <name>" template only when the company carries
 *  none of these — the previous behavior for every company, which made 200k+
 *  pages near-duplicates of each other. */
export function companyMetaDescription(company: Company, max = 200): string {
  const rawLead = company.tagline?.trim() || company.company_info?.description?.trim();
  // Strip a trailing period so it doesn't collide with the one the sentence below adds.
  const lead = rawLead?.endsWith('.') ? rawLead.slice(0, -1) : rawLead;

  const facts: string[] = [];
  if (company.industries?.length) facts.push(company.industries.slice(0, 2).join(' & '));
  if (company.employee_count) facts.push(`${company.employee_count.toLocaleString('en-US')}+ employees`);
  if (company.hq_country) facts.push(countryLabel(company.hq_country));
  const factClause = facts.length > 0 ? facts.join(', ') : undefined;

  let text: string;
  if (lead && factClause) {
    text = `${lead} — ${factClause}. Open roles on freehire.`;
  } else if (lead) {
    text = `${lead}. Open roles at ${company.name} on freehire.`;
  } else if (factClause) {
    text = `${company.name}: ${factClause}. Open roles on freehire.`;
  } else {
    text = `Open jobs at ${company.name}, aggregated by freehire.`;
  }

  if (text.length <= max) return text;
  return `${text.slice(0, max - 1).trimEnd()}…`;
}

/** "<title> — <company> · freehire" for the document title. */
export function jobPageTitle(job: Job): string {
  const lead = job.company ? `${job.title} — ${job.company}` : job.title;
  return `${lead} · ${SITE}`;
}

/** "<name> — <n> open jobs · freehire" for a company page's document title.
 *
 *  The bare "<name> · freehire" it replaces ran as short as 18 characters, which
 *  left most of the SERP title width unused and read identically whether the
 *  company had one opening or six thousand. The count is the fact a searcher is
 *  actually weighing, and it is the same live total the page's own heading shows.
 *
 *  No count (search failed) or a zero one falls back to the bare name: a company
 *  page still exists when nothing is open, and "0 open jobs" in a search result
 *  is an argument against clicking. */
export function companyPageTitle(name: string, total: number | undefined): string {
  if (!total) return `${name} · ${SITE}`;
  // Pinned locale, like collectionHeading: this is crawler-visible metadata, so
  // SSR and the client recompute must group digits identically.
  const roles = `${total.toLocaleString('en-US')} open ${total === 1 ? 'job' : 'jobs'}`;
  return `${name} — ${roles} · ${SITE}`;
}

/** "<n> companies hiring in tech · freehire" for the /companies directory title.
 *
 *  The bare "Companies · freehire" it replaces was 20 characters that named no
 *  subject: nothing in it could match a query, and most of the SERP title width went
 *  to the brand. The count is the fact a searcher weighs, and it is the same total
 *  the page's own heading and its ItemList describe.
 *
 *  No count (search failed) or a zero one falls back to the subject alone — the
 *  directory is a real page when a filter matches nothing, and "0 companies hiring"
 *  is an argument against clicking. */
export function companiesPageTitle(total: number | undefined): string {
  if (!total) return `Companies hiring in tech · ${SITE}`;
  // Pinned locale, like companyPageTitle: crawler-visible metadata, so SSR and the
  // client recompute must group digits identically.
  const count = total.toLocaleString('en-US');
  const subject = total === 1 ? 'company hiring in tech' : 'companies hiring in tech';
  return `${count} ${subject} · ${SITE}`;
}

/** The `<meta name="robots">` a listing page should carry, given how many results
 *  it actually rendered — `undefined` for the normal case of leaving it off.
 *
 *  A company page is reached from the sitemap, which is built from the companies
 *  search index, which is built from `companies.job_count`. That count and this
 *  page disagree by construction: the count scopes to open, non-duplicate rows,
 *  while the list is served by the JOB search index, which additionally drops
 *  private jobs, jobs with no body, and jobs whose category no dictionary resolved.
 *  A company hiring only for roles the dictionary never classified therefore ships
 *  a sitemap URL whose page says "0 open jobs" — a title, a heading and nothing to
 *  read. Telling crawlers so is the honest answer, and it self-corrects: the day a
 *  classifiable role opens, the count is non-zero and the directive is gone.
 *
 *  `undefined` on an unknown count is the load-bearing part. The company load lets
 *  a failed search resolve to null so the header and facts still render, and a
 *  transient search failure must not be spelled the same way as an empty company. */
export function listingRobots(total: number | undefined): string | undefined {
  return total === 0 ? 'noindex, follow' : undefined;
}

/** "<total> <title> jobs" — a collection's heading with its live open-job
 *  count, comma-grouped. Falls back to the plain "<title> jobs" when no count
 *  is available (the count is optional on the underlying job-search response). */
export function collectionHeading(title: string, total: number | undefined): string {
  // Pinned locale: this feeds crawler-visible <title>/OG metadata, so SSR and
  // the post-hydration client recompute must format the number identically
  // regardless of the visitor's browser locale (relevant for collections like
  // remote-brasil/remote-europe whose audience isn't en-US by default).
  return total === undefined ? `${title} jobs` : `${total.toLocaleString('en-US')} ${title} jobs`;
}

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
function applicantRegions(
  regions?: string[],
  countries?: string[]
): CountryRef | CountryRef[] | undefined {
  const iso = countries?.length
    ? countries
    : (regions ?? []).flatMap((r) => REGION_COUNTRIES[r] ?? []);
  const named = [
    ...new Set(iso.map(countryName).filter((n): n is string => Boolean(n))),
  ].sort();
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
// dictionary pinned one (job.countries is alpha-2), never a made-up street/postal
// code — mismatched structured data is a ranking liability. Shared by a non-remote
// posting and a remote one whose region never resolved (see jobPostingJsonLd).
//
// Returns undefined for a string that names no place, so the caller omits
// jobLocation rather than emitting an unresolvable one.
function jobLocationFromText(
  location: string,
  countries?: string[]
): Record<string, unknown> | undefined {
  if (isPlaceholderLocation(location)) return undefined;
  const address: Record<string, unknown> = {
    '@type': 'PostalAddress',
    addressLocality: location,
  };
  if (countries?.[0]) address.addressCountry = countries[0].toUpperCase();
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

// A shape check, deliberately NOT a vocabulary: every ISO 639-1 code passes, so
// a language new to the catalogue needs no change here.
//
// It rejects what is not a language code at all. The enrichment contract
// promises ISO 639-1 (internal/enrich/enrichment.go) but nothing enforces it —
// the value arrives as a bare string, written by an LLM — and a tag no consumer
// can resolve is worse than none, having announced a switch it cannot make.
// Should a source ever report ISO 639-3 (`fil`) or a regional tag (`pt-BR`),
// widen the pattern; until one does, both degrade to undefined rather than
// reaching markup wrong.
function isoLanguage(raw: string | undefined): string | undefined {
  const code = raw?.trim().toLowerCase();
  return code && /^[a-z]{2}$/.test(code) ? code : undefined;
}

/** The posting's own language, for a `lang` attribute on the subtree that holds
 *  it — the title and the description body, the only places a posting speaks
 *  for itself.
 *
 *  Deliberately a subtree and not the document. Public pages render English
 *  chrome by contract (hooks.server.ts pins `<html lang="en">` off `/my/**`)
 *  and that is accurate — the nav, the metadata rail and every control really
 *  are English. Only the posting is foreign, so only the posting is relabelled.
 *
 *  Undefined for English, for an unknown language, and for a `Card` (which
 *  carries no enrichment to read) — all three mean "emit no attribute", so a
 *  caller can hand the result straight to `lang` and let Svelte drop it. An
 *  empty string would be worse than silence: it asserts "language unknown" and
 *  overrides what the subtree would otherwise inherit. */
export function foreignContentLang(job: Job | JobCard): string | undefined {
  const enrichment = 'enrichment' in job ? job.enrichment : undefined;
  const code = isoLanguage(enrichment?.posting_language);
  return code === 'en' ? undefined : code;
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

/** schema.org Organization for a company page. Every company-info fact is added
 *  only when present and non-empty — an omitted field is never emitted as an empty
 *  string, null, or empty array (mismatched/empty structured data is a ranking
 *  liability, mirroring `jobPostingJsonLd`). */
export function organizationJsonLd(company: Company, origin: string): Record<string, unknown> {
  const info = company.company_info ?? {};
  const ld: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: company.name,
    url: `${origin}/companies/${company.slug}`,
  };

  if (info.logo) ld.logo = info.logo;
  if (info.description) ld.description = info.description;

  // sameAs advertises the company's canonical outbound links (homepage, LinkedIn);
  // emit only the ones we have, and drop the field entirely when we have none. The
  // homepage lives under `website` (hirebase/YC) or `homepage` (the bulk backfill,
  // often a bare domain) — read whichever is present and normalize to an absolute
  // URL, mirroring CompanyHeader.svelte, so sameAs never holds a relative string.
  const homepage = info.website ?? info.homepage;
  const sameAs: string[] = [];
  if (homepage) sameAs.push(homepage.startsWith('http') ? homepage : `https://${homepage}`);
  if (info.linkedin) sameAs.push(info.linkedin);
  if (sameAs.length > 0) ld.sameAs = sameAs;

  if (company.year_founded != null) ld.foundingDate = String(company.year_founded);
  if (company.employee_count != null) {
    ld.numberOfEmployees = { '@type': 'QuantitativeValue', value: company.employee_count };
  }
  if (company.hq_country) {
    ld.address = { '@type': 'PostalAddress', addressCountry: company.hq_country.toUpperCase() };
  }

  // aggregateRating (internal/companyfeedback's 1-5 star reviews) is the
  // highest-leverage addition here: it is what turns into a star rich-result
  // snippet in search. feedback_count is the reviewCount schema.org expects;
  // omitted entirely while it's 0, the same present-only rule every other
  // field on this object follows.
  if (company.feedback_count > 0 && company.feedback_rating_avg != null) {
    ld.aggregateRating = {
      '@type': 'AggregateRating',
      ratingValue: company.feedback_rating_avg,
      reviewCount: company.feedback_count,
      bestRating: 5,
      worstRating: 1,
    };
  }

  return ld;
}

/** schema.org WebSite for the homepage. The SearchAction advertises the job
 *  search so engines can offer a sitelinks search box straight into the feed
 *  (the homepage) at /?q=. */
export function websiteJsonLd(origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    name: SITE,
    url: `${origin}/`,
    description: SITE_DESCRIPTION,
    potentialAction: {
      '@type': 'SearchAction',
      target: {
        '@type': 'EntryPoint',
        urlTemplate: `${origin}/?q={search_term_string}`,
      },
      'query-input': 'required name=search_term_string',
    },
  };
}

/** schema.org Organization for freehire itself (the publisher) — names the entity
 *  for search/AI engines. Distinct from `organizationJsonLd`, which describes a
 *  hiring company on its own page. */
export function siteOrganizationJsonLd(origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: SITE,
    url: `${origin}/`,
    logo: `${origin}/apple-touch-icon.png`,
    description: SITE_DESCRIPTION,
    sameAs: [SITE_GITHUB],
  };
}

/** schema.org BreadcrumbList from an ordered list of trail steps. */
export function breadcrumbJsonLd(items: { name: string; url: string }[]): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: items.map((item, i) => ({
      '@type': 'ListItem',
      position: i + 1,
      name: item.name,
      item: item.url,
    })),
  };
}

/** schema.org CollectionPage wrapping an `ItemList` of whatever a list page
 *  renders — jobs on a collection landing, companies on the directory. Each item is
 *  a summary `ListItem` (position + name + detail URL), not an embedded entity —
 *  Google's recommended shape for a list page, and it keeps the payload small.
 *  Items arrive pre-resolved as `{name, url}` (the same shape `breadcrumbJsonLd`
 *  takes) so this stays entity-agnostic; `jobListItems` builds them from jobs. An
 *  empty `items` yields an empty `itemListElement`, still valid JSON-LD. */
export function collectionPageJsonLd(
  title: string,
  description: string,
  url: string,
  items: { name: string; url: string }[]
): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'CollectionPage',
    name: title,
    description,
    url,
    mainEntity: {
      '@type': 'ItemList',
      itemListElement: items.map((item, i) => ({
        '@type': 'ListItem',
        position: i + 1,
        name: item.name,
        url: item.url,
      })),
    },
  };
}

/** `{name, url}` list items for jobs, for `collectionPageJsonLd`. */
export function jobListItems(jobs: Job[], origin: string): { name: string; url: string }[] {
  return jobs.map((job) => ({ name: job.title, url: `${origin}/jobs/${job.public_slug}` }));
}

/** `{name, url}` list items for companies, for `collectionPageJsonLd`. Takes the
 *  name/slug pair structurally, so both `Company` and the lighter `CompanyListItem`
 *  the directory lists fit without a cast. */
export function companyListItems(
  companies: { name: string; slug: string }[],
  origin: string
): { name: string; url: string }[] {
  return companies.map((c) => ({ name: c.name, url: `${origin}/companies/${c.slug}` }));
}

/** schema.org Dataset descriptor for a page that presents aggregate figures
 *  (insights salary bands and demand rankings; the live /open snapshot) — tells
 *  search/AI engines the data is free to access and published by freehire.
 *
 *  `distributions` advertises the machine-readable endpoints the page's figures are
 *  read from, and is omitted when there are none: the insights pages are
 *  aggregate-only with no downloadable form, while /open cites the public JSON API
 *  beside every number. An engine that can fetch the distribution can verify a
 *  figure instead of merely quoting it. */
export function datasetJsonLd(
  name: string,
  description: string,
  url: string,
  origin: string,
  distributions: { name: string; contentUrl: string }[] = []
): Record<string, unknown> {
  const ld: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'Dataset',
    name,
    description,
    url,
    isAccessibleForFree: true,
    creator: { '@type': 'Organization', name: SITE, url: `${origin}/` },
  };
  if (distributions.length > 0) {
    ld.distribution = distributions.map((d) => ({
      '@type': 'DataDownload',
      name: d.name,
      encodingFormat: 'application/json',
      contentUrl: d.contentUrl,
    }));
  }
  return ld;
}

/** schema.org FAQPage. The questions must also appear as visible text on the page
 *  (Google's requirement), so it is built from the same source as the rendered FAQ. */
export function faqPageJsonLd(faqs: FaqItem[]): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: faqs.map((f) => ({
      '@type': 'Question',
      name: f.question,
      acceptedAnswer: { '@type': 'Answer', text: f.answer },
    })),
  };
}

/** schema.org DefinedTerm for one skill's glossary page.
 *
 *  The one thing on that page a machine cannot infer from the prose: that the heading
 *  names a term, that the paragraph under it is that term's definition, and that both
 *  belong to one glossary rather than being an article that happens to mention a word.
 *  `@id` is the page itself, so the term and the URL are the same entity. */
export function definedTermJsonLd(
  term: { slug: string; label: string; description: string },
  origin: string,
): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'DefinedTerm',
    '@id': `${origin}/skills/${term.slug}`,
    name: term.label,
    description: term.description,
    termCode: term.slug,
    inDefinedTermSet: {
      '@type': 'DefinedTermSet',
      '@id': `${origin}/skills`,
      name: 'freehire IT skills glossary',
    },
  };
}

/** schema.org WebAPI for the API reference — names the endpoint set as one entity
 *  and points at the OpenAPI document, so an assistant can fetch the machine-readable
 *  spec instead of scraping the prose it is rendered beside. */
export function webApiJsonLd(origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebAPI',
    name: 'freehire API',
    description:
      'A read-first, open HTTP API over the freehire job catalogue: query jobs by seniority, skills, region and salary, read companies, and track applications with an API key.',
    url: `${origin}/docs/api`,
    documentation: `${origin}/docs/api`,
    termsOfService: `${origin}/privacy`,
    provider: { '@type': 'Organization', name: SITE, url: `${origin}/` },
    // The OpenAPI document is the spec itself, not more prose about it.
    potentialAction: {
      '@type': 'ConsumeAction',
      target: { '@type': 'EntryPoint', urlTemplate: `${origin}/openapi.yaml`, encodingType: 'application/yaml' },
    },
  };
}

/** schema.org SoftwareApplication for the CLI page. Free and open source, so the
 *  zero-price `offers` is a fact rather than a marketing claim; `softwareHelp`
 *  points back at the page that documents it. */
export function cliApplicationJsonLd(
  origin: string,
  repositories: string[]
): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: 'freehire CLI',
    description:
      'A small Go CLI and an MCP server over the freehire job API, so an AI agent or a script can search, open and track jobs without a browser. One API key drives both.',
    url: `${origin}/cli`,
    applicationCategory: 'DeveloperApplication',
    operatingSystem: 'macOS, Linux',
    isAccessibleForFree: true,
    offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
    softwareHelp: { '@type': 'CreativeWork', url: `${origin}/cli` },
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
    codeRepository: repositories,
  };
}

/** schema.org SoftwareApplication for the browser-extension landing. `installUrl`
 *  is passed in rather than written here so it comes from the same constant the
 *  page's buttons use (`extensionLinks.ts`) — structured data must not name a
 *  destination the page does not offer. `browserRequirements` is the honest
 *  bound: the panel is built on Chrome's `sidePanel` API, which Firefox and
 *  Safari do not have. */
export function extensionApplicationJsonLd(
  origin: string,
  installUrl: string
): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: 'freehire browser extension',
    description:
      "A job-application agent in Chrome's side panel: it reads the posting you are on, scores it against your CV, and fills the application form from your profile — you press Submit.",
    url: `${origin}/features/extension`,
    applicationCategory: 'BrowserApplication',
    operatingSystem: 'Chrome',
    browserRequirements: 'Requires Chrome 114 or a Chromium browser with side-panel support',
    installUrl,
    isAccessibleForFree: true,
    offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
    softwareHelp: { '@type': 'CreativeWork', url: `${origin}/features/extension` },
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
  };
}

/** schema.org Blog for the feed index, listing each post as a `BlogPosting`. A
 *  `Blog` (rather than a bare CollectionPage) is what ties the index to the
 *  `Article` on each post page, so engines read the feed as one publication.
 *  Summary entries only — the post bodies live on their own pages. */
export function blogJsonLd(posts: PostMeta[], origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'Blog',
    name: `${SITE} blog`,
    url: `${origin}/blog`,
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
    blogPost: posts.map((post) => ({
      '@type': 'BlogPosting',
      headline: post.title,
      description: post.summary,
      datePublished: post.date,
      url: `${origin}/blog/${post.slug}`,
    })),
  };
}

/** schema.org Article for a blog post page. Built from the validated `PostMeta`,
 *  so every field is present; `keywords` is emitted only when the post has tags. */
export function articleJsonLd(post: PostMeta, origin: string): Record<string, unknown> {
  const ld: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'Article',
    headline: post.title,
    description: post.summary,
    datePublished: post.date,
    url: `${origin}/blog/${post.slug}`,
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
  };
  if (post.tags.length > 0) ld.keywords = post.tags.join(', ');
  return ld;
}

/** Render a JSON-LD object as a ready-to-inline `<script>` string. `<` is escaped
 *  to `<` so an embedded `</script>` (job descriptions are HTML) can never
 *  break out of the tag — a correctness and XSS guard. */
export function jsonLdScript(data: unknown): string {
  const json = JSON.stringify(data).replace(/</g, '\\u003c');
  return `<script type="application/ld+json">${json}</script>`;
}
