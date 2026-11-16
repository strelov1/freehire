// Pure SEO helpers: per-page metadata and schema.org JSON-LD built from the
// public wire shapes. Emitted server-side (see the route +page.svelte files) so
// crawlers and Google Jobs see structured data in the initial HTML.

import { countryLabel } from './facets';
import type { Company, Job, JobCard } from './types';

export const SITE = 'freehire';
// Site-level facts reused across the homepage WebSite/Organization schema.
const SITE_DESCRIPTION =
  'Freehire is an open-source search engine for tech jobs: it indexes millions of openings straight from company career boards, deduplicates them, and tags each with stack, seniority and location. Free and open source.';
const SITE_GITHUB = 'https://github.com/strelov1/freehire';

/** One question/answer pair; the shape shared by the visible FAQ and its schema. */
export type FaqItem = { question: string; answer: string };

const namedEntities: Record<string, string> = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: '\u00a0',
  AMP: '&',
  LT: '<',
  GT: '>',
  QUOT: '"',
  APOS: "'",
  NBSP: '\u00a0',
};

function decodeEntities(value: string): string {
  const quotes = value.replace(/&amp;(?:amp;)?#39;/g, '&#39;').replace(/&amp;(?:amp;)?#34;/g, '&#34;');
  return quotes.replace(/&(#x[0-9a-fA-F]+|#\d+|[a-zA-Z][a-zA-Z0-9]*);/g, (entity, body: string) => {
    if (body.charAt(0) !== '#') return namedEntities[body] ?? entity;
    const hex = body.startsWith('#x');
    const code = Number.parseInt(body.slice(hex ? 2 : 1), hex ? 16 : 10);
    const control = code < 32 && code !== 9 && code !== 10 && code !== 13;
    const surrogate = code >= 0xd800 && code <= 0xdfff;
    if (!Number.isInteger(code) || control || surrogate || code > 0x10ffff) return entity;
    return String.fromCodePoint(code);
  });
}

/** Plain-text, length-capped description for `<meta name="description">` and OG,
 *  derived from the job's sanitized HTML body. Cuts at the last whole word within
 *  the budget rather than mid-word, and defaults to ~155-160 chars — Google's own
 *  typical search-snippet width, beyond which it truncates the snippet itself and
 *  we lose control of where the cut lands. Character references are decoded after
 *  tags are stripped, so a list card shows the same letters the job page does. */
export function metaDescription(html: string, max = 160): string {
  const text = decodeEntities(html.replace(/<[^>]*>/g, ' '))
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
export function isoLanguage(raw: string | undefined): string | undefined {
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
 *  search so engines can offer a sitelinks search box straight into the feed, which
 *  lives at /jobs?q= — the homepage is the landing page above it, and a sitelinks box
 *  pointed there would hand the query to a page that renders no results. */
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
        urlTemplate: `${origin}/jobs?q={search_term_string}`,
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
  items: { name: string; url: string }[],
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
  origin: string,
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
  distributions: { name: string; contentUrl: string }[] = [],
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

/** Render a JSON-LD object as a ready-to-inline `<script>` string. `<` is escaped
 *  to `<` so an embedded `</script>` (job descriptions are HTML) can never
 *  break out of the tag — a correctness and XSS guard. */
export function jsonLdScript(data: unknown): string {
  const json = JSON.stringify(data).replace(/</g, '\\u003c');
  return `<script type="application/ld+json">${json}</script>`;
}
