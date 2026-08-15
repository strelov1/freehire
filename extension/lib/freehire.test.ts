import { describe, it, expect } from 'vitest';
import { freehireSlugFromUrl, resolveNotice, apiErrorMessage, partitionBlockers, type Blocker } from './freehire';

describe('freehireSlugFromUrl', () => {
  it('extracts the slug from a freehire job URL', () => {
    expect(freehireSlugFromUrl('https://freehire.me/jobs/backend-engineer-stripe-abc')).toBe(
      'backend-engineer-stripe-abc',
    );
  });

  it('works on the dev host', () => {
    expect(freehireSlugFromUrl('http://localhost:5173/jobs/go-dev-acme-t35nijto')).toBe(
      'go-dev-acme-t35nijto',
    );
  });

  it('ignores sub-paths under a job', () => {
    expect(freehireSlugFromUrl('https://freehire.me/jobs/abc/similar')).toBeNull();
  });

  it('ignores non-job freehire pages', () => {
    expect(freehireSlugFromUrl('https://freehire.me/companies')).toBeNull();
  });

  it('ignores foreign hosts even with a /jobs/ path', () => {
    expect(freehireSlugFromUrl('https://jobright.ai/jobs/whatever')).toBeNull();
  });

  it('returns null for junk input', () => {
    expect(freehireSlugFromUrl('not a url')).toBeNull();
  });
});

describe('apiErrorMessage', () => {
  it("carries the server's own sentence, which is the whole diagnosis", () => {
    // /me/autofill/run answers 409 for three unrelated states; only this
    // sentence says which one.
    expect(
      apiErrorMessage('/api/v1/me/autofill/run', 409, '{"error":"the browser extension is not connected"}'),
    ).toBe('the browser extension is not connected (HTTP 409)');
  });

  it('distinguishes the other states behind the same status', () => {
    expect(apiErrorMessage('/api/v1/me/autofill/run', 409, '{"error":"no fillable fields on this page"}')).toBe(
      'no fillable fields on this page (HTTP 409)',
    );
  });

  it('falls back to the path when the body says nothing', () => {
    expect(apiErrorMessage('/api/v1/me/autofill-profile', 500, '')).toBe(
      '/api/v1/me/autofill-profile → HTTP 500',
    );
  });

  it('falls back when a proxy answers with something that is not our JSON', () => {
    expect(apiErrorMessage('/api/v1/jobs/find', 502, '<html>Bad Gateway</html>')).toBe(
      '/api/v1/jobs/find → HTTP 502',
    );
  });

  it('falls back when the JSON carries no error field', () => {
    expect(apiErrorMessage('/api/v1/jobs/find', 404, '{"data":null}')).toBe('/api/v1/jobs/find → HTTP 404');
  });
});

describe('resolveNotice', () => {
  it('tells the user a posting was imported', () => {
    expect(resolveNotice('imported')).toMatch(/added/i);
  });

  it('tells the user the posting was already in the catalog', () => {
    expect(resolveNotice('found')).toMatch(/already/i);
  });

  it('tells the user an unreadable page went to a maintainer', () => {
    expect(resolveNotice('queued')).toMatch(/look/i);
  });

  it('distinguishes a company freehire already follows from a new one', () => {
    // Both imported the posting, so both must confirm it landed — but only one of them can
    // promise the company's other roles will appear, and saying that of an uncrawled company
    // would be a promise we cannot keep until its board is onboarded.
    const tracked = resolveNotice('tracked');
    expect(tracked).toMatch(/added/i);
    expect(tracked).toMatch(/already/i);
    expect(tracked).not.toBe(resolveNotice('imported'));
  });

  it('stays generic for a status it does not know', () => {
    expect(resolveNotice('something-new' as never)).toBeTruthy();
  });
});

describe('partitionBlockers', () => {
  const b = (category: string, severity: Blocker['severity'], score_cap: number, met: boolean): Blocker => ({
    category,
    severity,
    score_cap,
    reason: `${category} reason`,
    action: '',
    met,
  });

  it('splits unmet from met and orders unmet hardest-first', () => {
    const { unmet, met } = partitionBlockers([
      b('location_work_mode', 'soft', 75, false),
      b('certification', 'hard', 60, false),
      b('education', 'medium', 65, true),
    ]);
    expect(unmet.map((x) => x.category)).toEqual(['certification', 'location_work_mode']);
    expect(met.map((x) => x.category)).toEqual(['education']);
  });

  it('is empty for an empty list', () => {
    expect(partitionBlockers([])).toEqual({ unmet: [], met: [] });
  });

  it('is empty when blockers is undefined, as POST /me/match-text sends (no blockers field at all)', () => {
    expect(partitionBlockers(undefined)).toEqual({ unmet: [], met: [] });
  });
});
