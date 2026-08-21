import { describe, it, expect } from 'vitest';
import { geoScopeQuery, isCrawler, regionFromCountry, shouldOfferGeoScope } from './geoScope';

describe('regionFromCountry', () => {
  it('maps a placeable country to its region', () => {
    expect(regionFromCountry('BR')).toBe('latam');
    expect(regionFromCountry('DE')).toBe('eu');
  });

  it('accepts a lower-case code', () => {
    expect(regionFromCountry('br')).toBe('latam');
  });

  it('ignores surrounding whitespace', () => {
    expect(regionFromCountry(' DE ')).toBe('eu');
  });

  // Cloudflare's two reserved values. Both are shaped exactly like a country code
  // and neither is one — XX is "could not place it", T1 is a Tor exit.
  it('rejects the reserved edge values', () => {
    expect(regionFromCountry('XX')).toBeNull();
    expect(regionFromCountry('T1')).toBeNull();
  });

  it('rejects a country the region grouping does not carry', () => {
    expect(regionFromCountry('ZZ')).toBeNull();
  });

  it('rejects a missing header', () => {
    expect(regionFromCountry(null)).toBeNull();
    expect(regionFromCountry(undefined)).toBeNull();
    expect(regionFromCountry('')).toBeNull();
  });
});

describe('isCrawler', () => {
  it('recognizes the crawlers that dominate this site traffic', () => {
    expect(isCrawler('Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)')).toBe(true);
    expect(isCrawler('Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; ClaudeBot/1.0)')).toBe(true);
    expect(isCrawler('Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)')).toBe(true);
    expect(isCrawler('Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)')).toBe(true);
    expect(isCrawler('Mozilla/5.0 (compatible; SemrushBot/7~bl)')).toBe(true);
    expect(isCrawler('Mozilla/5.0 (compatible; YandexBot/3.0)')).toBe(true);
  });

  it('recognizes the generic shapes', () => {
    expect(isCrawler('some-crawler/1.0')).toBe(true);
    expect(isCrawler('BigSpider')).toBe(true);
    expect(isCrawler('Yahoo! Slurp')).toBe(true);
  });

  it('leaves a real browser alone', () => {
    expect(
      isCrawler(
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
      ),
    ).toBe(false);
    expect(isCrawler('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Safari/604.1')).toBe(
      false,
    );
  });

  // The scheduled Lighthouse watchdog must take the same path a first-time visitor
  // takes, or it measures a page nobody is served — see the change's design notes.
  it('does not mistake the Lighthouse watchdog for a crawler', () => {
    expect(
      isCrawler(
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Chrome-Lighthouse',
      ),
    ).toBe(false);
  });

  it('treats a missing user agent as a browser', () => {
    expect(isCrawler(null)).toBe(false);
    expect(isCrawler('')).toBe(false);
  });
});

describe('shouldOfferGeoScope', () => {
  const clean = { search: '', storedFilters: '', offered: false };

  it('offers to a browser that fails none of the guards', () => {
    expect(shouldOfferGeoScope(clean)).toBe(true);
  });

  it('stands aside for geography already in the URL', () => {
    expect(shouldOfferGeoScope({ ...clean, search: '?regions=eu' })).toBe(false);
  });

  // Any param, not only a geographic one: a link built to mean "senior jobs" must
  // show its recipient what its sender saw.
  it('stands aside for any other param in the URL', () => {
    expect(shouldOfferGeoScope({ ...clean, search: '?seniority=senior' })).toBe(false);
  });

  it('stands aside for a stored filter set', () => {
    expect(shouldOfferGeoScope({ ...clean, storedFilters: 'regions=eu' })).toBe(false);
  });

  // The stored set wins on the strength of being theirs, not on what it contains.
  it('stands aside for a stored set that names no geography', () => {
    expect(shouldOfferGeoScope({ ...clean, storedFilters: 'seniority=senior' })).toBe(false);
  });

  // The case the separate marker exists for: clearing the filters removes
  // hire.jobFilters, so this state is indistinguishable from a fresh browser
  // WITHOUT the marker — and the guess would undo the clear on every visit.
  it('does not re-impose itself after the visitor cleared the scope', () => {
    expect(shouldOfferGeoScope({ search: '', storedFilters: '', offered: true })).toBe(false);
  });
});

describe('geoScopeQuery', () => {
  it('applies the region alongside worldwide', () => {
    expect(geoScopeQuery('latam')).toBe('regions=latam,global');
  });
});
