// Pure presentation logic for the sidebar job-profile-match block. Kept out of the
// component so it is unit-testable (vitest) without a DOM: which of the four block
// states to render, and how to size the two-colour progress bar.

import type { Blocker, BlockerSeverity, JobMatch } from './types';

/** Split the hard-constraint blockers for display: the unmet ones (shown as warnings,
 *  hardest first — a lower score_cap is a harder blocker) and the met ones (shown as
 *  satisfied ✓). Pure, so it's unit-testable without a DOM. */
export function partitionBlockers(blockers: Blocker[] | null | undefined): {
  unmet: Blocker[];
  met: Blocker[];
} {
  const all = blockers ?? [];
  const unmet = all.filter((b) => !b.met).sort((a, b) => a.score_cap - b.score_cap);
  const met = all.filter((b) => b.met);
  return { unmet, met };
}

/** Warning tone by severity: hard constraints (work auth, certs) read as
 *  blocking, fit constraints (location, language) as softer cautions. Shared
 *  by JobMatch.svelte's own requirements list and ConfirmTailorDialog. */
export function toneText(severity: BlockerSeverity): string {
  if (severity === 'hard') return 'text-destructive';
  if (severity === 'medium') return 'text-warning-strong';
  return 'text-muted-foreground';
}

/** Skill-chip Tailwind classes by match kind — held, adjacent, or missing.
 *  Shared by JobMatch.svelte's skill rows and ConfirmTailorDialog. `inline-flex
 *  items-center gap-1` accommodates the optional SkillIcon each of those renders
 *  before the label — a no-op when a chip's value has no brand mark. */
const CHIP_BASE = 'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium';
export const haveChipClass = `${CHIP_BASE} border-brand/30 bg-brand-muted text-brand-strong`;
export const adjacentChipClass = `${CHIP_BASE} border-warning/30 bg-warning/10 text-warning-strong`;
export const missingChipClass = `${CHIP_BASE} border-destructive/30 bg-destructive/10 text-destructive-strong`;

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

/** A card-level match computed entirely in the browser: the exact overlap between a
 *  job's skills and the viewer's profile skills. No adjacency (that dictionary lives on
 *  the backend), so this is a lighter, exact-only signal than GET /jobs/:slug/match —
 *  the trade for computing it client-side, with zero requests per card. */
export type ClientMatch = { total: number; matched: number; percent: number };

/** Count how many of a job's skills the user already has (case-insensitive set
 *  intersection over canonical slugs) and the coverage percent. `total` is the job's
 *  skill count; a job with no skills is a zero match (no divide-by-zero). */
export function computeClientMatch(jobSkills: string[], profileSkills: string[]): ClientMatch {
  const have = new Set(profileSkills.map((s) => s.toLowerCase()));
  const total = jobSkills.length;
  const matched = jobSkills.filter((s) => have.has(s.toLowerCase())).length;
  const percent = total === 0 ? 0 : Math.round((matched / total) * 100);
  return { total, matched, percent };
}

/** The match as it reads once the viewer claims a skill they were missing, before the
 *  profile write settles. The skill joins the held group and leaves whichever group it
 *  came from; the counts and the coverage are recomputed by the server's own weighting
 *  (an exact match weighs 1, an adjacent one half), so the optimistic figure cannot drift
 *  from what `jobmatch.Compute` will answer next.
 *
 *  It is a strict under-estimate: the adjacency dictionary lives on the backend, so a
 *  claim that promotes some *other* missing skill to adjacent is invisible here. The block
 *  refetches once the write lands and that is where such a promotion shows up.
 *
 *  A skill this job does not carry, or one already held, yields the match untouched. */
export function claimSkill<M extends JobMatch>(match: M, skill: string): M {
  if (!match.missing.includes(skill) && !match.adjacent.some((a) => a.name === skill)) return match;

  const exact_count = match.exact_count + 1;
  const adjacent = match.adjacent.filter((a) => a.name !== skill);
  return {
    ...match,
    exact_count,
    adjacent_count: adjacent.length,
    coverage_percent: Math.round(((exact_count + 0.5 * adjacent.length) / match.total) * 100),
    matched: [...match.matched, skill],
    adjacent,
    missing: match.missing.filter((s) => s !== skill),
  };
}

/** The two progress-bar segment widths (in percent of the track): a full-weight
 *  green segment for exact matches and a half-weight warning segment for adjacent
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
