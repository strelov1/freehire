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
export const missingChipClass = `${CHIP_BASE} border-destructive/30 bg-destructive/10 text-destructive`;

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

/** The locked-state teaser: plausible figures for a match nobody has computed yet,
 *  shown blurred to a guest (or a signed-in viewer with no skills) as an invitation
 *  rather than an estimate. `{total, matched, percent}` is the `ClientMatch` shape, so
 *  the same bar renders it; `missing` names which of the job's own skills read as
 *  not-held, leaving each surface free to show as many chips as it has room for. */
export type MatchTeaser = ClientMatch & { missing: Set<string> };

/** FNV-1a (32-bit). Seeds the teaser from a job's slug so the same job yields the same
 *  figures in the SSR pass and after hydration — `Math.random()` would re-roll on both,
 *  flipping the score on first paint and again on every re-render of the feed. */
function hashSeed(seed: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < seed.length; i++) {
    h = Math.imul(h ^ seed.charCodeAt(i), 0x01000193);
  }
  return h >>> 0;
}

/** mulberry32 — a small seeded PRNG, enough to shuffle a handful of skill names. */
function mulberry32(seed: number): () => number {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = Math.imul(a ^ (a >>> 15), a | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const TEASER_MIN = 60;
const TEASER_MAX = 90;

/** Derive the teaser for one job. Null when the job has fewer than two skills: there is
 *  no have/missing story to tell about a single one, and "1 of 1 skills" beside an 87%
 *  bar is the one figure a viewer could catch out. Those jobs keep the plain card corner
 *  and the sidebar's bare call-to-action.
 *
 *  `matched` follows from `percent` rather than being rolled beside it, so the "N of M
 *  skills" label can never contradict the bar. What the seed actually randomises is
 *  *which* skills read as missing, so the red chips scatter through the row.
 *
 *  The band's top is capped per skill count: at five skills, 90% would round to 5 of 5
 *  and leave an all-green row under a bar that isn't full. Keeping the percent below
 *  `100 − 50/total` guarantees one missing skill, so both tones always show. */
export function matchTeaser(seed: string, jobSkills: string[]): MatchTeaser | null {
  const total = jobSkills.length;
  if (total < 2) return null;

  const h = hashSeed(seed);
  // The largest whole percent that still rounds below `total` — Math.ceil(x) - 1 is the
  // greatest integer strictly less than x, for a fractional x and an exact one alike.
  const ceiling = Math.min(TEASER_MAX, Math.ceil(100 - 50 / total) - 1);
  const percent = TEASER_MIN + (h % (ceiling - TEASER_MIN + 1));
  const matched = Math.round((percent / 100) * total);

  // Give every skill a seeded sort key and take the lowest `total - matched` of them as
  // the missing ones — a shuffle without the index juggling, so which skills read as
  // missing scatters through the row instead of always being its tail.
  const rand = mulberry32(h);
  const missing = new Set(
    jobSkills
      .map((name) => ({ name, key: rand() }))
      .toSorted((a, b) => a.key - b.key)
      .slice(0, total - matched)
      .map((s) => s.name),
  );

  return { total, matched, percent, missing };
}

/** Which skills a narrow teaser row should show. Takes the job's leading skills, but if
 *  that window happens to be all-held it trades its last chip for the first missing skill
 *  further down — a three-chip row cut from a job with one missing skill would otherwise
 *  come out uniformly green, losing the have/missing contrast that is the teaser's whole
 *  point. The job's own order is preserved among the chips that stay. */
export function teaserChips(jobSkills: string[], missing: Set<string>, limit: number): string[] {
  const shown = jobSkills.slice(0, limit);
  // A one-chip row has nothing to spare: trading its only chip would leave it all-missing,
  // which inverts the contrast rather than showing it.
  if (shown.length > 1 && shown.length < jobSkills.length && !shown.some((s) => missing.has(s))) {
    const borrowed = jobSkills.find((s) => missing.has(s));
    if (borrowed) return [...shown.slice(0, -1), borrowed];
  }
  return shown;
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
