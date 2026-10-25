import { describe, expect, it } from 'vitest';
import { buddyHeading, buddyRelativeDay, buddySharedCompanies, localDay } from './buddyPresent';

describe('buddyHeading', () => {
  it('builds the Talent Network heading from the summary', () => {
    expect(buddyHeading({ seniority: 'senior', category: 'backend' })).toBe('Senior Backend');
  });
  it('falls back to Member when neither value resolves', () => {
    expect(buddyHeading({ seniority: '', category: '' })).toBe('Member');
  });
});

describe('buddyRelativeDay', () => {
  const today = '2026-10-25';
  it('names today and yesterday', () => {
    expect(buddyRelativeDay('2026-10-25', today)).toBe('today');
    expect(buddyRelativeDay('2026-10-24', today)).toBe('yesterday');
  });
  it('counts days within two weeks, across a month boundary', () => {
    expect(buddyRelativeDay('2026-10-22', today)).toBe('3 days ago');
    expect(buddyRelativeDay('2026-11-02', '2026-11-05')).toBe('3 days ago');
    expect(buddyRelativeDay('2026-10-11', today)).toBe('14 days ago');
  });
  it('falls back to the date beyond two weeks', () => {
    expect(buddyRelativeDay('2026-10-10', today)).toBe('Oct 10');
    expect(buddyRelativeDay('2025-12-30', '2026-01-20')).toBe('Dec 30, 2025');
  });
  it('reads a day ahead of the viewer as today', () => {
    expect(buddyRelativeDay('2026-10-26', today)).toBe('today');
  });
  it('returns an unreadable day unchanged', () => {
    expect(buddyRelativeDay('soon', today)).toBe('soon');
  });
});

describe('localDay', () => {
  it('formats the local calendar day', () => {
    expect(localDay(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05');
  });
});

describe('buddySharedCompanies', () => {
  it('phrases the count', () => {
    expect(buddySharedCompanies(0)).toBe('');
    expect(buddySharedCompanies(1)).toBe('1 company in common');
    expect(buddySharedCompanies(3)).toBe('3 companies in common');
  });
});
