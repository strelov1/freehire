// The examples the jobs search box types into its own placeholder while it sits empty.
//
// The list is category KEYS, not display strings. A hand-written ['QA', 'DevOps'] would
// be a second copy of a vocabulary the backend already owns, and it would rot silently —
// a category renamed or retired upstream would leave the box confidently offering a value
// the feed can no longer filter on. Typing the array as Category[] turns that into a
// build failure, the same guard CATEGORY_LABELS uses for the same reason.
//
// Only the TAIL moves. The prefix is a fixed string the component never touches, so the
// sentence stays readable throughout and the eye has one place to look. That is why this
// module hands over the two parts rather than composed strings: the animation needs to
// know where the fixed half ends.
//
// Pure by design (no Svelte, no DOM), like facetModel.ts and suggestions.ts beside it —
// the stepping below is the whole animation, and it is testable without a browser.

import type { Category } from './generated/contracts';
import { categoryLabel } from './labels';

/** The half of the placeholder that never changes. */
export const ROLE_PREFIX = 'Search jobs — e.g. ';

/** The roles the box types, in order.
 *
 *  Busiest-looking first: under prefers-reduced-motion nothing is animated, so the first
 *  entry is the only one anyone sees and carries the whole weight of answering "what can
 *  I type here". Six is about what a visitor will sit through before typing. */
export const PLACEHOLDER_ROLES: Category[] = [
  'backend',
  'frontend',
  'devops',
  'qa',
  'data_science',
  'product',
];

/** The fixed prefix and the words that follow it, for the jobs search box.
 *
 *  A constant, not a factory. The labels are a pure lookup over a frozen list, so every
 *  call would return an equal object with a new identity — and a caller that recomputes
 *  it (the header re-derives its wording on every navigation) would hand the box a
 *  "changed" prop, restarting the animation's pending delay on each page change. */
export const ROLE_PLACEHOLDER: { prefix: string; roles: string[] } = {
  prefix: ROLE_PREFIX,
  roles: PLACEHOLDER_ROLES.map(categoryLabel),
};

// ---- the typing animation ----------------------------------------------------------
//
// Per-character, because that is what reads as "you type here" rather than as a banner
// cycling at you. Deleting runs faster than typing: a person backspaces faster than they
// compose, and matching the two makes the erase feel like a stall.

/** How long one typed character is on screen before the next arrives. */
export const TYPE_MS = 55;
/** …and one deleted character. */
export const DELETE_MS = 28;
/** How long the finished word stays whole before it is erased. Long enough to read it
 *  and to notice the box is showing an example rather than holding a stale query. */
export const HOLD_MS = 1500;

export interface TypewriterState {
  /** Index into the roles list. */
  role: number;
  /** How many of that role's characters are shown. */
  chars: number;
  /** Whether the next character is removed rather than added. */
  deleting: boolean;
}

/** Where the animation begins: the FIRST role already whole.
 *
 *  Not an empty tail. The server renders this state, and an empty tail would ship
 *  `"Search jobs — e.g. "` — a sentence that stops mid-air for anyone reading before
 *  hydration or with JavaScript off. Starting complete also means the first thing anyone
 *  reads is a whole example, and the animation's first act is to erase it rather than to
 *  correct a dangling line. */
export function typewriterStart(roles: string[]): TypewriterState {
  return { role: 0, chars: (roles[0] ?? '').length, deleting: false };
}

/** The visible tail for a state: the current role, cut to the characters typed so far. */
export function typedTail(state: TypewriterState, roles: string[]): string {
  return (roles[state.role] ?? '').slice(0, state.chars);
}

/** The next state, and how long to wait before applying it.
 *
 *  The delay belongs to the step it precedes rather than the one that produced it, which
 *  is what lets the finished word hold: the step that removes its first character is the
 *  one that waits HOLD_MS, so the whole word stays on screen for that long. */
export function typewriterStep(
  state: TypewriterState,
  roles: string[],
): { next: TypewriterState; delayMs: number } {
  const word = roles[state.role] ?? '';
  const nextRole = (state.role + 1) % Math.max(roles.length, 1);

  // A role whose label resolved to nothing has no characters to type or delete. Stepping
  // past it beats decrementing below zero, which would render `slice(0, -1)` — the
  // previous word minus a letter, which reads as a glitch rather than a missing label.
  if (word.length === 0) {
    return { next: { role: nextRole, chars: 0, deleting: false }, delayMs: TYPE_MS };
  }

  if (!state.deleting) {
    if (state.chars < word.length) {
      return { next: { ...state, chars: state.chars + 1 }, delayMs: TYPE_MS };
    }
    return { next: { ...state, chars: word.length - 1, deleting: true }, delayMs: HOLD_MS };
  }

  if (state.chars > 0) {
    return { next: { ...state, chars: state.chars - 1 }, delayMs: DELETE_MS };
  }
  return { next: { role: nextRole, chars: 1, deleting: false }, delayMs: TYPE_MS };
}

/** The same word, finished.
 *
 *  What an interruption lands on. Freezing mid-word would leave "Devo" sitting in the
 *  box, which reads as a typo rather than as an animation that stopped — and it stops at
 *  the exact moment the visitor's attention is on the field. */
export function completeWord(state: TypewriterState, roles: string[]): TypewriterState {
  return { role: state.role, chars: (roles[state.role] ?? '').length, deleting: false };
}
