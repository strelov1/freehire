## Why

`WorkModeFromDescription` (`internal/dict/location/workmode.go`) treats "work from
anywhere" as an unconditional remote-work statement. In production it is also a
common idiom for a bounded travel/PTO perk ("the freedom to work from anywhere in
the world for up to a month", "work from anywhere for up to 12 weeks a year", "a
Work from Anywhere policy allowing up to 20 remote days"). Because this detector is
the lowest-priority source in the work-mode precedence chain
(structured ATS → location marker → description phrase), the false match only
lands when both higher-priority sources are silent — but when it lands, it fills
`work_mode=remote` on a posting that is actually hybrid or onsite, with nothing
downstream able to correct it (`RemoteContradicted` only overrules an explicit
denial of remote, never a phrase-fill outcome). Reported as freehire#2696, with a
live example (a San Francisco/New York hybrid posting whose only "remote" signal is
a PTO line offering a month of location freedom).

Confirmed against production data (read-only Meilisearch queries over the live
`jobs` index, freehire#2696 investigation): of 143 open postings where "work from
anywhere" co-occurs with the qualifier "for up to", 61 currently resolve to
`work_mode=remote`; manual inspection of a sample found several genuine
misclassifications (a Berlin-office posting offering "work from anywhere for up to
12 weeks a year" resolving to `remote` on one source copy while an identical
posting from the same employer correctly resolves to `hybrid` via its own
structured signal; a UK posting with "in-office presence four days per week and a
Work from Anywhere policy allowing up to 20 remote days" resolving to `remote`). The
qualifying phrase appears on either side of the match in real postings, not only
after it.

## What Changes

- `WorkModeFromDescription` no longer treats "work from anywhere" / "work-from-anywhere"
  as a remote signal when a bounded-duration qualifier ("up to") appears near the
  match (either before or after, within a measured window) — the same "usually
  unambiguous but sometimes qualified" shape `RemoteContradicted`'s
  `denialQualifiers` already handles for the denial side of this file, applied here
  to the fill side.
- When the qualifier suppresses the match and no other phrase in
  `descriptionWorkModePhrases` matches, `WorkModeFromDescription` returns `""` (no
  guess) rather than `"remote"` — consistent with the dictionary's existing
  never-guess behavior elsewhere in this package.
- No change to the other remote phrases (`100% remote`, `fully remote`,
  `remote-first`, `remote role`, `remote position`, etc.) — measurement showed no
  comparable ambiguity for them (see design.md).
- No change to the hybrid phrase family, `RemoteContradicted`, or the
  `jobderive` precedence chain.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `deterministic-facets`: the `work_mode` facet's description-derived tier gains a
  qualifier guard on the "work from anywhere" phrase, narrowing when it may fill an
  empty `work_mode`.

## Impact

- **Code:** `internal/dict/location/workmode.go` (single function,
  `WorkModeFromDescription`), `internal/dict/location/workmode_test.go` (new
  table-driven cases using real production sentences).
- **No schema change, no migration, no new source files.**
- **Data:** existing jobs already carrying the wrong `work_mode=remote` need
  `cmd/backfill-derive` followed by a full `cmd/reindex` to reach Meilisearch — the
  standard propagation path for any dictionary change to this facet (see
  `internal/dict/location/AGENTS.md`). Not part of this code change; called out as a
  deploy follow-up in design.md.
