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
    }
  });
});
