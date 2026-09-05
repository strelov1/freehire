import { describe, it, expect } from 'vitest';
import { formatPeriodDate, formatPeriodRange } from './periodDate';

describe('formatPeriodDate', () => {
  it('formats undefined as empty', () => {
    expect(formatPeriodDate(undefined)).toBe('');
  });

  it('formats a year-only date', () => {
    expect(formatPeriodDate({ year: 2018 })).toBe('2018');
  });

  it('formats a month-and-year date', () => {
    expect(formatPeriodDate({ year: 2018, month: 3 })).toBe('Mar 2018');
  });

  it('treats a non-positive year as absent, matching the Go side', () => {
    expect(formatPeriodDate({ year: 0 })).toBe('');
  });
});

describe('formatPeriodRange', () => {
  it('joins two present sides with an en dash', () => {
    expect(formatPeriodRange({ year: 2018, month: 3 }, { year: 2021 })).toBe('Mar 2018 – 2021');
  });

  it('renders current as Present regardless of end', () => {
    expect(formatPeriodRange({ year: 2018, month: 10 }, undefined, true)).toBe('Oct 2018 – Present');
  });

  it('renders just the start when there is no end and not current', () => {
    expect(formatPeriodRange({ year: 2018 }, undefined)).toBe('2018');
  });

  it('renders empty when neither side is set', () => {
    expect(formatPeriodRange(undefined, undefined)).toBe('');
  });
});
