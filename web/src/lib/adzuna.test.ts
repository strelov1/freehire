import { describe, expect, it } from 'vitest';
import { ADZUNA_FALLBACK_DOMAIN, adzunaLocalDomain } from './adzuna';

describe('adzunaLocalDomain', () => {
  it('credits the country domain the posting itself links to', () => {
    // The four domains our stored Adzuna rows actually use, including the two-part .com.au.
    const cases: Record<string, string> = {
      'https://www.adzuna.com/jobs/land/ad/5804941790?se=x': 'https://www.adzuna.com',
      'https://www.adzuna.co.uk/jobs/land/ad/5660339691': 'https://www.adzuna.co.uk',
      'https://www.adzuna.de/jobs/land/ad/1': 'https://www.adzuna.de',
      'https://www.adzuna.com.au/jobs/land/ad/2': 'https://www.adzuna.com.au',
    };
    for (const [url, want] of Object.entries(cases)) {
      expect(adzunaLocalDomain(url)).toBe(want);
    }
  });

  it('falls back to the domain the terms name when the URL cannot supply one', () => {
    for (const url of ['', 'not a url', '/jobs/land/ad/1']) {
      expect(adzunaLocalDomain(url)).toBe(ADZUNA_FALLBACK_DOMAIN);
    }
  });

  it('never credits a non-Adzuna host', () => {
    // A stored row whose link stopped being an Adzuna one must not turn a required credit into
    // a link somewhere else — including a host that merely ends in the brand name.
    for (const url of [
      'https://example.com/jobs/1',
      'https://notadzuna.com/jobs/1',
      'https://adzuna.com.evil.test/jobs/1',
      'javascript:alert(1)//adzuna.co.uk',
    ]) {
      expect(adzunaLocalDomain(url)).toBe(ADZUNA_FALLBACK_DOMAIN);
    }
  });
});
