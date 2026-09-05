// Runtime feature flags, read from `$env/dynamic/public` so flipping one is an env
// change plus a restart — never a rebuild. The predicates take the env map as an
// argument rather than importing it, which keeps this module free of SvelteKit
// runtime imports and unit-testable; callers pass `env` from `$env/dynamic/public`.

// Only the ON spellings are listed. Everything else — unset, empty, "0", "false", a
// typo — is off, so there is no third state to reason about and no way for an
// unparseable value to reveal a feature.
const TRUTHY = new Set(['1', 'true', 'on', 'yes']);

// The match sort had a flag here (PUBLIC_MATCH_SORT). It existed to hide the control
// until a full index rebuild had written the skill vectors it ranks against — before
// that the ordering returned almost nothing and read as broken. That rebuild has long
// since landed, so the flag was answering a question nobody asks any more, and it was
// also hiding the control from every visitor without a profile — the people who most
// need to be told the ordering exists. The feed explains what it needs instead; see
// facetModel's matchSortNeedsSkills.

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
