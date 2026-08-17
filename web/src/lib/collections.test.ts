import { describe, it, expect } from 'vitest';
import {
  FILTER_COLLECTIONS,
  POPULAR_COLLECTION_FALLBACK,
  collectionBySlug,
  collectionSlugs,
  jobFacetsFromJob,
  popularCollectionLinks,
  relatedCollectionLinks,
  relatedCollectionSlugs,
} from './collections';
import type { Job } from './types';
import { FACETS } from './facets';

// Minimal JobFacets fixture — every field defaults to "no signal" so a test
// only has to set the facet(s) it cares about.
function jobFacets(overrides: Partial<Parameters<typeof relatedCollectionSlugs>[0]> = {}) {
  return {
    skills: [],
    countries: [],
    regions: [],
    collections: [],
    ...overrides,
  };
}

describe('collectionBySlug', () => {
  it('resolves a company-membership collection to a collections facet param', () => {
    const c = collectionBySlug('yc');
    expect(c).toBeDefined();
    expect(c?.title).toBe('Y Combinator');
    expect(c?.params).toEqual({ collections: 'yc' });
  });

  it('resolves a filter collection to its own facet params', () => {
    const c = collectionBySlug('remote-worldwide');
    expect(c).toBeDefined();
    expect(c?.title).toBe('Remote Worldwide');
    expect(c?.params).toEqual({ work_mode: 'remote', regions: 'global' });
  });

  it('returns undefined for an unknown slug', () => {
    expect(collectionBySlug('does-not-exist')).toBeUndefined();
  });
});

describe('collectionSlugs', () => {
  it('lists every collection slug across both registries with no duplicates', () => {
    const slugs = collectionSlugs();
    expect(slugs).toContain('yc'); // company collection
    expect(slugs).toContain('remote-worldwide'); // filter collection
    expect(new Set(slugs).size).toBe(slugs.length);
  });
});

describe('FILTER_COLLECTIONS invariants', () => {
  it('every filter collection has non-empty params', () => {
    // params is the single source of a card's count and the landing page's scoped
    // feed; an empty map would render the bare /jobs feed under a collection URL.
    for (const c of FILTER_COLLECTIONS) {
      expect(Object.keys(c.params).length, `filter collection "${c.slug}"`).toBeGreaterThan(0);
    }
  });

  it('every filter collection pins only known job-search facet params', () => {
    // A mistyped param key (e.g. `skill` for `skills`, `catgory` for `category`) is
    // silently ignored by the search, so the landing page would render an unfiltered
    // feed. Guard it against the same facet-param set the filter UI drives.
    const known = new Set(FACETS.map((f) => f.param));
    for (const c of FILTER_COLLECTIONS) {
      for (const key of Object.keys(c.params)) {
        expect(known.has(key), `filter collection "${c.slug}" param "${key}"`).toBe(true);
      }
    }
  });
});

describe('credential collections in the job-search facet', () => {
  const collectionFacets = FACETS.filter((f) => f.param === 'collections');

  it('occupies exactly one facet entry', () => {
    // Two entries sharing a query param would break the facet machinery, not just
    // the layout: filtersToParams iterates FACETS and would append every selected
    // value to the URL twice, and activeFilterCount would double the badge. The
    // credential/collection split is therefore a grouping inside one facet, not a
    // second facet — this test is what stops someone splitting it later.
    expect(collectionFacets).toHaveLength(1);
  });

  it('offers every sponsor credential, grouped apart from editorial collections', () => {
    const options = collectionFacets[0]?.options ?? [];
    const credentials = options.filter((o) => o.group);
    expect(credentials.map((o) => o.value)).toEqual([
      'uk-skilled-worker-sponsor',
      'nl-recognised-sponsor',
      'us-h1b-sponsor',
    ]);
    for (const c of credentials) {
      expect(c.group).toBe('Employer credentials');
    }
  });

  it('keeps editorial collections ungrouped and ahead of the credentials', () => {
    const options = collectionFacets[0]?.options ?? [];
    const firstGrouped = options.findIndex((o) => o.group);
    expect(firstGrouped).toBeGreaterThan(0);
    expect(options.slice(0, firstGrouped).every((o) => !o.group)).toBe(true);
    expect(options.slice(firstGrouped).every((o) => o.group)).toBe(true);
  });

  it('resolves a credential slug to a landing page scoped by the collections facet', () => {
    const resolved = collectionBySlug('uk-skilled-worker-sponsor');
    expect(resolved?.params).toEqual({ collections: 'uk-skilled-worker-sponsor' });
    expect(resolved?.title).toBe('Licensed UK sponsor');
    // The disclaimer travels with the copy: a landing page must not read as a
    // promise that any listed role is sponsored.
    expect(resolved?.description).toMatch(/not a commitment to sponsor/i);
  });

  it('lists credential slugs in the sitemap alongside every other collection', () => {
    expect(collectionSlugs()).toContain('uk-skilled-worker-sponsor');
    expect(collectionSlugs()).toContain('nl-recognised-sponsor');
  });
});

