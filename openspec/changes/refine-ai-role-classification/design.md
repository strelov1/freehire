## Context

`internal/roletag` and `internal/skilltag` are both curated, dict-only
dictionaries (never guess, never persist out-of-vocabulary). `roletag`
resolves a job's `roles` facet from **title only** (plus already-resolved
seniority/category); `skilltag` resolves the `skills` facet from job text via
a word/phrase alias dictionary, with a gate (`ambiguousWords`) that requires
corroboration for tokens that collide with ordinary English.

Prod validation (`https://freehire.me/api/v1/jobs/search`, read-only, 2026-08-09):

| Check | Result |
|---|---|
| `category=ai_engineering` | 7,766 jobs |
| `category=ml_ai` | 7,196 jobs |
| `q=MCP&category=ai_engineering` (free-text mentions) | 159 jobs |
| `skills=mcp` (currently tagged) | 1 job |
| `skills=rag&category=ai_engineering` (currently tagged) | 1,181 / 7,766 = 15.2% |
| Field guide baseline for RAG mentions in "AI Engineer" titled jobs | 35.9% |
| `role=forward_deployed_engineer` (current single-phrase alias) | 1,570 jobs |

This confirms both dictionary gaps are real (MCP near-total miss; RAG
under-tagged by more than half) and that the existing FDE alias already
produces a meaningful base to extend rather than a broken signal to replace.

## Goals / Non-Goals

**Goals:**
- Derive a new `ai_archetype` facet from a job's already-resolved `skills` and
  `category` — no new free-text parsing, no LLM call, no migration.
- Close the two confirmed `skilltag` dictionary gaps (MCP, RAG) without
  reopening the RAG/"RAG status" collision on the rest of the catalogue.
- Widen `forward_deployed_engineer`'s alias coverage and add the two
  field-guide-documented synonym titles as their own named roles.

**Non-Goals:**
- Not replacing or reweighting the existing `roles` facet — `ai_archetype` is
  additive, scoped to `category ∈ {ai_engineering, ml_ai}`.
- Not reproducing the field guide's k-means clustering exactly — the six
  archetypes are approximated with a small ordered rule set over the existing
  skill dictionary, not a model. A rule-based approximation is the only option
  consistent with the project's dict-only doctrine (`internal/classify`,
  `internal/roletag`, `internal/skilltag` all forbid a model call for a
  production facet).
- Not touching the declining trainer-stack skills (`PyTorch`, `Fine-Tuning`,
  `RLHF`) the field guide flags — they're already present or out of this
  change's confirmed-gap scope.

## Decisions

### 1. `ai_archetype` lives in a new package, not inside `roletag` or `skilltag`

`roletag` and `skilltag` are independent dictionaries with no cross-dependency
today (`roletag` never imports `skilltag`, and vice versa). `ai_archetype`'s
whole job is to consume `skilltag`'s *output* (the resolved skill slugs) plus
`category`, so it is a distinct responsibility layered on top of both — the
same relationship `roletag` has to `classify`. New package: `internal/aiarchetype`.

```go
// Derive returns the single skill-signature archetype a job's skills and
// category resolve to, or "" if category is out of scope or no rule
// matches. Dict-only: never guesses, never returns more than one archetype
// (the archetypes are the mutually-exclusive clusters the field guide's
// k-means analysis found, not additive facets).
func Derive(skills []string, category string) string
```

Alternative considered: extend `roletag.Derive`'s signature to also take
`skills` and append archetype slugs to its existing multi-value `[]string`
return. Rejected — `roletag`'s existing return value is additive (seniority +
category + named role can all coexist on one job); an archetype is a single
mutually-exclusive cluster assignment, a different shape that would need its
own sub-slice convention inside the same return value regardless.

### 2. Wiring follows the `roles` facet pattern exactly: computed at index time, never persisted

Confirmed via investigation: `roletag`'s `roles` are not stored in Postgres at
all. They're recomputed on every `internal/search/document.go` `FromJob()`
call from already-persisted `jobs.seniority`/`category`/`title`, added to
`JobDocument` (not `jobview.Job` — it backs a facet, not the public wire
shape), and reach historic jobs the next time `cmd/reindex` runs (unconditional
full rebuild). `ai_archetype` follows the same shape: a field on
`JobDocument`, set via `aiarchetype.Derive(doc.Skills, doc.Category)` inside
`FromJob`, plus:
- `internal/search/client.go` `facetSettings()`: add `"ai_archetype"` to
  `FilterableAttributes`.
- `internal/search/query_filter.go` `StringFacets`: add
  `"ai_archetype": "ai_archetype"`.
- `internal/search/settings_test.go`: a sibling to `TestRolesIsFilterable`.

No migration, no `cmd/backfill-derive` change — nothing new is written to
Postgres. Per `internal/search/AGENTS.md`, adding a filterable attribute opens
a brief hard-500 window if the app starts filtering on it before the settings
push/reindex has run — the task order pushes settings first.

### 3. Archetype rules: an ordered, first-match-wins rule set over confirmed `skilltag` canonicals

Six archetypes, checked in this order (most AI-distinctive signature first,
most generic/overlapping last — a job matching an earlier rule is claimed
before a later, broader rule sees it):

