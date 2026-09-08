import { describe, expect, it } from 'vitest';

import {
  MAX_FILTER_TERMS,
  MAX_LIMIT,
  readTalentQuery,
  talentFilterSearch,
  talentPageSearch,
  writeTalentQuery,
} from './talentQuery';

const read = (search: string) => readTalentQuery(new URLSearchParams(search));

describe('readTalentQuery', () => {
  it('reads every filter', () => {
    expect(
      read(
        'categories=backend,devops&seniorities=senior&skills=go,postgresql&tz=Europe&cities=berlin&specializations=platform&min_years=5&limit=10&offset=20',
      ),
    ).toEqual({
      categories: ['backend', 'devops'],
      seniorities: ['senior'],
      skills: ['go', 'postgresql'],
      tz: ['Europe'],
      cities: ['berlin'],
      specializations: ['platform'],
      minYears: 5,
      limit: 10,
      offset: 20,
    });
  });

  it('treats an absent filter and an empty one identically', () => {
    expect(read('categories=&skills=,,&cities=%20')).toEqual({});
  });

  it('trims and drops blank terms', () => {
    expect(read('skills=%20go%20,,%20postgresql%20,').skills).toEqual(['go', 'postgresql']);
  });

  it('caps a filter list rather than refusing it', () => {
    const many = Array.from({ length: MAX_FILTER_TERMS + 10 }, (_, i) => `skill${i}`).join(',');
    expect(read(`skills=${many}`).skills).toHaveLength(MAX_FILTER_TERMS);
  });

  // Omitted, not clamped. The API answers a limit it cannot read by widening the answer
  // and saying so in meta.ignored_params — so forwarding one would show an unfiltered
  // catalogue behind filter chips that claim otherwise.
  it('omits unusable paging so the API applies its own default', () => {
    expect(read('limit=0').limit).toBeUndefined();
    expect(read(`limit=${MAX_LIMIT + 1}`).limit).toBeUndefined();
    expect(read('limit=abc').limit).toBeUndefined();
    expect(read('limit=12abc').limit).toBeUndefined();
    expect(read('offset=-1').offset).toBeUndefined();
    expect(read('min_years=lots').minYears).toBeUndefined();
    // The ceiling mirrors the Go one exactly. A tighter bound here would drop a filter
    // the API would have read, and the visitor would see a wider catalogue than their
    // own chips claim, with nothing in meta.ignored_params to explain it.
    expect(read('min_years=60').minYears).toBe(60);
    expect(read('min_years=61').minYears).toBeUndefined();
  });
});

describe('writeTalentQuery', () => {
  it('omits a cleared filter rather than writing it empty', () => {
    expect(writeTalentQuery({ categories: [], skills: ['go'] })).toBe('skills=go');
  });

  it('omits a zero offset, which is the default', () => {
    expect(writeTalentQuery({ offset: 0 })).toBe('');
    expect(writeTalentQuery({ offset: 24 })).toBe('offset=24');
  });

  it('round-trips through readTalentQuery', () => {
    const query = {
      categories: ['backend'],
      skills: ['go', 'postgresql'],
      tz: ['Europe'],
      minYears: 5,
      limit: 24,
      offset: 48,
    };
    expect(read(writeTalentQuery(query))).toEqual(query);
  });
});

describe('talentPageSearch', () => {
  it('keeps the filters and moves only the offset', () => {
    expect(talentPageSearch({ categories: ['backend'], offset: 0 }, 24)).toBe(
      'categories=backend&offset=24',
    );
  });

  it('is empty for the bare first page, so the caller links to the plain path', () => {
    expect(talentPageSearch({}, 0)).toBe('');
  });
});

describe('talentFilterSearch', () => {
  // Narrowing means "show me these", not "show me page four of these" — an offset kept
  // across a filter change lands on an empty page that looks like no matches.
  it('resets paging when a filter changes', () => {
    expect(talentFilterSearch({ offset: 48 }, 'categories', ['backend'])).toBe(
      'categories=backend',
    );
  });

  it('clears a filter when handed no values', () => {
    expect(talentFilterSearch({ categories: ['backend'], skills: ['go'] }, 'categories', [])).toBe(
      'skills=go',
    );
  });
});
