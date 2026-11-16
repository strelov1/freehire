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
