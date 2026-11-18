// The header search's pacing and section caps.

// How long the draft must sit still before the suggestions are refetched.
//
// A settled draft costs THREE requests (completions, postings, companies), so the
// window is not sized against one cheap pass — it is sized against the gap between
// keystrokes. At 120ms it sat inside that gap: a fast typist's 18 characters fired
// six rounds, eighteen requests, for one query they had not finished writing. 300ms
// is past the gap and is the window the filter store already debounces its reload by
// (see urlSynced.svelte.ts) — one answer to "how long is a pause" across the app.
export const SUGGEST_DEBOUNCE_MS = 300;

// Below this, the box asks nothing. A single character matches a large fraction of
// the catalogue, so the round trip buys a list nobody can act on — and it is the one
// round that fires with certainty, since every query passes through its first letter.
export const SUGGEST_MIN_CHARS = 2;

// Section caps. The dropdown is a shortcut, not a results page: past a handful each
// section stops being scannable and the whole thing stops fitting on a phone.
export const jobsLimit = 5;
export const completionsLimit = 5;
export const companiesLimit = 3;
// Asked for, before the relevance filter takes its cut.
export const companiesFetch = 12;

/**
 * What the dropdown's first section offers: a box with nothing to complete offers the
 * catalogue's shape (`starters`); a typed one offers what matches (`completions`).
 *
 * The first case is the whole point of opening on focus, and it is answered LOCALLY: the
 * curated group order lives in the filter modal's own grouping, checked there against the
 * category vocabulary at compile time, so asking a server for it would be a second copy of
 * that order. The typed case is the endpoint's — it completes a phrase against the
 * catalogue's real vocabulary, which no dictionary shipped to the browser can do.
 *
 * The threshold is SUGGEST_MIN_CHARS, the same one the fetch gates on, and deliberately not
 * a separate `=== ''`. Two thresholds opened a state nobody designed: at exactly one
 * character the box was past "empty" but short of "asked", so it showed neither the
 * starters nor completions it had not fetched — a ten-row panel collapsing to the single
 * free-text row, then refilling on the next keystroke. One predicate answers "is there a
 * query yet", so there is no gap for a state to fall into.
 */
export function panelSuggestions<T>(settledText: string, starters: T[], completions: T[]): T[] {
  return settledText.trim().length < SUGGEST_MIN_CHARS ? starters : completions;
}
