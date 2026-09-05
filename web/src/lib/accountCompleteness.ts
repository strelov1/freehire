// What "how complete is my account" means, decided in one place.
//
// The first four steps are the onboarding wizard's own (cv, confirm, skills, location),
// so the card after the funnel measures exactly what the funnel asked for and the two
// cannot drift. The fifth is the one the reviewer did not name and the product needs: an
// alert is what makes jobs arrive without a visit, and a completeness meter that ends at
// a filled-in form measures paperwork rather than activation.
//
// Pure by design (no Svelte, no network): the caller passes what it has already loaded,
// which is also why this needs no endpoint of its own.

import type { UserProfile } from './types';

/** Where a step is completed. A literal union rather than `string` because SvelteKit's
 *  `resolve()` is typed against the real route table — so a step pointing at a page that
 *  does not exist fails the build here, instead of rendering a link to a 404. Spelling
 *  the routes out keeps this module free of any SvelteKit import, which is what lets it
 *  be unit-tested in plain Node. */
type SetupHref = '/my/cvs' | '/my/profile' | '/my/searches';

export interface CompletenessStep {
  /** Stable key. Used by tests and as the list key — never shown. */
  id: string;
  label: string;
  href: SetupHref;
  done: boolean;
}

export interface CompletenessInput {
  /** Whether a CV is stored (resumeStore.present). */
  hasCv: boolean;
  /** The saved profile, or null when the user has none yet. */
  profile: UserProfile | null;
  /** How many search alerts the user has. Only "any" matters; the count is what the
   *  caller already holds, so it is not narrowed to a boolean here. */
  alertCount: number;
}

/** Whether the location block says anything at all.
 *
 *  Presence is not an answer: the API returns an empty block for a user who opened the
 *  step and saved nothing, so testing `!== null` would mark the step done for someone
 *  who never stated a preference. Any ONE of the three parts counts — the block is
 *  deliberately a free combination, and demanding all of them would hold the card open
 *  on a candidate who is simply remote-only with nowhere in particular to be. */
function statesLocation(profile: UserProfile | null): boolean {
  const loc = profile?.location_preferences;
  if (!loc) return false;
  return Boolean(
    loc.work_modes?.length ||
      loc.base?.country ||
      loc.base?.city ||
      loc.remote?.regions?.length ||
      loc.remote?.countries?.length,
  );
}

/** The setup steps and whether each is done, in the order they are asked for. */
export function accountSteps({ hasCv, profile, alertCount }: CompletenessInput): CompletenessStep[] {
  return [
    {
      id: 'cv',
      label: 'Add your CV',
      href: '/my/cvs',
      done: hasCv,
    },
    {
      // One step, not two: a specialization without a level and a level without a
      // specialization are both halves of the same answer, and the wizard asks them on
      // one screen.
      id: 'role',
      label: 'Say what you do, and at what level',
      href: '/my/profile',
      done: Boolean(profile?.specializations.length && profile?.seniorities.length),
    },
    {
      id: 'skills',
      label: 'List your skills',
      href: '/my/profile',
      done: Boolean(profile?.skills.length),
    },
    {
      id: 'location',
      label: 'Set where and how you want to work',
      href: '/my/profile',
      done: statesLocation(profile),
    },
    {
      id: 'alerts',
      label: 'Get new matches sent to you',
      href: '/my/searches',
      done: alertCount > 0,
    },
  ];
}

/** The steps still open. Zero of them means the card and its dot are done showing.
 *
 *  Takes the steps rather than the input so "what counts as outstanding" is written once
 *  and every caller shares it — the card needs both the full list and this subset, and
 *  would otherwise re-filter on `!done` itself. */
export function outstandingOf(steps: CompletenessStep[]): CompletenessStep[] {
  return steps.filter((step) => !step.done);
}
