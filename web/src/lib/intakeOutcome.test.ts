import { describe, it, expect } from 'vitest';
import { intakeOutcomeMessage } from './intakeOutcome';
import type { ResolvedLink } from './types';

const link = (over: Partial<ResolvedLink>): ResolvedLink => ({
  public_slug: null,
  status: 'queued',
  ...over,
});

describe('intakeOutcomeMessage', () => {
  it('tells a known posting apart from a newly added one', () => {
    expect(intakeOutcomeMessage(link({ status: 'found' }))).toMatch(/already have/i);
    expect(intakeOutcomeMessage(link({ status: 'tracked' }))).toMatch(/^Added/);
  });

  it('separates a new company from a new board of a company we carry', () => {
    const newCompany = intakeOutcomeMessage(link({ status: 'imported' }));
    const knownCompany = intakeOutcomeMessage(
      link({ status: 'imported', company_slug: 'acme' }),
    );
    expect(newCompany).toMatch(/new to us/i);
    expect(knownCompany).toMatch(/already carry this company/i);
  });

  it('does not promise a crawl for a careers site nothing recognised', () => {
    expect(intakeOutcomeMessage(link({ status: 'review' }))).toMatch(/by hand/i);
  });

  it('reads an unreadable page as "we will look", not as a refusal', () => {
    const msg = intakeOutcomeMessage(link({ status: 'queued' }));
    expect(msg).toMatch(/couldn't read/i);
    expect(msg).toMatch(/by hand/i);
  });

  it('promises no reward — the currency it was paid in no longer exists', () => {
    for (const status of ['found', 'tracked', 'imported', 'review', 'queued'] as const) {
      expect(intakeOutcomeMessage(link({ status }))).not.toMatch(/credit/i);
      expect(intakeOutcomeMessage(link({ status }), 'ru')).not.toMatch(/кредит/i);
    }
  });

  it('defaults to English, so the public search box needs no locale', () => {
    expect(intakeOutcomeMessage(link({ status: 'found' }))).toBe(
      intakeOutcomeMessage(link({ status: 'found' }), 'en'),
    );
  });

  it('answers in Russian, keeping the five outcomes distinct', () => {
    const ru = (over: Partial<ResolvedLink>) => intakeOutcomeMessage(link(over), 'ru');
    const all = [
      ru({ status: 'found' }),
      ru({ status: 'tracked' }),
      ru({ status: 'imported' }),
      ru({ status: 'imported', company_slug: 'acme' }),
      ru({ status: 'review' }),
      ru({ status: 'queued' }),
    ];
    // Every outcome is translated (no English left over) and none collapsed onto
    // another — a copy/paste that merged two outcomes would read fine in isolation.
    for (const msg of all) expect(msg).toMatch(/[А-Яа-яЁё]/);
    expect(new Set(all).size).toBe(all.length);
  });

  it('falls back to English for a locale nothing has been translated into', () => {
    expect(intakeOutcomeMessage(link({ status: 'found' }), 'de')).toBe(
      intakeOutcomeMessage(link({ status: 'found' }), 'en'),
    );
  });
});
