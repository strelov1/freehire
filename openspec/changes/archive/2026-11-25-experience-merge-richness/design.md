## Context

See proposal.md for why. Constraints that shape the approach:

- The bank is additive (`ClaimKey` exact match + unique index). Near-paraphrases are siblings on purpose; import must not start merging them.
- `Atom` is the public wire type (`internal/experience/experience.go`) and the assistant/HTTP payload. Enrichment flags are derived on read and must not become persistable fields the client can POST back.
- `get_profile` reports bank *shape*, not atom bodies — a tool result is replayed every later turn. Soft-dup / richness surfacing there has to stay id-lists and counts.
- `cv.Document` is a snapshot. A bullet already on a CV keeps its text if the cited atom is later deleted; only a later agent `cv_edit` that re-cites the deleted id fails the evidence gate. That is the existing contract, not a new problem unique to merge.
- `PUT`/`DELETE` on existing atoms are cookie-only; `POST` creates take a key. Merge deletes an atom, so it sits with correct/delete, not with create.
- `meaningfulTokens` + the retrieval stopword list already live in `internal/experience/retrieve.go` and reuse `ClaimKey` normalisation. Soft-dup must use that, not a second tokeniser.
- Latest applied-style migration prefix in tree is `0076_*`; a new column on `users` is an expansive read on the next binary (`experience_add` / `get_profile`).

## Goals / Non-Goals

**Goals:**

- One `Store.MergeAtoms` used by HTTP and `experience_merge`, deterministic, owner-scoped, transactional.
- Read-time soft-dup clusters (employment-scoped Jaccard) and `needs_context` / `needs_metrics` on the owner list, `get_profile` (shape only), and `interview_context` evidence.
- Experience UI: multi-select merge + tailor-into-`profile` with atom ids; edit context and metrics, not only claim.
- Chat-opt-in context gate on interactive creates only (`experience_add`, `POST /me/experience/atoms`).

**Non-Goals:**

- Broadening `ClaimKey`, stemming, embeddings, or auto-merge on import.
- LLM-composed claim text inside `MergeAtoms`. After a merge, chat may `experience_update` the kept claim if the owner wants a richer sentence.
- STAR columns, employment-level merge, or merging more than two atoms in one call.
- A visual settings toggle for the context gate.
- Rewriting existing CV `evidence_id`s when the loser is deleted.
- Changing `assistant-agent-runtime` as a separate capability — the new tool is an experience-bank surface registered like the other bank tools.

## Decisions

### One Store method, one SQL write

`Store.MergeAtoms(ctx, userID, a, b uuid.UUID) (Atom, error)` loads both atoms (missing/foreign → `ErrNotFound`), validates the pair, chooses keep vs loser, builds the unioned keep, then persists in **one statement**: `DELETE` the loser `RETURNING` into a CTE, `UPDATE` the keep only if the delete landed. Zero rows → `ErrNotFound`. No new `Repository` transaction helper; the CTE is the atomicity.

HTTP `POST /me/experience/atoms/merge` (cookie, body `{"ids":[id1,id2]}`) and tool `experience_merge` (`ids` array, every preset) both call this method. Handler maps sentinels through `experienceError`; add `ErrMergeCrossEmployment`, `ErrMergeSameAtom` (or one `ErrInvalidMerge` with a model-readable message listing why).

*Alternative considered:* `UpdateAtom` then `DeleteAtom` without a tx. A crash leaves a richer keep *and* the loser — a retry still works, but the bank is briefly more duplicated, which is the opposite of the feature. Rejected.

*Alternative considered:* model-supplied `keep_id`. Rejected — the owner confirms the *pair*, the system chooses the richer keep so UI and chat cannot disagree, and evidence_id stability is deterministic.

### Keep selection and field union (no claim rewrite)

Richer score, in order of weight: non-empty context (1) + `len(metrics)` + `len(skills)` + publishable provenance (1). Tie → older `created_at`, then smaller id (citation stability when richness is equal).

