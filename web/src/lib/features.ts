// Runtime feature flags, read from `$env/dynamic/public` so flipping one is an env
// change plus a restart — never a rebuild. The predicates take the env map as an
// argument rather than importing it, which keeps this module free of SvelteKit
// runtime imports and unit-testable; callers pass `env` from `$env/dynamic/public`.

// Only the ON spellings are listed. Everything else — unset, empty, "0", "false", a
// typo — is off, so there is no third state to reason about and no way for an
// unparseable value to reveal a feature.
const TRUTHY = new Set(['1', 'true', 'on', 'yes']);

/** Whether the profile-match sort control is revealed in the jobs feed.
 *
 *  This ships DARK on purpose. The backend accepts `?sort=match` from the moment the
 *  binary rolls out, but it ranks against skill vectors that only exist once a full
 *  index rebuild has written them — before that the sort returns the handful of
 *  postings re-indexed since, which reads as a broken feed rather than a new feature.
 *  So the control stays hidden until someone confirms the rebuild landed, and the URL
 *  param stays usable throughout for exactly that check.
 *
 *  Default OFF: an unset or unrecognized value hides the control. A flag that reveals
 *  a feature when it cannot parse its own value is how one ships by accident. */
export function matchSortEnabled(env: Record<string, string | undefined>): boolean {
  return TRUTHY.has((env.PUBLIC_MATCH_SORT ?? '').trim().toLowerCase());
}

/** Whether the "open within N days" bound is revealed — the filter over how long a
 *  posting has been in the catalogue, as distinct from the date its source states.
 *
 *  Dark for the same shape of reason as the match sort, one step further along. The API
 *  honours `open_within_days` the moment the binary rolls out, but it filters on
 *  `created_ts`, and declaring a filterable attribute does not retro-fill the documents
 *  already in the index: the incremental drain only pushes documents whose content_hash
 *  moved, and this field is not in that hash. Until a full rebuild lands the bound
 *  matches almost nothing — a thin feed rather than an error, so nothing alerts and the
 *  control would simply look broken.
 *
 *  Default OFF, and the URL param is honoured whether or not this is set — which is how
 *  the bound gets verified against production before anyone can click it. */
export function openWithinEnabled(env: Record<string, string | undefined>): boolean {
  return TRUTHY.has((env.PUBLIC_OPEN_WITHIN ?? '').trim().toLowerCase());
}
