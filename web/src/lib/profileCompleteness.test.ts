import { describe, it, expect } from 'vitest';
import { getProfileCompleteness, PROFILE_SECTION_TOTAL } from './profileCompleteness';
import type { LocationPreferences, ResumeMeta, UserProfile } from './types';

// The section predicates are the whole point of the module, so they are asserted one at
// a time: a section that silently reads as filled is a profile that claims to be complete
// when a candidate still has to write it.

const profile = (patch: Partial<UserProfile> = {}): UserProfile => ({
  specializations: [],
  skills: [],
  seniorities: [],
  excluded_skills: [],
  excluded_sources: [],
  excluded_companies: [],
  location_preferences: null,
  derived_location: null,
  cv: null,
  created_at: null,
  updated_at: null,
  ...patch,
});

const resume = (patch: Partial<ResumeMeta> = {}): ResumeMeta => ({
  enabled: true,
  present: true,
  uploaded_at: null,
  structured: null,
  contacts: null,
  ...patch,
});

/** The "Where you're based" block the job filter reads: a structured country + city. */
const locationPrefs = (patch: Partial<LocationPreferences> = {}): LocationPreferences => ({
  remote: {},
  base: { country: 'PT', city: 'Lisbon' },
  relocation: { open: false },
  ...patch,
});

const fullProfile = (patch: Partial<UserProfile> = {}): UserProfile =>
  profile({
    skills: ['go', 'sql'],
    cv: { experience: [{ title: 'Engineer' }] },
    ...patch,
  });

const fullResume = resume({
  contacts: {
    full_name: 'Ada Lovelace',
    email: 'ada@example.com',
    education: [{ degree: 'BSc Computer Science' }],
    summary: 'Backend engineer.',
  },
});

const fullScreening = { notice_period_days: 30 };

const complete = (input: Parameters<typeof getProfileCompleteness>[0]) =>
  getProfileCompleteness(input);

describe('getProfileCompleteness', () => {
  it('reports an empty profile as 0 of 6, with location false', () => {
    const marks = complete({ profile: null });
    expect(marks).toMatchObject({
      contacts: false,
      location: false,
      skills: false,
      experience: false,
      education: false,
      screening: false,
      summary: false,
    });
    expect(marks.filled).toBe(0);
    expect(marks.total).toBe(PROFILE_SECTION_TOTAL);
    expect(marks.total).toBe(6);
  });

  // Location reports its real value but is not one of the counted sections, so stating
  // it alone moves the badge and not the count.
  it('reports a location-only profile as location true but still 0 of 6', () => {
    const marks = complete({ profile: profile({ location_preferences: locationPrefs() }) });
    expect(marks.location).toBe(true);
    expect(marks.filled).toBe(0);
    expect(marks.total).toBe(6);
  });

  it('reports a full profile without a location as 6 of 6, location false', () => {
    const marks = complete({
      profile: fullProfile(),
      resume: fullResume,
      screening: fullScreening,
    });
    expect(marks).toMatchObject({
      contacts: true,
      location: false,
      skills: true,
      experience: true,
      education: true,
      screening: true,
      summary: true,
    });
    expect(marks.filled).toBe(6);
    expect(marks.total).toBe(6);
  });

  it('reports a full profile with a location as 6 of 6, location true', () => {
    const marks = complete({
      profile: fullProfile({ location_preferences: locationPrefs() }),
      resume: fullResume,
      screening: fullScreening,
    });
    expect(marks.location).toBe(true);
    expect(marks.filled).toBe(6);
    expect(marks.total).toBe(6);
  });

  it('needs BOTH the structured country and city for a location', () => {
    const countryOnly = complete({
      profile: profile({ location_preferences: locationPrefs({ base: { country: 'PT' } }) }),
    });
    const cityOnly = complete({
      profile: profile({ location_preferences: locationPrefs({ base: { city: 'Lisbon' } }) }),
    });
    expect(countryOnly.location).toBe(false);
    expect(cityOnly.location).toBe(false);
  });

  // The contact block already carries a free-text place, so it is a valid fallback when
  // the structured block states no base.
  it('falls back to the contact block free-text location', () => {
    const marks = complete({
      profile: profile(),
      resume: resume({ contacts: { location: 'Lisbon, Portugal' } }),
    });
    expect(marks.location).toBe(true);
    expect(marks.filled).toBe(0);
  });

  it('reports a partial profile without counting the sections it has not reached', () => {
    const marks = complete({
      profile: profile({ skills: ['go'] }),
      resume: resume({ contacts: { full_name: 'Ada Lovelace' } }), // no email yet
    });
    expect(marks.contacts).toBe(false);
    expect(marks.skills).toBe(true);
    expect(marks.experience).toBe(false);
    expect(marks.education).toBe(false);
    expect(marks.screening).toBe(false);
    expect(marks.summary).toBe(false);
    expect(marks.filled).toBe(1); // skills only — location is uncounted
  });

  it('needs BOTH a name and an email for contacts, from the contacts block or the CV parse', () => {
    const parsed = complete({
      profile: profile(),
      resume: resume({
        structured: { full_name: 'Ada Lovelace', email: 'ada@example.com' },
      }),
    });
    expect(parsed.contacts).toBe(true);

    const nameOnly = complete({
      profile: profile(),
      resume: resume({ contacts: { full_name: 'Ada Lovelace' } }),
    });
    expect(nameOnly.contacts).toBe(false);
  });

  it('counts experience and education from one entry, never from an empty list', () => {
    const none = complete({
      profile: profile({ cv: { experience: [], education: [] } }),
    });
    expect(none.experience).toBe(false);
    expect(none.education).toBe(false);

    const oneEach = complete({
      profile: profile({ cv: { experience: [{ title: 'Engineer' }], education: [{ degree: 'BSc' }] } }),
    });
    expect(oneEach.experience).toBe(true);
    expect(oneEach.education).toBe(true);
  });

  it('treats a stated false as a screening answer', () => {
    expect(complete({ profile: profile(), screening: {} }).screening).toBe(false);
    expect(complete({ profile: profile(), screening: { willing_to_relocate: false } }).screening).toBe(
      true,
    );
  });

  it('never counts the avoid lists as a missing section', () => {
    const marks = complete({
      profile: profile({
        excluded_skills: ['java'],
        excluded_sources: ['adzuna'],
        excluded_companies: ['acme'],
      }),
    });
    expect(marks.filled).toBe(0);
    expect(Object.keys(marks)).not.toContain('avoid');
  });
});