- **Claim / claim_key / employment_id / source_ref:** keep’s, unchanged.
- **Context:** the longer non-empty string. Do not concatenate — two paraphrases glued together are worse than one situation paragraph. Empty + non-empty → non-empty.
- **Metrics / skills:** union, order preserved (keep first), then `Sanitize` (canonical skills, caps).
- **Provenance:** if either `Publishable()`, kept provenance becomes the publishable one (prefer keep’s if both are; else the publishable sibling). If both `agent_inferred`, stay `agent_inferred`. Merge does **not** stamp `manual` — that is `UpdateAtom`’s confirmation path, and auto-confirming two unconfirmed readings would launder inference onto a CV.

Pair rules (all fail closed, persist nothing):

| Pair | Outcome |
|---|---|
| Same id twice / fewer or more than two | refuse |
| Different `employment_id` (including one set and one nil) | refuse |
| Both nil (unplaced) or same employment | allow |
| Either not owned | `ErrNotFound` |

Unplaced + unplaced is the screenshot case (assistant readings of one project, no role yet).

### Soft-dup is employment-scoped Jaccard, computed on read

`SoftDuplicateClusters(atoms []Atom) [][]uuid.UUID` and `Richness(a Atom) (needsContext, needsMetrics bool)` live next to `meaningfulTokens`.

- Tokenise **claim only** (not context/metrics — those are what we want to *union*, not what defines “same work”).
- Jaccard ≥ **0.40** on the meaningful-token sets. Threshold is a named constant with table-driven cases: the two faster-whisper plugin claims cluster (~0.44); “Cut latency 20s to 1s” vs “Cut p99 30s to 1s” do not (~0.33); stopword-only overlap does not (~0.25). 0.55 was too high for the motivating pair.
- Cluster with union-find, **within one employment bucket** (each `employment_id`, plus one bucket for `nil`). Cross-role “built a plugin” must not look mergeable — merge would refuse it anyway, and a false cluster in the UI is worse than a missed one.
- `needs_context` ↔ `strings.TrimSpace(context) == ""`.
- `needs_metrics` ↔ no metrics **and** claim has no digit (`\d`). A claim that already carries “40%” is not thin on numbers.

`Atom` stays clean. `ListExperience` projects:

```json
{
  "employments": [{ "...Employment", "atoms": [atomView, ...] }],
  "unplaced": [atomView, ...]
}
```

`atomView` is the atom plus `needs_context`, `needs_metrics`, and optional `cluster_id` (opaque, stable for this response only — e.g. first member’s id). Clients must not persist `cluster_id`.

`get_profile` / `experienceSummary` gains `soft_duplicate_clusters: [[id, id], ...]` (cap **8** clusters, each cap **6** ids), `needs_context_count`, `needs_metrics_count`, and `require_context` (the opt-in flag). Still **no claim/context text**. Extend `TestProfileToolReportsShapeNotContents`.

`interview_context` / `requirementEvidence` already returns id + claim + publishable; add the two bools there. No cluster lists — that payload is already requirement-capped.

*Alternative considered:* pg_trgm / embedding index. Rejected — banks are tens-to-hundreds of atoms; a linear pass matches retrieval and needs no migration.

*Alternative considered:* storing `cluster_id` or richness on the row. Rejected — they drift as soon as one atom is edited, and import would have to maintain them.

### Experience UI: multi-select + deeper edit + atom-scoped kickoff

`ExperienceBankView.svelte`:

- Checkbox (or toggle) per achievement; a slim action bar when `selected.length > 0`.
- **Merge** enabled iff `selected.length === 2` and they share a bucket (same employment or both unplaced). Confirm copy: one sentence that they will become a single achievement. Call `POST /me/experience/atoms/merge`, reload.
- **Tailor** always enabled for 1+ selection → `/my/assistant?preset=profile&atoms=<id,id>`.
- Edit mode: claim textarea stays; add context textarea + metrics (comma or one-per-line). Save still goes through existing `PUT` (stamps `manual` — that remains the confirmation path).

`entryFromQuery`: if `preset=profile` and `atoms` is a comma-separated list of UUIDs, keep `PROFILE_KICKOFF` but append a short, non-prompt-injection-safe line that *names the ids* (“Start with these achievements: `<id>`, `<id>` — they look related or thin. Ask whether to merge or what to add.”). Ids are server-minted UUIDs, not user prose. Empty/malformed `atoms` → ignore, same kickoff as today.

