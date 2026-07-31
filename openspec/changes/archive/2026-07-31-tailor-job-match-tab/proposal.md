## Why

The tailoring workspace has a Verdict tab that stacks three measurements taken in three different
coordinate systems and calls them one thing: the ATS delta scores the *rendered tailored PDF against
the base CV*, the autopilot report describes *one agent run*, and the fit analysis scores the
*candidate's base profile* — the experience bank, not the document on screen. Editing the CV moves
the first, leaves the second stale, and cannot move the third at all.

So the one question the workspace exists to answer — "is the document I am editing right now a
better match for this vacancy than it was a minute ago?" — has no answer on the page. The candidate
tailors blind and is shown a fit score that was frozen before they typed a character.

The inputs to answer it are already in the repo and already deterministic: the vacancy's canonical
skills, the vacancy's title, `skilltag` over the rendered PDF's text layer, `classify` for seniority,
and the cached requirement ledger. Nothing here needs an LLM call, which means it can be recomputed
on every save and costs no AI credits.

## What Changes

- **New deterministic job-anchored score** over the tailored CV's rendered text layer, scored
  against the vacancy alone (no base-CV comparison). Four weighted categories: Job Title Match,
  Keyword Match, Seniority Fit, Requirements Coverage. Reported with each category's weight, so the
  UI can tell the candidate which row is worth their attention.
- **Requirements Coverage is re-derived, never re-asserted.** The requirement *texts* come from the
  cached fit analysis (they describe the vacancy and do not depend on any CV); their covered/missing
  status is recomputed against the current document. A requirement that carries no machine-checkable
  skill is reported as unverifiable and falls back to the cached LLM status, labelled as such — it
  is never silently reported as missing.
- **The workspace's right panel splits its tabs along the axis each one measures**: a new **Job
  Match** tab (this document vs this vacancy, live), a **Score** tab (the existing ATS-readability
  delta and the autopilot run log, unchanged in substance), and the existing Job description and
  Templates tabs. The frozen LLM verdict moves under Job Match, explicitly labelled as a snapshot of
  the base profile with its existing Recompute control.
- **Category rows expand.** The panel renders each category as a disclosure carrying the line items
  that produced its score — `atscheck` has been computing `LineItem`s with per-item points, status
  and fix text all along, and the current `AtsDelta.svelte` throws every one of them away.
- **The workspace links to the vacancy.** There is currently no route from `/tailor/<slug>` to
  `/jobs/<slug>`; the Job description tab shows the logo, title and company as dead text. A View Job
  link lands in the panel header.
- The Job Match score is recomputed when the workspace opens, after an agent turn, and after an edit
  is persisted — the ATS delta's two-render cost keeps it on its existing open/after-run cadence.

## Capabilities

### New Capabilities
- `tailor-job-match`: deterministic, job-anchored scoring of a tailored CV against the vacancy it is
  bound to — the four categories, their weights and line items, the requirement re-derivation and
  its unverifiable fallback, the endpoint's ownership and degradation rules, and the workspace tab
  that surfaces it live.

### Modified Capabilities
- `tailor-workspace`: the right-hand context panel's tab set changes from three tabs to four, split
  by what each measures; the workspace gains a link to the vacancy's own page.
- `tailor-ats-delta`: the delta is no longer surfaced under a tab named Verdict alongside the fit
  analysis — it moves to a Score tab that carries readability alone, and its per-category rows
  become expandable to the line items behind them.

## Impact

- **New**: `internal/jobmatchscore` (deterministic scorer, pure and I/O-free), `GET
  /me/cvs/:id/job-match`, `web/src/lib/tailor/JobMatch.svelte` + its unit-tested view model.
- **Modified**: `internal/handler/cv_ats_delta.go` (the rendered-text scoring helper is shared, so it
  moves to a home both endpoints can reach), `web/src/lib/tailor/ArtifactPanel.svelte` (tab set),
  `web/src/lib/tailor/AtsDelta.svelte` (line-item disclosures),
  `web/src/routes/tailor/[slug]/+page.svelte` (refresh cadence, mobile tab bar).
- **Additive on `internal/atscheck`**: `CategoryChange` gains the tailored side's line items so the
  Score tab's rows can expand. The category set, the weights and `Report` itself are untouched — the
  standalone CV ATS page reads exactly the report it reads today. `internal/matchanalysis` gains
  nothing and loses nothing; no new LLM call is introduced anywhere in this change.
- **Contract codegen**: the new wire shape is generated to TS via `cmd/gen-contracts`.
