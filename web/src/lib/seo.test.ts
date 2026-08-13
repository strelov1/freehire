import { describe, expect, it } from 'vitest';
import {
  articleJsonLd,
  collectionHeading,
  collectionPageJsonLd,
  companyListItems,
  companyMetaDescription,
  datasetJsonLd,
  jobListItems,
  jobPostingJsonLd,
  organizationJsonLd,
} from './seo';
import { companyLogoUrl } from './logo';
import type { PostMeta } from './blog';
import type { Company, Job } from './types';

// collectionPageJsonLd reads only title + public_slug off each job.
function job(title: string, slug: string): Job {
  return { title, public_slug: slug } as Job;
}

// A job carrying the fields jobPostingJsonLd reads; tests spread the facts under test.
function postingJob(overrides: Partial<Job> = {}): Job {
  return {
    public_slug: 'engineer-abc',
    title: 'Engineer',
    company: 'Acme',
    description: 'Build things.',
    skills: [],
    ...overrides,
  } as Job;
}

// Minimal valid Company; individual tests spread in the facts under test.
function company(overrides: Partial<Company> = {}): Company {
  return {
    slug: 'acme',
    name: 'Acme',
    collections: [],
    created_at: null,
    updated_at: null,
    upvote_count: 0,
    downvote_count: 0,
    my_vote: 0,
    feedback_count: 0,
    feedback_rating_avg: null,
    ...overrides,
  };
}

const ORIGIN = 'https://freehire.me';

describe('collectionHeading', () => {
  it('includes the exact live count, comma-grouped', () => {
    expect(collectionHeading('React', 1234)).toBe('1,234 React jobs');
  });

  it('falls back to the plain title when no count is available', () => {
    expect(collectionHeading('React', undefined)).toBe('React jobs');
  });

  it('renders a zero count honestly rather than hiding it', () => {
    expect(collectionHeading('React', 0)).toBe('0 React jobs');
  });
});

describe('companyMetaDescription', () => {
  it('falls back to the generic template when no facts are present', () => {
    expect(companyMetaDescription(company())).toBe('Open jobs at Acme, aggregated by freehire.');
  });

  it('leads with the tagline and appends industries, headcount and HQ', () => {
    expect(
      companyMetaDescription(
        company({
          tagline: 'Rockets, on demand',
          industries: ['Aerospace', 'Robotics', 'Defense'],
          employee_count: 250,
          hq_country: 'us',
        })
      )
    ).toBe('Rockets, on demand — Aerospace & Robotics, 250+ employees, United States. Open roles on freehire.');
  });

  it('prefers company_info.description over the tagline when no tagline is set', () => {
    expect(
      companyMetaDescription(company({ company_info: { description: 'Acme builds rockets.' } }))
    ).toBe('Acme builds rockets. Open roles at Acme on freehire.');
  });

  it('uses only the facts when there is no tagline or description', () => {
    expect(companyMetaDescription(company({ hq_country: 'de' }))).toBe(
      'Acme: Germany. Open roles on freehire.'
    );
  });

  it('truncates to max length with an ellipsis', () => {
    const result = companyMetaDescription(company({ tagline: 'x'.repeat(250) }), 50);
    expect(result.length).toBe(50);
    expect(result.endsWith('…')).toBe(true);
  });
});

describe('organizationJsonLd', () => {
  it('emits every company-info fact the company provides', () => {
    const ld = organizationJsonLd(
      company({
        year_founded: 2015,
        employee_count: 250,
        hq_country: 'us',
        company_info: {
          logo: 'https://logo.dev/acme.png',
          description: 'Acme builds rockets.',
          website: 'https://acme.example',
          linkedin: 'https://linkedin.com/company/acme',
        },
      }),
      ORIGIN
    );

    expect(ld['@type']).toBe('Organization');
    expect(ld.name).toBe('Acme');
    expect(ld.url).toBe('https://freehire.me/companies/acme');
    expect(ld.logo).toBe('https://logo.dev/acme.png');
    expect(ld.description).toBe('Acme builds rockets.');
    expect(ld.sameAs).toEqual([
      'https://acme.example',
      'https://linkedin.com/company/acme',
    ]);
    expect(ld.foundingDate).toBe('2015');
    expect(ld.numberOfEmployees).toEqual({
      '@type': 'QuantitativeValue',
      value: 250,
    });
    expect(ld.address).toEqual({
      '@type': 'PostalAddress',
      addressCountry: 'US',
    });
  });

  it('omits every fact a bare company lacks', () => {
    const ld = organizationJsonLd(company(), ORIGIN);

    expect(ld).toEqual({
      '@context': 'https://schema.org',
      '@type': 'Organization',
      name: 'Acme',
      url: 'https://freehire.me/companies/acme',
    });
  });

  it('includes only the links that are present in sameAs', () => {
    const ld = organizationJsonLd(
      company({ company_info: { website: 'https://acme.example' } }),
      ORIGIN
    );

    expect(ld.sameAs).toEqual(['https://acme.example']);
  });

  it('falls back to the homepage field and normalizes a bare domain to https', () => {
    // The bulk company-info backfill stores the homepage under `homepage` (often a
    // bare domain), not `website` — sameAs must still emit a valid absolute URL.
    const ld = organizationJsonLd(
      company({ company_info: { homepage: 'acme.com' } }),
      ORIGIN
    );

    expect(ld.sameAs).toEqual(['https://acme.com']);
  });

  it('prefers website over homepage and leaves an already-absolute homepage as-is', () => {
    const ld = organizationJsonLd(
      company({
        company_info: { website: 'https://acme.io', homepage: 'https://old.acme.com' },
      }),
      ORIGIN
    );

    expect(ld.sameAs).toEqual(['https://acme.io']);
  });

  it('omits sameAs entirely when no links are present', () => {
    const ld = organizationJsonLd(
      company({ company_info: { description: 'no links here' } }),
      ORIGIN
    );

    expect(ld).not.toHaveProperty('sameAs');
  });

  it('emits aggregateRating when the company has feedback', () => {
    const ld = organizationJsonLd(
      company({ feedback_count: 12, feedback_rating_avg: 4.5 }),
      ORIGIN
    );

    expect(ld.aggregateRating).toEqual({
      '@type': 'AggregateRating',
      ratingValue: 4.5,
      reviewCount: 12,
      bestRating: 5,
      worstRating: 1,
    });
  });

  it('omits aggregateRating when the company has no feedback yet', () => {
    const ld = organizationJsonLd(company(), ORIGIN);

    expect(ld).not.toHaveProperty('aggregateRating');
  });
});

