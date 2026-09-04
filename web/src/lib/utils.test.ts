import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { formatCount, formatDate, formatDateOrAgo } from './utils';

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
  const at = (hoursAgo: number) =>
    new Date(NOW.getTime() - hoursAgo * 3600 * 1000).toISOString();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
  });
  afterEach(() => vi.useRealTimers());

  it('reads as an age inside the last day', () => {
    expect(formatDateOrAgo(at(0.5))).toBe('30 minutes ago');
    expect(formatDateOrAgo(at(3))).toBe('3 hours ago');
  });

  // The switch is at exactly 24h: "yesterday" is where the relative form stops
  // beating the date, so the date must already be showing when it would be said.
  it('switches to the date at the day boundary', () => {
    expect(formatDateOrAgo(at(23))).toBe('23 hours ago');
    expect(formatDateOrAgo(at(24))).toBe(formatDate(at(24)));
    expect(formatDateOrAgo(at(72))).toBe(formatDate(at(72)));
  });

  // Clock skew between a source's stated date and ours would otherwise print
  // "in 2 hours" as a posting's age.
  it('gives a future timestamp the date, not an age', () => {
    expect(formatDateOrAgo(at(-2))).toBe(formatDate(at(-2)));
  });

  it('has nothing to say about a missing or unparseable timestamp', () => {
    expect(formatDateOrAgo(null)).toBe('');
    expect(formatDateOrAgo(undefined)).toBe('');
    expect(formatDateOrAgo('not a date')).toBe('');
  });
});