describe('POPULAR_COLLECTION_FALLBACK', () => {
  it('every fallback slug resolves to a real collection', () => {
    // The fallback pool pads a sparse "see also" block — a typo here would
    // silently link to a page that 404s.
    for (const slug of POPULAR_COLLECTION_FALLBACK) {
      expect(collectionBySlug(slug), `fallback slug "${slug}"`).toBeDefined();
    }
  });
});

describe('relatedCollectionSlugs', () => {
  it('matches a job facet against FILTER_COLLECTIONS (Source A)', () => {
    const slugs = relatedCollectionSlugs(jobFacets({ skills: ['react'] }));
    expect(slugs).toContain('react');
  });

  it('matches the job collections field against the company registry (Source B)', () => {
    const slugs = relatedCollectionSlugs(jobFacets({ collections: ['yc'] }));
    expect(slugs).toContain('yc');
  });

  it('never matches on role — the job wire object carries no role field', () => {
    // software-engineer is a `role`-keyed FILTER_COLLECTIONS entry; nothing in
    // JobFacets can satisfy it, so it must never appear from a facet match.
    const slugs = relatedCollectionSlugs(
      jobFacets({ category: 'backend', seniority: 'senior', skills: ['react'] })
    );
    expect(slugs).not.toContain('software-engineer');
  });

  it('matches category, seniority, and region facets, not just skills', () => {
    // Each of facetMatches' non-skills branches asserted positively, not just
    // exercised as a side effect of another test.
    expect(relatedCollectionSlugs(jobFacets({ category: 'backend' }))).toContain('backend');
    expect(relatedCollectionSlugs(jobFacets({ seniority: 'senior' }))).toContain('senior');
    expect(
      relatedCollectionSlugs(jobFacets({ workMode: 'remote', regions: ['global'] }))
    ).toContain('remote-worldwide');
  });

  it('requires every param on a multi-key entry (AND across keys, not OR)', () => {
    // remote-brasil = { work_mode: 'remote', countries: 'br' }. Satisfying only
    // one of its two keys must not match — this is the property that would
    // silently break if collectionMatchesJob's `.every()` became `.some()`.
    expect(relatedCollectionSlugs(jobFacets({ workMode: 'remote' }))).not.toContain(
      'remote-brasil'
    );
    expect(relatedCollectionSlugs(jobFacets({ countries: ['br'] }))).not.toContain(
      'remote-brasil'
    );
    expect(
      relatedCollectionSlugs(jobFacets({ workMode: 'remote', countries: ['br'] }))
    ).toContain('remote-brasil');
  });

  it('orders facet matches (Source A) before collection matches (Source B)', () => {
    const slugs = relatedCollectionSlugs(jobFacets({ skills: ['react'], collections: ['yc'] }));
    const reactIndex = slugs.indexOf('react');
    const ycIndex = slugs.indexOf('yc');
    expect(reactIndex).toBeGreaterThanOrEqual(0);
    expect(ycIndex).toBeGreaterThanOrEqual(0);
    expect(reactIndex).toBeLessThan(ycIndex);
  });

  it('de-duplicates a slug that both matches a facet and sits in the fallback pool', () => {
    // 'javascript' is in POPULAR_COLLECTION_FALLBACK; a job that also matches
    // it by skill must not render it twice.
    const slugs = relatedCollectionSlugs(jobFacets({ skills: ['javascript'] }));
    expect(slugs.filter((s) => s === 'javascript')).toHaveLength(1);
  });

  it('pads with the popular fallback when a job matches nothing', () => {
    const slugs = relatedCollectionSlugs(jobFacets());
    expect(slugs).toEqual(POPULAR_COLLECTION_FALLBACK);
  });

  it('never renders more than the requested target size', () => {
    const slugs = relatedCollectionSlugs(
      jobFacets({ category: 'backend', seniority: 'senior', skills: ['react', 'python', 'aws'] }),
      3
    );
    expect(slugs.length).toBeLessThanOrEqual(3);
  });

  it('never exceeds the available collection pool and never links a dead slug', () => {
    const slugs = relatedCollectionSlugs(jobFacets({ skills: ['react'] }), 1000);
    expect(new Set(slugs).size).toBe(slugs.length); // no duplicates
    for (const slug of slugs) {
      expect(collectionBySlug(slug), `related slug "${slug}"`).toBeDefined();
    }
  });
});