describe('jobPostingJsonLd', () => {
  it('adds logo, skills, and experience/education requirements when present', () => {
    const ld = jobPostingJsonLd(
      postingJob({
        company: 'Acme',
        skills: ['Go', 'Kubernetes'],
        enrichment: { experience_years_min: 5, education_level: 'bachelor' },
      }),
      ORIGIN
    );

    expect((ld.hiringOrganization as Record<string, unknown>).logo).toBe(companyLogoUrl('Acme'));
    expect(ld.skills).toBe('Go, Kubernetes');
    expect(ld.experienceRequirements).toEqual({
      '@type': 'OccupationalExperienceRequirements',
      monthsOfExperience: 60,
    });
    expect(ld.educationRequirements).toEqual({
      '@type': 'EducationalOccupationalCredential',
      credentialCategory: 'bachelor degree',
    });
  });

  it('maps master and phd education to a postgraduate degree', () => {
    for (const level of ['master', 'phd']) {
      const ld = jobPostingJsonLd(postingJob({ enrichment: { education_level: level } }), ORIGIN);
      expect(ld.educationRequirements).toEqual({
        '@type': 'EducationalOccupationalCredential',
        credentialCategory: 'postgraduate degree',
      });
    }
  });

  it('omits logo, skills, and signal-free requirements', () => {
    const ld = jobPostingJsonLd(
      postingJob({
        company: '',
        skills: [],
        enrichment: { experience_years_min: 0, education_level: 'none' },
      }),
      ORIGIN
    );

    expect(ld.hiringOrganization).not.toHaveProperty('logo');
    expect(ld).not.toHaveProperty('skills');
    expect(ld).not.toHaveProperty('experienceRequirements');
    expect(ld).not.toHaveProperty('educationRequirements');
  });

  it('emits identifier from external_id, and omits it when absent', () => {
    const withId = jobPostingJsonLd(postingJob({ company: 'Acme', external_id: 'gh:42' }), ORIGIN);
    expect(withId.identifier).toEqual({ '@type': 'PropertyValue', name: 'Acme', value: 'gh:42' });

    expect(jobPostingJsonLd(postingJob(), ORIGIN)).not.toHaveProperty('identifier');
  });

  it('drops the leading colon of a boardless external_id in the identifier', () => {
    const ld = jobPostingJsonLd(postingJob({ external_id: ':https://x.dev/jobs/1' }), ORIGIN);
    expect((ld.identifier as Record<string, unknown>).value).toBe('https://x.dev/jobs/1');
  });

  it('sets validThrough to the close time for a closed posting', () => {
    const ld = jobPostingJsonLd(
      postingJob({ closed_at: '2026-02-01T00:00:00Z', last_seen_at: '2026-01-30T00:00:00Z' }),
      ORIGIN
    );
    expect(ld.validThrough).toBe('2026-02-01T00:00:00Z');
  });

  it('estimates validThrough 30 days out from last_seen_at for an open posting', () => {
    const ld = jobPostingJsonLd(postingJob({ last_seen_at: '2026-01-01T00:00:00Z' }), ORIGIN);
    expect(ld.validThrough).toBe('2026-01-31T00:00:00.000Z');
  });

  it('falls back to posted_at, then created_at, when last_seen_at is absent', () => {
    const byPosted = jobPostingJsonLd(
      postingJob({ posted_at: '2026-01-01T00:00:00Z', created_at: '2025-01-01T00:00:00Z' }),
      ORIGIN
    );
    expect(byPosted.validThrough).toBe('2026-01-31T00:00:00.000Z');

    const byCreated = jobPostingJsonLd(postingJob({ created_at: '2026-01-01T00:00:00Z' }), ORIGIN);
    expect(byCreated.validThrough).toBe('2026-01-31T00:00:00.000Z');
  });

  it('omits validThrough for an open posting with no date evidence at all', () => {
    expect(jobPostingJsonLd(postingJob(), ORIGIN)).not.toHaveProperty('validThrough');
  });
});

