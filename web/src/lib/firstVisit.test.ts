import { describe, it, expect } from 'vitest';
import { isCrawler, defaultFilterTarget } from './firstVisit';
import { DEFAULT_JOB_FILTERS } from './filterStorage';

describe('isCrawler', () => {
  it('flags common search-engine and social bots', () => {
    for (const ua of [
      'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
      'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)',
      'Mozilla/5.0 (compatible; YandexBot/3.0)',
      'facebookexternalhit/1.1',
      'Twitterbot/1.0',
      'Mozilla/5.0 (compatible; AhrefsBot/7.0)',
      'DuckDuckBot/1.1',
    ]) {
      expect(isCrawler(ua)).toBe(true);
    }
  });

  it('treats a real browser UA as not a crawler', () => {
    expect(
      isCrawler(
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36',
      ),
    ).toBe(false);
  });

  it('treats a missing UA as not a crawler (real users may omit it; bots we care about send one)', () => {
    expect(isCrawler(null)).toBe(false);
    expect(isCrawler('')).toBe(false);
  });
});

describe('defaultFilterTarget', () => {
  const target = `/?${DEFAULT_JOB_FILTERS}`;

  it('redirects a brand-new visitor on a bare homepage to the default filter slice', () => {
    expect(defaultFilterTarget({ search: '', touched: false, crawler: false })).toBe(target);
  });

  it('does not redirect when the URL already carries filters (a shared link)', () => {
    expect(defaultFilterTarget({ search: '?regions=cis', touched: false, crawler: false })).toBeNull();
    // even when the shared link happens to equal the default
    expect(
      defaultFilterTarget({ search: `?${DEFAULT_JOB_FILTERS}`, touched: false, crawler: false }),
    ).toBeNull();
  });

  it('does not redirect once the visitor has changed filters (cookie set, incl. a clear)', () => {
    expect(defaultFilterTarget({ search: '', touched: true, crawler: false })).toBeNull();
  });

  it('does not redirect crawlers, so the homepage still indexes the full catalogue', () => {
    expect(defaultFilterTarget({ search: '', touched: false, crawler: true })).toBeNull();
  });
});
