import { describe, expect, it } from 'vitest';
import { isLegacyFeedUrl } from './legacyFeedUrl';

const q = (search: string) => new URLSearchParams(search);

describe('isLegacyFeedUrl', () => {
  it('serves the landing for a bare homepage', () => {
    expect(isLegacyFeedUrl(q(''))).toBe(false);
  });

  it('serves the landing for the params the homepage itself reads', () => {
    // Every guarded page bounces a signed-out visitor here to sign in. Redirecting
    // these would drop them into a job list instead of the dialog they were sent for.
    expect(isLegacyFeedUrl(q('auth=required&redirect=%2Fmy%2Ftracking'))).toBe(false);
    expect(isLegacyFeedUrl(q('auth_error=1'))).toBe(false);
    expect(isLegacyFeedUrl(q('ref=abc123'))).toBe(false);
  });

  it('serves the landing for campaign tails nothing reads', () => {
    expect(isLegacyFeedUrl(q('utm_source=twitter&utm_medium=social'))).toBe(false);
    expect(isLegacyFeedUrl(q('gclid=xyz'))).toBe(false);
    expect(isLegacyFeedUrl(q('fbclid=xyz'))).toBe(false);
  });

  it('redirects free text and facets', () => {
    expect(isLegacyFeedUrl(q('q=golang'))).toBe(true);
    expect(isLegacyFeedUrl(q('work_mode=remote'))).toBe(true);
    expect(isLegacyFeedUrl(q('category=backend&seniority=senior'))).toBe(true);
    expect(isLegacyFeedUrl(q('skills_exclude=java'))).toBe(true);
  });

  it('redirects the params that address the feed without filtering it', () => {
    expect(isLegacyFeedUrl(q('page=2'))).toBe(true);
    expect(isLegacyFeedUrl(q('sort=recent'))).toBe(true);
  });

  it('redirects search params the browser filter model does not serialise', () => {
    // The loader this replaced forwarded the query verbatim to the search API, so `/`
    // honoured every parameter that API accepts — not only the ones the filter store
    // can reproduce. These are exactly the URLs a model-derived predicate would have
    // dropped in silence.
    expect(isLegacyFeedUrl(q('is_tech=true'))).toBe(true);
    expect(isLegacyFeedUrl(q('salary_max=200000'))).toBe(true);
    expect(isLegacyFeedUrl(q('experience_years_min=3'))).toBe(true);
    expect(isLegacyFeedUrl(q('education_level=bachelor'))).toBe(true);
    expect(isLegacyFeedUrl(q('order=asc'))).toBe(true);
  });

  it('redirects a value the model rejects but the API tolerates', () => {
    // `positiveDays` refuses this; the server does not. A predicate built on the model
    // would read it as "no filters here" and serve the landing.
    expect(isLegacyFeedUrl(q('posted_within_days=1e2'))).toBe(true);
  });

  it('redirects when a filter travels beside a campaign tail', () => {
    expect(isLegacyFeedUrl(q('utm_source=newsletter&category=backend'))).toBe(true);
  });
});
