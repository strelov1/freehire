import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { formatCount, formatDate, formatDateOrAgo, formatDateTime, timeAgo } from './utils';

// formatCount lives here rather than in activityChart because its callers share nothing
// with a chart module: two axis labels (activity bars, skill pulse) and the job card's
// view count. The card also imports timeAgo from here for the very same header rail, so
// the two rail helpers sit together.
describe('formatCount', () => {
  it('leaves counts under a thousand alone', () => {
    expect(formatCount(0)).toBe('0');
    expect(formatCount(842)).toBe('842');
  });

  // 999 → 1000 is the first threshold, and it is the one a view count crosses daily.
  it('abbreviates at the thousand boundary and not before', () => {
    expect(formatCount(999)).toBe('999');
    expect(formatCount(1000)).toBe('1K');
  });

  it('keeps one decimal below a hundred thousand', () => {
    expect(formatCount(1240)).toBe('1.2K');
    expect(formatCount(3400)).toBe('3.4K');
  });

  // Above 1e5 the decimal is dropped, so 99999 rounds up into a bare 100K by the
  // one-decimal branch and 100000 reaches the same string by the zero-decimal one.
  // Both spellings must agree, or the label would jump backwards across the boundary.
  it('drops the decimal from a hundred thousand up', () => {
    expect(formatCount(99999)).toBe('100K');
    expect(formatCount(100000)).toBe('100K');
    expect(formatCount(697191)).toBe('697K');
  });

  it('switches to millions at a million', () => {
    expect(formatCount(999999)).toBe('1000K');
    expect(formatCount(1000000)).toBe('1M');
    expect(formatCount(3354251)).toBe('3.4M');
  });

  // A trailing ".0" is trimmed rather than shown, so a round figure reads "1K" and
  // never "1.0K".
  it('trims a trailing zero decimal', () => {
    expect(formatCount(2000)).toBe('2K');
    expect(formatCount(2000000)).toBe('2M');
  });
});

// The job header prints a posting's two timestamps through this one helper, so the
// day boundary it switches on is the whole of its behaviour. Time is faked rather
// than measured: a test that builds "23 hours ago" from the real clock is a test
// that fails at whatever hour the boundary lands on.
describe('formatDateOrAgo', () => {
  const NOW = new Date('2026-09-04T12:00:00Z');
  const at = (hoursAgo: number) => new Date(NOW.getTime() - hoursAgo * 3600 * 1000).toISOString();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
  });
  afterEach(() => vi.useRealTimers());

  it('reads as an age inside the last day', () => {
    expect(formatDateOrAgo(at(0.5), 'en')).toBe('30 minutes ago');
    expect(formatDateOrAgo(at(3), 'en')).toBe('3 hours ago');
  });

  // The switch is at exactly 24h: "yesterday" is where the relative form stops
  // beating the date, so the date must already be showing when it would be said.
  it('switches to the date at the day boundary', () => {
    expect(formatDateOrAgo(at(23), 'en')).toBe('23 hours ago');
    expect(formatDateOrAgo(at(24), 'en')).toBe(formatDate(at(24), 'en'));
    expect(formatDateOrAgo(at(72), 'en')).toBe(formatDate(at(72), 'en'));
  });

  // Clock skew between a source's stated date and ours would otherwise print
  // "in 2 hours" as a posting's age.
  it('gives a future timestamp the date, not an age', () => {
    expect(formatDateOrAgo(at(-2), 'en')).toBe(formatDate(at(-2), 'en'));
  });

  it('has nothing to say about a missing or unparseable timestamp', () => {
    expect(formatDateOrAgo(null, 'en')).toBe('');
    expect(formatDateOrAgo(undefined, 'en')).toBe('');
    expect(formatDateOrAgo('not a date', 'en')).toBe('');
  });

  // The short style is for a label an icon has already named, so the unit may be
  // abbreviated but the NUMBER and the direction must survive. Asserted by shape rather
  // than by string: `Intl` owns the abbreviation, and pinning "30 min. ago" here would be
  // this repo asserting a CLDR spelling it does not control.
  it('abbreviates the unit in the short style without losing the number', () => {
    const long = formatDateOrAgo(at(0.5), 'en');
    const short = formatDateOrAgo(at(0.5), 'en', 'short');
    expect(short).toContain('30');
    expect(short.length).toBeLessThan(long.length);
    expect(timeAgo(at(3), 'en', 'short')).toContain('3');
  });

  // Past the day boundary the short style has nothing to shorten — it is a date either
  // way, and a date the reader compares against another posting's must not be abbreviated
  // out from under them.
  it('leaves the date branch alone', () => {
    expect(formatDateOrAgo(at(24), 'en', 'short')).toBe(formatDate(at(24), 'en'));
  });

  it('defaults to the long style', () => {
    expect(formatDateOrAgo(at(3), 'en')).toBe(timeAgo(at(3), 'en'));
  });
});

// The locale argument is required (no default) precisely so every call site is forced
// through the compiler — see fix-my-account-date-locale/design.md. These tests exercise
// the behavior that requirement exists for: the same instant reads differently depending
// on which locale is passed, never on the test runner's own default locale.
describe('locale-aware date formatting', () => {
  const NOW = new Date('2026-09-04T12:00:00Z');
  const at = (hoursAgo: number) => new Date(NOW.getTime() - hoursAgo * 3600 * 1000).toISOString();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
  });
  afterEach(() => vi.useRealTimers());

  // Asserted by absence of Latin letters rather than a pinned CLDR string, the same
  // shape-not-spelling rule the short-style test above already follows — a Russian
  // month/relative-unit name has none, an English one is nothing but.
  const NO_LATIN = /^[^a-zA-Z]*$/;

  it('formats an absolute date in the given locale', () => {
    const en = formatDate(at(48), 'en');
    const ru = formatDate(at(48), 'ru');
    expect(en).toMatch(/[a-zA-Z]/);
    expect(ru).toMatch(NO_LATIN);
    expect(ru).not.toBe(en);
  });

  it('formats a date-time in the given locale', () => {
    const en = formatDateTime(at(48), 'en');
    const ru = formatDateTime(at(48), 'ru');
    expect(en).toMatch(/[a-zA-Z]/);
    expect(ru).toMatch(NO_LATIN);
    expect(ru).not.toBe(en);
  });

  it('formats a relative time in the given locale, in both styles', () => {
    expect(timeAgo(at(3), 'en')).toBe('3 hours ago');
    expect(timeAgo(at(3), 'ru')).toMatch(NO_LATIN);
    expect(timeAgo(at(3), 'en', 'short')).toMatch(/[a-zA-Z]/);
    expect(timeAgo(at(3), 'ru', 'short')).toMatch(NO_LATIN);
  });

  // Regression guard for the formatter cache: constructing an Intl.RelativeTimeFormat
  // is deliberately memoized (see utils.ts), so a call under one locale must not leave a
  // stale formatter behind for the next call under a different locale — interleaved here
  // in the order a header notification bell would hit it across a client-side navigation.
  it('does not let one locale leak into another through the cached formatter', () => {
    expect(timeAgo(at(3), 'en')).toBe('3 hours ago');
    expect(timeAgo(at(3), 'ru')).toMatch(NO_LATIN);
    expect(timeAgo(at(3), 'en')).toBe('3 hours ago');
  });
});