| Order | Archetype | Rule (all canonicals already exist in `skilltag`) |
|---|---|---|
| 1 | `rag_app_builder` | `rag` AND (`langchain` OR `langgraph` OR `llamaindex`) AND `vector-databases` |
| 2 | `agent_builder` | `agentic-ai` AND `prompt-engineering` AND `rag` |
| 3 | `cloud_ml_platform_engineer` | `mlops` AND `kubernetes` AND (`pytorch` OR `tensorflow`) |
| 4 | `ml_trainer_researcher` | (`pytorch` OR `tensorflow`) AND NOT `rag` AND NOT `agentic-ai` |
| 5 | `fullstack_ai_engineer` | (`react` OR `nodejs`) AND (`llm` OR `openai` OR `anthropic`) |
| 6 | `devops_infra_engineer` | `terraform` AND `kubernetes` AND `docker` AND `ci-cd` |

Rule 1 before rule 2: `rag_app_builder`'s signature (RAG + a named
orchestration framework + vector DB) is strictly more specific than
`agent_builder`'s (agents + prompting + RAG with no framework/DB requirement),
so every job rule 1 would claim is also a superset match for rule 2 — checking
rag_app_builder first prevents it from losing those jobs to the broader rule.
Rule 4 explicitly excludes `rag`/`agentic-ai` so a job that mentions both
training basics and RAG (64% of ai-first roles need "some" ML per the field
guide) lands in `rag_app_builder`/`agent_builder`, not `ml_trainer_researcher`
— consistent with the field guide's own framing of ML knowledge as a baseline
skill, not the archetype-defining one, for AI-first roles.

This is a deliberate simplification of a statistical clustering into a
deterministic ordered rule set — see Risks.

### 4. `mcp` added as a plain (non-gated) word alias

`"Microsoft Certified Professional"` is the only real-world collision for the
bare token, and it's a legacy-2000s abbreviation effectively absent from
current job postings — unlike the live English-word collisions
(`react`/`swift`/`spring`/…) `ambiguousWords` exists for. Added as a strong,
ungated alias: `"mcp": "mcp"`, consistent with `langgraph`/`crewai`/other
modern single-token framework names already in the same block.

### 5. `RAG` acronym: category-scoped strong match, not global

`resumeAcronyms["RAG"]` already proves the acronym is desired but gated
specifically because job postings (not résumés) carry "RAG status" reporting
prose. Rather than a blanket move to `sharedAcronyms` (reopening that
collision catalogue-wide) or leaving the gap, `skilltag.Parse` gains a new
functional option, `WithAcronymCategory(category string)`, that the job
ingest path (which already resolves `category` via `classify.Parse` before
calling `skilltag.Parse`) opts into. A new `categoryScopedAcronyms` table
(mirroring `sharedAcronyms`'s shape) resolves `RAG → rag` only when the
caller-supplied category is in its own allow-list
(`ai_engineering`, `ml_ai`) — colocated with the acronym entry, same
self-documenting pattern the file already uses for `ambiguousWords`. Every
existing `Parse` call site is unaffected (the option defaults to unset); only
the job ingest path (`internal/jobderive`) is updated to pass it.

Alternative considered: gate on `ambiguousWords`-style corroboration instead
(only tag bare "RAG" when another strong AI token is also present). Rejected
as strictly worse here — a job in `ai_engineering`/`ml_ai` already carries that
corroboration by construction (it required an AI-flavored title to classify
into the category in the first place via `classify`), so the category check
already does the corroboration's job with one lookup instead of a text re-scan.

## Risks / Trade-offs

- **[Risk]** The six ordered rules are a hand-authored approximation of a
  k-means clustering fit on a different (smaller, differently-sourced)
  dataset — they will not reproduce the field guide's exact archetype
  percentages on freehire's catalogue. → **Mitigation**: dict-only facets in
  this codebase are already approximations by design (`classify`, `roletag`
  make the same trade); ship the rule set, and treat a follow-up recalibration
  against freehire's own `ai_archetype` facet distribution (once populated) as
  a natural next iteration, not a blocker for this change.
- **[Risk]** A job matching no rule (most jobs even within
  `ai_engineering`/`ml_ai`) gets no archetype — expected under the dict-only
  "never guess" doctrine, but means the facet's coverage will be partial from
  day one. → **Mitigation**: this is the same trade-off `roletag`'s named
  roles already make; no action needed, just documented so it isn't mistaken
  for a bug.
- **[Risk]** `WithAcronymCategory` is a new kind of option for `skilltag`
  (category-aware, where every existing option is category-agnostic) —
  a second category-scoped acronym added carelessly later could turn this
  into a maze of special cases. → **Mitigation**: the design keeps exactly one
  table (`categoryScopedAcronyms`) for this class of exception, mirroring how
  `ambiguousWords`/`nonCorroboratingPhrases` are each a single table already;
  a future addition extends that table, it doesn't add a new mechanism.

## Migration Plan

1. `internal/skilltag` dictionary + `WithAcronymCategory` option (independent,
   no dependents yet).
2. `internal/roletag` alias/named-role additions (independent).
3. `internal/aiarchetype` new package, consuming `skilltag` canonicals only
   (unit-testable in isolation).
4. `internal/jobderive` — pass category into `skilltag.Parse` via the new
   option.
5. `internal/search` wiring — `document.go`, `client.go` (push settings first),
   `query_filter.go`.
6. `cmd/reindex` — full rebuild so `ai_archetype` reaches every historic job
   in `ai_engineering`/`ml_ai` category. Standard operational step for any new
   filterable attribute (`internal/search/AGENTS.md`), not a new mechanism.

No rollback complexity: every change is additive (new dictionary entries, new
facet); removing the facet later is a revert with no data cleanup, since
nothing is persisted to Postgres.

## Open Questions

None outstanding — scope, taxonomy shape, RAG-gap fix, and FDE-synonym
handling were each resolved during brainstorming (see proposal.md's
Capabilities section for the resulting scope).
