import { describe, expect, it } from 'vitest';

import type { CatalogueMember } from './generated/contracts';
import { talentHeading, talentPlace } from './talentCard';

const member = (over: Partial<CatalogueMember> = {}): CatalogueMember => ({
  handle: 'backend-7f2a',
  card: { skills: [], roles: [] },
  cities: [],
  specializations: [],
  updated_at: '2026-09-01T12:00:00Z',
  ...over,
});

describe('talentHeading', () => {
  it('joins the two dictionary values', () => {
    expect(talentHeading('senior', 'backend', 'Candidate')).toBe('Senior Backend');
  });

  // classify never guesses, so either half can be absent. A member whose title resolved
  // to nothing still needs a heading rather than an empty line.
  it('uses whichever half resolved', () => {
    expect(talentHeading('senior', undefined, 'Candidate')).toBe('Senior');
    expect(talentHeading(undefined, 'backend', 'Candidate')).toBe('Backend');
  });

  it('falls back when neither resolved', () => {
    expect(talentHeading(undefined, undefined, 'Candidate')).toBe('Candidate');
    expect(talentHeading('', '', 'Role')).toBe('Role');
  });

  // Both label maps are exception tables — SENIORITY_LABELS carries exactly one entry —
  // so anything absent must fall through to titleCase, not to the raw slug. Reading them
  // as complete dictionaries rendered headings as "senior backend".
  it('title-cases a value the label map does not carry', () => {
    expect(talentHeading(undefined, 'data_engineering', 'Candidate')).toBe('Data Engineering');
  });

  it('keeps the one label the map overrides', () => {
    expect(talentHeading('c_level', undefined, 'Candidate')).toBe('C-level');
  });
});

describe('talentPlace', () => {
  it('reads the country from the timezone, not the city', () => {
    // Asia/Calcutta is the case the table exists for: a renamed zone whose name says
    // nothing about its country, and the largest single group in this data.
    expect(talentPlace(member({ timezone: 'Asia/Calcutta' })).country).toBe('in');
  });

  it('drops the region prefix and restores the underscores', () => {
    expect(talentPlace(member({ timezone: 'America/New_York' })).zone).toBe('New York');
  });

  it('has no country for a zone that names an offset', () => {
    expect(talentPlace(member({ timezone: 'Etc/GMT-3' })).country).toBeUndefined();
  });

  it('is empty rather than undefined for a member with no cities', () => {
    expect(talentPlace(member()).place).toBe('');
  });

  // Labelled, not raw. Without this a card reads "berlin" beside the timezone's own
  // "Berlin", which looks like a bug in the half that is correct.
  it('labels the normalised city slugs', () => {
    expect(talentPlace(member({ cities: ['berlin', 'new-york'] })).place).toBe('Berlin, New York');
  });
});
