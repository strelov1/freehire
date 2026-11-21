import { describe, it, expect } from 'vitest';
import { filtersToParams } from './facetModel';
import { filtersFromProfile } from './profileFilters';
import type { UserProfile } from './types';
import sharedCases from './fixtures/profileFeedParity.json';

// The Go feed holds the same cases (internal/search/feed/profilequery_test.go): the feed's
// server-side profile query and "Apply my profile" here must emit the same params.
describe('profile feed parity', () => {
  it('has the shared cases', () => expect(sharedCases.length).toBeGreaterThanOrEqual(12));

  it.each(sharedCases)('$name', ({ profile, params }) => {
    const got = filtersToParams(filtersFromProfile(profile as unknown as UserProfile));
    expect(Object.fromEntries(got)).toEqual(params);
    // One value per key, so the object comparison sees every emitted value.
    expect([...got.keys()].length).toBe(Object.keys(params).length);
  });
});
