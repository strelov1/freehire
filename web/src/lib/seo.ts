// Pure SEO helpers: per-page metadata and schema.org JSON-LD built from the
// public wire shapes. Emitted server-side (see the route +page.svelte files) so
// crawlers and Google Jobs see structured data in the initial HTML.

import type { Company, Enrichment, Job } from './types';

const SITE = 'freehire';
// Site-level facts reused across the homepage WebSite/Organization schema.
const SITE_DESCRIPTION =
  'FreeHire aggregates tech jobs straight from company career boards, deduplicates them, and tags each with stack, seniority and location. Free and open source.';
const SITE_GITHUB = 'https://github.com/strelov1/freehire';

/** One question/answer pair; the shape shared by the visible FAQ and its schema. */
export type FaqItem = { question: string; answer: string };

/** Plain-text, length-capped description for `<meta name="description">` and OG,
 *  derived from the job's sanitized HTML body. */
export function metaDescription(html: string, max = 200): string {
  const text = html
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (text.length <= max) return text;
  return `${text.slice(0, max - 1).trimEnd()}…`;
}

/** "<title> — <company> · freehire" for the document title. */
export function jobPageTitle(job: Job): string {
  const lead = job.company ? `${job.title} — ${job.company}` : job.title;
  return `${lead} · ${SITE}`;
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

// Region codes that name a place for applicantLocationRequirements. Broad or
// worldwide reaches ('global') intentionally omit a requirement.
const REGION_NAME: Record<string, string> = {
  eu: 'European Union',
  eea: 'European Economic Area',
  uk: 'United Kingdom',
  us: 'United States',
  ru: 'Russia',
  americas: 'Americas',
  north_america: 'North America',
  latam: 'Latin America',
  apac: 'Asia-Pacific',
  emea: 'EMEA',
  mena: 'MENA',
  africa: 'Africa',
  cis: 'CIS',
  central_asia: 'Central Asia',
};

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

function applicantRegions(regions?: string[]): unknown {
  const named = (regions ?? [])
    .map((r) => REGION_NAME[r])
    .filter((name): name is string => Boolean(name))
    .map((name) => ({ '@type': 'Country', name }));
  if (named.length === 0) return undefined;
  return named.length === 1 ? named[0] : named;
}

/** schema.org JobPosting for a job-detail page, eligible for Google Jobs. A
 *  closed posting sets `validThrough` to its close time so it reads as expired,
 *  not open. `origin` is the absolute site origin (e.g. https://freehire.dev). */
export function jobPostingJsonLd(job: Job, origin: string): Record<string, unknown> {
  const e = job.enrichment ?? {};
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
    },
  };

  // datePosted is required for Google Jobs. Many sources carry no publish date,
  // so fall back to created_at (the ingest time; always set) rather than omit it.
  const datePosted = job.posted_at ?? job.created_at;
  if (datePosted) ld.datePosted = datePosted;
  // A closed posting is no longer accepting applications: mark it expired.
  if (job.closed_at) ld.validThrough = job.closed_at;

  const empType = e.employment_type ? EMPLOYMENT_TYPE[e.employment_type] : undefined;
  if (empType) ld.employmentType = empType;

  if (job.work_mode === 'remote') {
    ld.jobLocationType = 'TELECOMMUTE';
    const regions = applicantRegions(job.regions);
    if (regions) ld.applicantLocationRequirements = regions;
  } else if (job.location) {
    ld.jobLocation = {
      '@type': 'Place',
      address: { '@type': 'PostalAddress', addressLocality: job.location },
    };
  }

  if (e.salary_min != null || e.salary_max != null) {
    ld.baseSalary = baseSalary(e);
  }

  return ld;
}

/** schema.org Organization for a company page. */
export function organizationJsonLd(company: Company, origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: company.name,
    url: `${origin}/companies/${company.slug}`,
  };
}

/** schema.org WebSite for the homepage. The SearchAction advertises the job
 *  search so engines can offer a sitelinks search box straight into /jobs?q=. */
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

/** Render a JSON-LD object as a ready-to-inline `<script>` string. `<` is escaped
 *  to `<` so an embedded `</script>` (job descriptions are HTML) can never
 *  break out of the tag — a correctness and XSS guard. */
export function jsonLdScript(data: unknown): string {
  const json = JSON.stringify(data).replace(/</g, '\\u003c');
  return `<script type="application/ld+json">${json}</script>`;
}
