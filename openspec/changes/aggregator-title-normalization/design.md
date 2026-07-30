## Context

`SuppressAggregatorDuplicatesForCompany` matches an aggregator posting to its ATS twin on two
title keys, UNION ALL-ed as two separate single-equality hash joins. The query carries an explicit
warning: an `OR` of both equalities in one `ON` defeats the hash join and goes quadratic on a large
company. Any change must preserve that shape.

The decorated key (`ntitle2`) already decodes HTML entities and strips a trailing ` - `/` — `/` | `
clause. It does not strip a trailing `: …` clause.

## Goals / Non-Goals

**Goals:**
- Treat a trailing colon clause as decoration, so an aggregator that appends technologies to an
  otherwise identical title finds its ATS twin.
- Keep the two-hash-join shape; no new join, no new scan.

**Non-Goals:**
- Stripping parentheticals or comma clauses — measured, and both merge distinct roles.
- Any similarity/fuzzy matching. A normalization rule cannot bridge titles that differ by more than
  a trailing clause.

## Decisions

### D1 — Extend `ntitle2` in place rather than adding a third key

The colon clause is the same *kind* of thing the expression already strips: a trailing decorative
clause. Folding it into the existing `regexp_replace` chain keeps the query at two match paths.
A third key would mean a third hash join and a third UNION ALL branch for a 3% gain.

Order matters inside the chain: strip the colon clause **before** the dash clause, so
`Engineer: Go, K8s - Remote` reduces to `Engineer` rather than stopping at `Engineer: Go, K8s`.

### D2 — Reject the parenthetical and the comma, and record why in the spec

Measured on prod: adding the parenthetical raises matches from 536 to 575, and the 39 extra pairs
are wrong — one company's `Senior Software Engineer, Backend (Traffic)` matching its `(Payments)`,
`(Identity)`, `(Infrastructure)`, `(ML Feature)` and `(Lake Analytics)` roles. The comma fails the
same way (`, Backend` vs `, Fullstack`). Both are recorded as prohibitions in the spec so a later
reader does not re-derive the idea and re-introduce the bug.

### D3 — Do NOT reverse the subset arm

The query already has a third match path: `a.ntitle <@ t.ntitle`, i.e. the aggregator *dropped*
words the ATS keeps, guarded so the added words include a non-seniority one. Our case is the
mirror image — the aggregator *adds* words — so the obvious idea is to add the reverse containment
`t.ntitle <@ a.ntitle`.

Rejected, for the same reason as the parenthetical. Reversed, `Senior Software Engineer, Backend
(Traffic)` contains `Senior Software Engineer`, so an aggregator posting for one team would be
suppressed by an unrelated ATS posting with a shorter title — and with several such teams the
`MIN(ats_id)` would pick an arbitrary one. The existing direction is safe precisely because dropping
words loses information while adding words asserts it; only the latter can be checked against the
words themselves.

### D4 — Verify by counting, not by reasoning

The gain (+16) and the rejection (39 bad pairs) both came from running the candidate expressions
against prod and reading the pairs. Any future decoration rule should be justified the same way:
the corpus decides which punctuation is decorative, not intuition.

## Risks / Trade-offs

- **A colon clause that carries meaning** — e.g. `Engineer: Level II` — would merge with a plain
  `Engineer`. Not observed in the sampled pairs, and the seniority-grade guard on the role pass does
  not apply here. Accepted: the pass is reversible per reindex, and a wrong merge shows up as a
  missing card rather than a wrong one (the ATS twin stays canonical and correct).
- **Small gain.** +16 rows of 6371. Worth it only because the change is one expression and carries
  the measured prohibitions into the spec, which is the durable part.

## Open Questions

None. The rule, its gain and its two rejected variants were all measured before implementation.