describe('collectionPageJsonLd', () => {
  const URL = 'https://freehire.me/collections/react';

  it('wraps the jobs in a CollectionPage → ItemList of summary ListItems', () => {
    const ld = collectionPageJsonLd(
      'React jobs',
      'Every open React role.',
      URL,
      jobListItems(
        [job('Senior React Engineer', 'senior-react-engineer-abc'), job('React Native Dev', 'react-native-dev-xyz')],
        ORIGIN
      )
    );

    expect(ld['@type']).toBe('CollectionPage');
    expect(ld.name).toBe('React jobs');
    expect(ld.description).toBe('Every open React role.');
    expect(ld.url).toBe(URL);
    expect(ld.mainEntity).toEqual({
      '@type': 'ItemList',
      itemListElement: [
        {
          '@type': 'ListItem',
          position: 1,
          name: 'Senior React Engineer',
          url: 'https://freehire.me/jobs/senior-react-engineer-abc',
        },
        {
          '@type': 'ListItem',
          position: 2,
          name: 'React Native Dev',
          url: 'https://freehire.me/jobs/react-native-dev-xyz',
        },
      ],
    });
  });

  it('emits an empty ItemList for a collection with no jobs', () => {
    const ld = collectionPageJsonLd('Empty', 'Nothing yet.', URL, []);

    expect(ld.mainEntity).toEqual({ '@type': 'ItemList', itemListElement: [] });
  });

  // The directory reuses the same CollectionPage with company items, so a company's
  // ListItem must point at /companies/<slug>, never the job path.
  it('wraps companies in the same CollectionPage shape', () => {
    const ld = collectionPageJsonLd(
      'Companies hiring in tech',
      'Browse companies hiring in tech.',
      'https://freehire.me/companies',
      companyListItems(
        [
          { name: 'Acme', slug: 'acme' },
          { name: 'Globex', slug: 'globex' },
        ],
        ORIGIN
      )
    );

    expect(ld['@type']).toBe('CollectionPage');
    expect(ld.mainEntity).toEqual({
      '@type': 'ItemList',
      itemListElement: [
        { '@type': 'ListItem', position: 1, name: 'Acme', url: 'https://freehire.me/companies/acme' },
        { '@type': 'ListItem', position: 2, name: 'Globex', url: 'https://freehire.me/companies/globex' },
      ],
    });
  });
});

describe('datasetJsonLd', () => {
  const URL = 'https://freehire.me/insights/salary/engineering';

  // The insights pages are aggregate-only: an empty `distribution` array would be a
  // dead end for a crawler, so the key must be absent rather than empty.
  it('omits distribution when the page advertises no endpoint', () => {
    const ld = datasetJsonLd('Engineering salaries', 'Bands by seniority.', URL, ORIGIN);

    expect(ld['@type']).toBe('Dataset');
    expect(ld.isAccessibleForFree).toBe(true);
    expect(ld).not.toHaveProperty('distribution');
  });

  it('advertises each endpoint as a JSON DataDownload', () => {
    const ld = datasetJsonLd('Live figures', 'Catalogue scale.', `${ORIGIN}/open`, ORIGIN, [
      { name: 'Jobs API', contentUrl: `${ORIGIN}/api/v1/jobs` },
    ]);

    expect(ld.distribution).toEqual([
      {
        '@type': 'DataDownload',
        name: 'Jobs API',
        encodingFormat: 'application/json',
        contentUrl: 'https://freehire.me/api/v1/jobs',
      },
    ]);
  });
});

// articleJsonLd reads a validated PostMeta.
function post(overrides: Partial<PostMeta> = {}): PostMeta {
  return {
    slug: 'launch',
    title: 'Launch',
    date: '2026-01-15',
    summary: 'We shipped it.',
    type: 'article',
    tags: ['product', 'launch'],
    draft: false,
    ...overrides,
  };
}

describe('articleJsonLd', () => {
  const ORIGIN = 'https://freehire.me';

  it('builds an Article with headline, description, date, url and keywords', () => {
    const ld = articleJsonLd(post(), ORIGIN);
    expect(ld['@type']).toBe('Article');
    expect(ld.headline).toBe('Launch');
    expect(ld.description).toBe('We shipped it.');
    expect(ld.datePublished).toBe('2026-01-15');
    expect(ld.url).toBe('https://freehire.me/blog/launch');
    expect(ld.keywords).toBe('product, launch');
  });

  it('omits keywords when the post has no tags', () => {
    const ld = articleJsonLd(post({ tags: [] }), ORIGIN);
    expect(ld.keywords).toBeUndefined();
  });
});
