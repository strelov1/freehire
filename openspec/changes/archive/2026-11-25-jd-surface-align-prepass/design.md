## Context

See proposal.md for motivation. Today `skilltag.Parse` collapses aliases to
canonicals for matching, but throws away which surface form appeared in the text.
`skilladjacency` holds peer substitutes; `skillbundle` holds market combinations.
Neither exposes "JD wrote X, CV wrote Y, same canonical — rewrite Y to X." Tailor
and autopilot ask the LLM to "use the vacancy's language," which rediscovers
dictionary facts at token cost.

`internal/cv` writes `cvs.data` on **create** only. Every later mutation goes
through `cvedit.Editor`. Autopilot undo reverts that run's edit batches — there
is no pre-run document snapshot.

## Goals / Non-Goals

**Goals:**
- Invert alias→canonical enough to recover preferred JD surfaces and apply them
  to a `cv.Document` deterministically, mirroring the JD (expand or shrink).
- Run that rewrite before any tailor/autopilot LLM turn.
- Keep prose replacement safe: no ambiguous English-word aliases in bullets.
- Keep Phase 1 strictly to same-canonical alias swaps; leave a clear seam for
  family/dedup phases.

**Non-Goals (design-level; full list in proposal):**
- Family relations, facet collapse, adjacency merges, LLM synonym discovery,
  provider-gated Taleo modes, fit-analysis semantic changes, aligning the
  experience bank.

## Decisions

### D1 — Preferred surface comes from the JD text, not a default display name

Scan the vacancy description with the same alias tables `Parse` uses; for each
canonical found, record the surface form(s) that actually matched, preserving
JD casing. When multiple surfaces for one canonical appear, prefer the longest
match (usually the expanded phrase over the acronym). When the JD uses only the
short form, the preferred surface **is** the short form (shrink).

*Rejected:* a fixed "pretty name" per canonical — that would ignore the JD's
own spelling, which is what a literal ATS screen looks for.
*Rejected:* always expand acronyms — Taleo matches what the posting wrote.

### D2 — Two replace tiers

| Field | What may be replaced |
|---|---|
| Skills-group items, experience/project `stack` | Any alias of the preferred canonical (`Canonicalize` — caller assertion) |
| Summary and bullets | Only unambiguous aliases: multi-word phrases and strong acronyms (`IaC`, `k8s`, `CI/CD`). Not `ambiguousWords` (`go`, `react`, `c`, …) and not 1–2 letter tokens |

Chips are assertions. Prose is English. The corroboration rule exists because
`go`/`react` in a sentence are usually verbs.

### D3 — Same-canonical chip collapse after rewrite

If the skills line listed both `IaC` and `Infrastructure as Code`, both become
the preferred surface; drop the duplicate. This is alias-identity, not family
dedup (`pgvector` vs `vector-databases` stays Phase 3).

### D4 — Write paths respect `cvedit`

- **First mint:** align the copied base document, then `CreateTailored`. No
  revision; the stored copy never held the unaligned wording.
- **Autopilot start and reset-from-résumé:** `cvedit.Editor.Commit` (revision).
  Autopilot undo then includes or excludes that batch according to whether it
  shares the run's batch id — prefer a **distinct** align batch so undoing the
  *run* does not revert wording that is still JD-correct. The candidate can
  revert the align revision separately if they want the acronym back.
- **Repeated bootstrap** of an existing copy: do not re-align (would fight
  interactive edits).

*Rejected:* silent `Store` overwrite on autopilot — `update` is unexported;
that path does not exist.
*Rejected:* passing only a hint list to the LLM — still spends tokens.

### D5 — Brief both presets

`tailorPrompt` and the autopilot brief both state that skill surface forms are
already aligned to the vacancy; do not rename skills for wording.

### D6 — Invert API stays beside Parse, does not change Parse

`Parse` still returns canonicals only (ingest/facets unchanged). New helpers
recover surfaces (`PreferredFromText`, alias lookup). Phase 2–4 consume the same
API.

### D7 — Phased dictionary growth (future; no code in this change)

```
Phase 1 (this change)     alias surfaces     IaC ↔ infrastructure as code
Phase 2                   skill families     vector-databases ⊃ {pgvector,…}
Phase 3                   family ensure+dedup  plant JD term; dedupe chips
Phase 4                   literal diagnostic   report remaining text misses
```

ATS delta already scores the rendered PDF: mint-time align will move keyword
strength vs base before any LLM edit. Phase 4 is then a `from`→`to` receipt,
not a new scorer.

Phase 2+ keep canonicals distinct in facets. Family is a new relation, not an
extension of `skilladjacency` peers. Do not align the experience bank — only
the throwaway tailored copy.

### D8 — Out of scope (pinned)

| Item | Why excluded |
|---|---|
| Alias collapse changes for ingest | Facets and matching stay as today |
| Collapse products into one facet | Filters must stay precise |
| Merge families into `skilladjacency` | Peers ≠ hyponyms |
| LLM-invented synonym/family links | Dict-only invariant |
| Inventing skills / tone paraphrase / stuffing gaps | Evidence gate |
| `source=taleo` special mode | Literal align helps every ATS |
| Fit-analysis / evidence-gate changes | Orthogonal |
| Reindex / Meili schema | Document-only rewrite |
| Rewriting banked atoms | Bank stays the candidate's words |

## Risks / Trade-offs

- **[Risk] Ambiguous prose replace** (`go` → `Golang` in "go-to person") →
  **Mitigation:** D2; thorough tests on `ambiguousWords` and short tokens in
  bullets, and on safe acronym/phrase hits in the same bullets.
- **[Risk] Long expansions bloat the skills line** → **Mitigation:** correct
  for literal ATS; Phase 3 trims family redundancy, not aliases.
- **[Risk] User reverts to acronym; next autopilot re-expands** →
  **Mitigation:** intended for an unattended run; interactive edits after
  bootstrap stay until reset or autopilot.
- **[Risk] Align commit vs autopilot-undo** → **Mitigation:** distinct batch
  (D4); tests pin that undoing the run leaves JD wording in place.
- **[Risk] Phase 2 scope creep** → **Mitigation:** Phase 1 tests only
  same-canonical pairs; family fixtures must not pass Phase 1.

## Migration Plan

Ship behind normal deploy. No DB migration. Existing tailored CVs align on next
autopilot start or reset; new bootstraps align at mint. Rollback: remove call
sites; copies already rewritten remain (harmless expansions or shrinks).

## Open Questions

None that block implementation. Workspace UI for the receipt can stay log-only
in Phase 1.
