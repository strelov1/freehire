## Why

`README.md` describes freehire as a catalogue and an ingest pipeline — every
section below "Why freehire?" is about getting postings *in*. Roughly a third of
`internal/` serves what happens *after* a posting is found (`cv`, `cvedit`,
`cvmatch`, `matchanalysis`, `experience`, `tracerlink`, `jobtracking`,
`appevent`, `inbox`, `mailingest`, `maillink`, `assistant`, `referral`, `ghost`,
`subscription`, `savedsearch`), and none of it is mentioned. A visitor
underestimates the project; a contributor sees only one seam to contribute to.

## What Changes

- **New `docs/features.md`** (~150 lines): a hybrid reference covering every
  production feature in five funnel sections — Find, Apply, Track, Ask, Build on
  it. Per feature: 2–3 sentences of what it does and why it is built that way,
  then one pointer line (`Live:` route · `Code:` packages · `Deep dive:`
  AGENTS.md).
- **`README.md`, four additive edits**: a new `## Beyond the catalogue` section
  after "Why freehire?" (five-row table keyed by the funnel, linking into
  `docs/features.md`); a `Features` entry in the nav line; the load-bearing
  product packages added to `## Layout`; the last "Why freehire?" bullet widened
  to name the workspace.
- **Accuracy rules bind the write-up**: every `AGENTS.md` target and every
  `Live:` route is verified against disk before commit; LLM-gated features are
  marked as needing `LLM_*`; the ghost-job signal is described as a signal, not
  a verdict; anything not reachable in production is dropped rather than hedged.

No code changes. The header, tagline, catalogue counts and source tables are
untouched.

## Capabilities

### New Capabilities

(none) — documentation-only change; no new requirement-level behavior.

### Modified Capabilities

(none) — no existing capability's requirements change. Per the repo's convention
for doc-only and refactor changes, this change carries no spec deltas and is
archived with `openspec archive <name> --skip-specs -y`.

## Impact

- `README.md` — four additive edits, grows from 390 to roughly 405 lines.
- `docs/features.md` — new file.
- No Go, SQL, or web source is touched; no migration, no worker, no endpoint.
- Risk is broken links: a README pointing at a non-existent `AGENTS.md` or route
  is worse than one omitting the feature. Verification is a real link check, not
  a read-through.
