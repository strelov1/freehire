## Why

The experience bank is additive by design (`ClaimKey` exact match only). That
protects against silently losing distinct claims, but near-paraphrases — chat vs
CV upload, or two assistant readings of the same project — land as sibling atoms.
The Experience tab can edit and delete; it cannot merge. The `profile` interviewer
nudges for numbers and situation, but `get_profile`'s experience summary only
counts achievements per role — it does not flag near-duplicates or thin
(metric-/context-poor) atoms.

When the bank holds two half-complete siblings instead of one rich atom,
`Retrieve` / tailor evidence and `cv.Seed` either pick one incomplete bullet or
present redundant near-duplicates. When a single atom is thin (claim only, no
metrics/context), tailor has nothing quantified to cite.

The product goal is richer, less duplicate-prone evidence. The preferred lever is
**contextualize at write time** (claim + situation when banking), which the owner
may opt into or out of via chat. Merge and post-hoc enrich remain recovery paths
for banks that already have thin or near-duplicate atoms. Neither merge alone nor
thin-flags alone is enough.

Approved design: [design.md](./design.md).

## What Changes

- Owner-scoped **merge** of two experience atoms (HTTP + assistant tool, one
  `Store.MergeAtoms` path): system chooses the richer keep; owner confirms; union
  metrics/skills; richer context; publishable provenance; delete the loser.
- **Soft-duplicate** clusters (token Jaccard on `meaningfulTokens`, threshold
  0.40) and two derived richness signals (`needs_context`, `needs_metrics`) on
  Experience list, `get_profile`, and `interview_context` — no `ClaimKey`
  broadening, no STAR columns, no embeddings.
- Experience UI **multi-select** action bar: Merge (valid pair) and Tailor
  (`/my/assistant?preset=profile&atoms=<ids>`). Same edit mode as claim also
  edits context + metrics.
- Opt-in per-user **context gate** (`users.experience_require_context`, default
  off): assistant asks; after agreement, interactive creates refuse empty
  context. Import, update, and merge stay ungated. No visual toggle for MVP.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `experience-bank`: owner merge, soft-dup / richness surfacing, enrich via UI
  and `profile` chat, and a chat-opt-in write-time context gate on interactive
  atom creates.

## Impact

- `internal/experience` (`MergeAtoms`, similarity + richness helpers), one
  migration for `users.experience_require_context`, handlers
  (`me_experience`, assistant experience/profile/interview tools),
  `internal/assistant/prompt.go`, SPA `ExperienceBankView.svelte` + `api.ts`.
- ATS continues to ignore the bank. Exact-key dedup and import additive
  behaviour unchanged.
