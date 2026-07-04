// Pure presentation logic for the sidebar job-profile-match block. Kept out of the
// component so it is unit-testable (vitest) without a DOM: which of the four block
// states to render, and how to size the two-colour progress bar.

/** Which state the match block renders. `loading` is the brief window while an
 *  authenticated viewer's profile is still being fetched — shown before we know
 *  whether it is `no-profile` or `ready`, so the CTA doesn't flash. */
export type MatchState = 'no-skills' | 'guest' | 'loading' | 'no-profile' | 'ready';

/** Resolve the block state from what the page already knows. `no-skills` wins over
 *  everything (nothing personal to show), then the auth gate, then the profile gate. */
export function resolveMatchState(input: {
  jobSkills: string[];
  authenticated: boolean;
  profileLoaded: boolean;
  profileSkills: string[] | null | undefined;
}): MatchState {
  if (input.jobSkills.length === 0) return 'no-skills';
  if (!input.authenticated) return 'guest';
  if (!input.profileLoaded) return 'loading';
  if (!input.profileSkills || input.profileSkills.length === 0) return 'no-profile';
  return 'ready';
}

/** The two progress-bar segment widths (in percent of the track): a full-weight
 *  green segment for exact matches and a half-weight amber segment for adjacent
 *  ones. Their sum is the (unrounded) coverage percent. */
export function matchBarSegments(m: {
  total: number;
  exact_count: number;
  adjacent_count: number;
}): { exact: number; adjacent: number } {
  if (m.total <= 0) return { exact: 0, adjacent: 0 };
  return {
    exact: (m.exact_count / m.total) * 100,
    adjacent: ((0.5 * m.adjacent_count) / m.total) * 100,
  };
}
