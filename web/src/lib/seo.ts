// Pure SEO helpers: per-page metadata and schema.org JSON-LD built from the
// public wire shapes. Emitted server-side (see the route +page.svelte files) so
// crawlers and Google Jobs see structured data in the initial HTML.

import type { PostMeta } from './blog';
import { logoDevUrl } from './logo';
import type { Company, Enrichment, Job } from './types';

const SITE = 'freehire';
// Site-level facts reused across the homepage WebSite/Organization schema.
const SITE_DESCRIPTION =
  'FreeHire is an open-source search engine for tech jobs: it indexes millions of openings straight from company career boards, deduplicates them, and tags each with stack, seniority and location. Free and open source.';
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

// schema.org educationRequirements.credentialCategory, from the enrich
// education_level vocabulary ("none"/"bachelor"/"master"/"phd"). "none" and any
// unmapped value carry no requirement and are omitted.
const EDUCATION_CREDENTIAL: Record<string, string> = {
  bachelor: 'bachelor degree',
  master: 'postgraduate degree',
  phd: 'postgraduate degree',
};

/** schema.org JobPosting for a job-detail page, eligible for Google Jobs. A
 *  closed posting sets `validThrough` to its close time so it reads as expired,
 *  not open. `origin` is the absolute site origin (e.g. https://freehire.dev). */
export function jobPostingJsonLd(job: Job, origin: string): Record<string, unknown> {
  const e = job.enrichment ?? {};
  // logo.dev resolves a logo from the company name (404s for unknown companies,
  // which Google silently ignores); same source the SPA and OG cards use.
  const logo = logoDevUrl(job.company);
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
  // A closed posting is no longer accepting applications: mark it expired.
  if (job.closed_at) ld.validThrough = job.closed_at;

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
    ld.jobLocationType = 'TELECOMMUTE';
    const regions = applicantRegions(job.regions);
    if (regions) ld.applicantLocationRequirements = regions;
  } else if (job.location) {
    // Google accepts a PostalAddress with just locality; add the ISO country code
    // when the geo dictionary pinned one (job.countries is alpha-2), never a made-up
    // street/postal code — mismatched structured data is a ranking liability.
    const address: Record<string, unknown> = {
      '@type': 'PostalAddress',
      addressLocality: job.location,
    };
    if (job.countries?.[0]) address.addressCountry = job.countries[0].toUpperCase();
    ld.jobLocation = { '@type': 'Place', address };
  }

  if (e.salary_min != null || e.salary_max != null) {
    ld.baseSalary = baseSalary(e);
  }

  // skills is the dictionary facet (canonical names), served as Google Text.
  if (job.skills?.length) ld.skills = job.skills.join(', ');

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

/** schema.org CollectionPage wrapping an `ItemList` of the jobs rendered on a
 *  collection landing page. Each item is a summary `ListItem` (position + name +
 *  detail URL), not an embedded `JobPosting` — Google's recommended shape for a
 *  list page, and it keeps the payload small. An empty `jobs` yields an empty
 *  `itemListElement`, still valid JSON-LD. */
export function collectionPageJsonLd(
  title: string,
  description: string,
  url: string,
  jobs: Job[],
  origin: string
): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'CollectionPage',
    name: title,
    description,
    url,
    mainEntity: {
      '@type': 'ItemList',
      itemListElement: jobs.map((job, i) => ({
        '@type': 'ListItem',
        position: i + 1,
        name: job.title,
        url: `${origin}/jobs/${job.public_slug}`,
      })),
    },
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