`profilePrompt`: add three work-list items alongside empty roles / uncovered skills / numberless claims — (1) `soft_duplicate_clusters` from `get_profile`, (2) `needs_context` / `needs_metrics` counts, (3) when the arrival ids are in the user message, search those first. Instruct: ask before `experience_merge`; after merge, optionally `experience_update` the kept claim if the owner supplies a richer sentence. Explain the context-gate trade-off once if `require_context` is false; call the setter only after they agree.

### Context gate is a user flag, default off, checked only on interactive create

Migration `0077_users_experience_require_context.sql`:

```sql
ALTER TABLE public.users
    ADD COLUMN experience_require_context boolean NOT NULL DEFAULT false;
```

No third state — off means ungated. Expansive: migrate **before** the binary that reads it on `experience_add` / `get_profile` / `POST /me/experience/atoms`.

sqlc: `GetUserExperienceRequireContext`, `SetUserExperienceRequireContext`. Not on `GetUserByID` — `/auth/me` should not grow this.

New sentinel `ErrContextRequired`. Check lives in the **handler/tool**, not `Store.AddAtom`: import and merge/update must not see it. `experienceHandlers.AddAtom` and `experienceAddTool` read the flag; if on and `strings.TrimSpace(context)==""`, refuse.

Write path for the flag: assistant tool `experience_set_require_context` `{enabled: bool}`, every preset (so a tailoring chat can opt in too), cookie-equivalent: the tool runs as the session owner. No HTTP toggle for MVP (spec: no visual control). Prompt rule: do not call it until they agree — same honesty model as “ask before `experience_add`”, not a transcript substring check (a preference is not a claim).

*Alternative considered:* gate inside `Store.AddAtom`. Rejected — import calls `AddAtom`/`InsertAtomIfNew` and must stay ungated; putting a user-row read in the bank store couples `experience` to `users`.

*Alternative considered:* visual switch on the Experience tab. Rejected for MVP per proposal; chat is where the trade-off can be explained.

### Evidence ids after merge

Kept id is stable. Loser id disappears. CVs that already cite the loser remain printable snapshots; a later agent edit citing that id fails “no banked achievement”. Acceptable. Do not scan `cvs` to rewrite citations — that would mutate documents the candidate may have already sent.

## Risks / Trade-offs

- [Jaccard 0.40 false-positives two distinct claims that share a stack phrase] → Employment-scoped clustering + merge still requires an explicit owner confirm (UI or chat). Tune the constant with the screenshot pair and a negative table in tests; do not raise it silently in production without a new change.
- [False-negatives leave near-paraphrases unflagged] → Chat + multi-select still merge any valid pair; flags are a hint, not a gate.
- [Dangling `evidence_id` on tailored CVs after merge] → Snapshot rule; mention in `internal/experience/AGENTS.md`. No CV rewrite.
- [Context gate on `experience_add` surprises a tailoring turn that used to bank a claim-only confirmation] → Default off; interviewer explains before enabling; `experience_update` can add context to an existing thin atom without the gate.
- [CTE merge vs unique `claim_key`] → Keep’s claim is unchanged, so the unique index cannot fire unless a bug rewrites claim. Test that merge does not change `claim_key`.
- [Multi-select on a long bank is clumsy] → Same page already lists every atom; no pagination today. Do not invent virtualisation here.
- [`atoms` query param as a pseudo-prompt] → Only UUIDs, validated client-side before kickoff text; ignore junk.

## Migration Plan

1. Apply `0077_users_experience_require_context.sql` on existing DBs **before** deploying the binary that reads the column (`cmd/migrate`). Fresh volumes pick it up via initdb filename order.
2. Deploy API + web together: old SPA ignores unknown list fields; new SPA talking to an old API would show no flags and a dead merge route — avoid mixed deploy for the Experience tab.
3. Rollback: revert binary first; column stays (`DEFAULT false`), unread. Merge CTE and new tool disappear with the binary; no data backfill to undo. Merged atoms stay merged (owner-initiated deletes are not rolled back).
4. No backfill of clusters or richness — derived on read.

## Open Questions

None that change specs or task shape. Threshold 0.40 and keep-score weights can be tuned from production false-positive examples in a follow-up without a schema change.