describe('relatedCollectionLinks', () => {
  it('resolves each slug to its collection title, for JobSeeAlso to render', () => {
    const links = relatedCollectionLinks(jobFacets({ skills: ['react'] }));
    expect(links).toContainEqual({ slug: 'react', title: 'React' });
  });

  it('resolves both filter and company collection kinds', () => {
    const links = relatedCollectionLinks(jobFacets({ collections: ['yc'] }));
    expect(links).toContainEqual({ slug: 'yc', title: 'Y Combinator' });
  });

  it('falls back to the popular collections when a job matches nothing', () => {
    const links = relatedCollectionLinks(jobFacets());
    expect(links.map((l) => l.slug)).toEqual(POPULAR_COLLECTION_FALLBACK);
    expect(links.every((l) => typeof l.title === 'string' && l.title.length > 0)).toBe(true);
  });

  it('produces the same slugs, in the same order, as relatedCollectionSlugs', () => {
    const job = jobFacets({ skills: ['react'], collections: ['yc'] });
    expect(relatedCollectionLinks(job).map((l) => l.slug)).toEqual(relatedCollectionSlugs(job));
  });
});

describe('jobFacetsFromJob', () => {
  it('reads category/seniority from enrichment, the rest from top-level Job fields', () => {
    const job = {
      skills: ['react'],
      work_mode: 'remote',
      countries: ['us'],
      regions: ['global'],
      collections: ['yc'],
      enrichment: { category: 'backend', seniority: 'senior' },
    } as Job;

    expect(jobFacetsFromJob(job)).toEqual({
      category: 'backend',
      seniority: 'senior',
      skills: ['react'],
      workMode: 'remote',
      countries: ['us'],
      regions: ['global'],
      collections: ['yc'],
    });
  });

  it('leaves category/seniority undefined when the job carries neither', () => {
    const job = {
      skills: [],
      countries: [],
      regions: [],
      collections: [],
      enrichment: {},
    } as unknown as Job;

    const facets = jobFacetsFromJob(job);
    expect(facets.category).toBeUndefined();
    expect(facets.seniority).toBeUndefined();
    expect(facets.workMode).toBeUndefined();
  });
});

describe('popularCollectionLinks', () => {
  // The gap this closes: the homepage — the site's strongest page — carried 20
  // /jobs/* links, 4 /features/* and zero links to any individual collection, so
  // the 101 collection landing pages got nothing from it. Footer links reach every
  // page, not only the homepage.
  it('resolves every popular slug to a title', () => {
    const links = popularCollectionLinks();
    expect(links.length).toBeGreaterThan(0);
    for (const link of links) {
      expect(link.title, `title for "${link.slug}"`).toBeTruthy();
    }
  });

  // A link to a page that 404s is worse than no link. collectionBySlug is the same
  // resolver the landing route and the sitemap use, so this cannot drift from them.
  it('never emits a slug that has no landing page', () => {
    for (const link of popularCollectionLinks()) {
      expect(collectionBySlug(link.slug), `slug "${link.slug}"`).toBeDefined();
    }
  });

  it('keeps the curated order and holds no duplicates', () => {
    const slugs = popularCollectionLinks().map((l) => l.slug);
    expect(slugs).toEqual(POPULAR_COLLECTION_FALLBACK.filter((s) => collectionBySlug(s)));
    expect(new Set(slugs).size).toBe(slugs.length);
  });
});
