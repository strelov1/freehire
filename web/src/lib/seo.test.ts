import { describe, expect, it } from 'vitest';
import { articleJsonLd, collectionPageJsonLd, jobPostingJsonLd, organizationJsonLd } from './seo';
import { logoDevUrl } from './logo';
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
    ...overrides,
  };
}

const ORIGIN = 'https://freehire.dev';

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
    expect(ld.url).toBe('https://freehire.dev/companies/acme');
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
      url: 'https://freehire.dev/companies/acme',
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

    expect((ld.hiringOrganization as Record<string, unknown>).logo).toBe(logoDevUrl('Acme'));
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
});

describe('collectionPageJsonLd', () => {
  const URL = 'https://freehire.dev/collections/react';

  it('wraps the jobs in a CollectionPage → ItemList of summary ListItems', () => {
    const ld = collectionPageJsonLd(
      'React jobs',
      'Every open React role.',
      URL,
      [job('Senior React Engineer', 'senior-react-engineer-abc'), job('React Native Dev', 'react-native-dev-xyz')],
      ORIGIN
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
          url: 'https://freehire.dev/jobs/senior-react-engineer-abc',
        },
        {
          '@type': 'ListItem',
          position: 2,
          name: 'React Native Dev',
          url: 'https://freehire.dev/jobs/react-native-dev-xyz',
        },
      ],
    });
  });

  it('emits an empty ItemList for a collection with no jobs', () => {
    const ld = collectionPageJsonLd('Empty', 'Nothing yet.', URL, [], ORIGIN);

    expect(ld.mainEntity).toEqual({ '@type': 'ItemList', itemListElement: [] });
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
  const ORIGIN = 'https://freehire.dev';

  it('builds an Article with headline, description, date, url and keywords', () => {
    const ld = articleJsonLd(post(), ORIGIN);
    expect(ld['@type']).toBe('Article');
    expect(ld.headline).toBe('Launch');
    expect(ld.description).toBe('We shipped it.');
    expect(ld.datePublished).toBe('2026-01-15');
    expect(ld.url).toBe('https://freehire.dev/blog/launch');
    expect(ld.keywords).toBe('product, launch');
  });

  it('omits keywords when the post has no tags', () => {
    const ld = articleJsonLd(post({ tags: [] }), ORIGIN);
    expect(ld.keywords).toBeUndefined();
  });
});
