## Context

The tailoring workspace already renders three measurements and owns none of the one that matters
while editing. `internal/atscheck` scores the rendered PDF's text layer into five categories, of
which exactly one — Keyword Strength, at 40 of 100 points — is anchored to the vacancy; the other
four describe the document in isolation. `internal/matchanalysis` scores the candidate's *base
profile* through a three-stage LLM chain and is served to the workspace from cache, frozen. The
`AtsDelta.svelte` panel receives `LineItem`s from neither: `atscheck.CategoryChange` carries only
`base`, `tailored` and `change`, so the fix text the scorer computed never reaches the screen.

Everything the job-anchored score needs is already in the repo and already dictionary-only:
`skilltag.Parse` with résumé acronyms over the rendered text, the vacancy's canonical `skills`,
`classify.Parse` / `classify.Categories` for titles and grades, `vocab.SeniorityValues` as an
ordered ladder, and `cachedAnalysisCtx` for the requirement texts.

Constraint from the project conventions that shapes most of the decisions below: **dictionaries are
dict-only — never guess, emit nothing for unknowns.** A deterministic matcher that reports "missing"
for a requirement it simply cannot read would be guessing.

## Goals / Non-Goals

**Goals:**
- One number in the workspace that moves when the candidate edits the document, computed against
  the vacancy and nothing else.
- Every category explains itself: weight visible, line items expandable, missing skills named.
- No LLM call, therefore no AI-credit metering question, therefore safe to recompute on autosave.
- The two scores in the workspace stop competing for the same heading.

**Non-Goals:**
- Re-running the fit analysis against the tailored document. That is a real feature and a different
  change: it needs `matchanalysis.Input` to accept a document instead of the experience bank, and it
  costs three LLM stages per edit. The snapshot label is the honest interim.
- Re-weighting or re-shaping `internal/atscheck`. Its five categories and their maxima stay as they
  are; the standalone CV ATS page must keep reading the same `Report`.
- Making the score authoritative. Like the ATS delta, it is informational and gates nothing.
- An "apply this fix" action on a line item. The seam is obvious (a missing keyword is a chat
  message away) and deliberately not built here.

## Decisions

### A new package, `internal/cvmatch`, rather than a second scorer inside `atscheck`

The two scorers share their input (rendered text) and their shape (weighted categories of line
items), which argues for one package — but they differ where it counts. `atscheck.ScoreCategory`
has no notion of *unavailable*: every one of its five categories is always computable from any
text. `cvmatch`'s categories are not — a vacancy with no canonical skills, a title the dictionary
cannot grade, a pair with no cached analysis — and the unavailable rule (below) is the core of the
design, not an edge case. Bolting `Available`/`Reason` onto `atscheck.ScoreCategory` would put a
field on the CV ATS page's wire shape that is always `true` there.

`internal/jobmatch` was also considered and rejected: it already owns the deterministic skills-only
bar rendered on job cards, and `jobmatch.JobMatch` meaning two different things in two contexts is
the kind of name collision that costs an afternoon later.

`cvmatch` therefore declares its own small `LineItem`/`Category` types. The duplication is four
fields. The seam for a shared `scorecard` vocabulary package is noted here and not taken — the two
types are not the same type today, and merging them before they are would be building the
abstraction first.

### The overall score is earned ÷ possible, so "unavailable" needs no special case

    overall = round(Σ earned(available categories) ÷ Σ weight(available categories) × 100)

This one formula covers every degraded state. Three categories available out of four gives a
denominator of 60 and a score still expressed out of 100. Nothing is scored as zero for being
unreadable, and no branch exists for "some categories are missing".

The same rule recurses one level down: inside Requirements Coverage, an unverifiable requirement
leaves that category's denominator too. This mirrors the must-have skill-bearing denominator
already used elsewhere in the codebase — a denominator that counts things nobody can check produces
a number that punishes the candidate for our dictionary's gaps.

Alternative considered: score unavailable categories as zero and show a caveat. Rejected — the
number would be wrong in exactly the direction that makes a candidate rewrite a CV that was fine.

### Requirement texts are borrowed; requirement statuses are recomputed

The cached `Analysis.RequirementMatch` is the only LLM-derived input, and only its `text` and
`priority` fields are used — both describe the posting and are independent of any CV. `status` and
`evidence` were determined against the base profile and are deliberately not trusted here.

Re-derivation asks a narrower question than "what skills are in this text": it asks **which of the
vacancy's own canonical skills this requirement names**.

The first draft of this design said `skilltag.Parse(requirement.text)`, and implementation showed
why that fails. `Parse` applies a **corroboration rule**: a weak alias — an English word that
doubles as a technology (`go`, `react`, `swift`, `spring`) — tags only when the same text carries at
least one strong match. That rule is written for whole documents. A requirement is one line with
nothing to corroborate against, so `"5+ years of Go"` parses to nothing and
`"distributed systems in Go or Rust"` loses Go while keeping Rust. The flagship 40-point category
would have evaporated on precisely the most ordinary requirements.

`skilltag.Canonicalize` resolves a token without corroboration, but its own contract says why it may:
it is for tokens a caller *asserts*, not prose to be interpreted. A requirement is prose.

So the resolution runs the other way round. `job.Skills` were resolved from the full description,
where the context to disambiguate existed. For each requirement we ask which of those already-known
skills its text names — canonicalizing the line's words and phrases and keeping only what the
vacancy already carries. Nothing is discovered, so nothing needs corroborating.

- **names ≥1 vacancy skill** → checkable. Covered when every skill it names is in the document's
  parsed skill set. Weighted 2 for `required`, 1 for `preferred`.
- **names none** → unverifiable. Excluded from the denominator; the cached LLM status rides along in
  the wire shape, labelled, so the panel can still show what the earlier analysis thought without
  the score depending on it.

The "every skill it names" rule (rather than "any") is the strict reading, chosen because a
requirement's text usually names one skill; when it names three, partial coverage is not coverage.

This also buys an invariant the first draft lacked: Requirements Coverage cannot reference a skill
outside `job.Skills`, so it and Keyword Match draw from one set and cannot disagree about what the
vacancy asks for.

Reading the same cached row the tailoring agent already reads (`cachedAnalysisCtx`) means no new
query and no new staleness rule.

### Job Title Match and Seniority Fit read the dictionaries, and stop where the dictionaries stop

**Job Title Match (20).** Two line items: the vacancy's normalized title occurring in the
document's normalized text (12), and the vacancy's role category from `classify.Parse(job.Title)`
appearing among `classify.Categories(cvText)` (8). Normalization is lowercase plus whitespace
collapse — the role-fingerprint machinery is not reused, because it exists to make two *postings*
comparable and strips decorations a CV legitimately carries.

**Seniority Fit (10).** `classify.Parse(job.Title).Seniority` against `classify.Parse(cvText)
.Seniority`. `Parse` walks `seniorityTable` in precedence order and returns the first alias present,
so run over a whole CV it yields the highest grade the document claims — which is the right reading
for a CV. Distance is the index gap in `vocab.SeniorityValues` (`intern … c_level`): 0 → full, 1 →
half, ≥2 → none. Either side resolving to `""` makes the category unavailable, never a mismatch.

Seniority is the weakest signal here and is weighted 10 accordingly: it barely moves under
tailoring, which is exactly why the candidate should not be pushed to chase it.

### One render, not two — which is what makes the autosave cadence affordable

The ATS delta renders the base *and* the tailored CV on every read; the job-match score renders only
the tailored one. That halving is what lets Job Match refresh after every persisted edit while the
delta stays on its existing open/after-run cadence.

`cvHandlers.scoreRenderedCV` splits: the render-and-extract half becomes `renderedCVText`, shared by
both endpoints, and each endpoint applies its own scorer to the text. The missing-toolchain check
moves with it, so both endpoints degrade to `available: false` through one path.

Client-side, the refresh is chained off the existing `persist()` success rather than off the
`$effect` that schedules it — an in-flight save must land before the score is read, or the number
describes the previous document.

### The wire shape mirrors the ATS delta's, including its degraded envelope

    { available: bool, reason?: string, score?: { overall, categories[], ... } }

An unavailable score is an absence, not an error — the same decision `AtsDelta` already documents,
and for the same reason: a workspace that says "score unavailable" teaches the candidate to ignore
the panel.

### `CategoryChange` gains the tailored side's line items

Additive on `atscheck.Delta`: each category change carries the tailored report's `Items`. The base
side's items are deliberately not carried — the candidate is editing the tailored copy, and a
before/after list of individual checks is a diff nobody asked for.

### The panel splits along the axis each tab measures

`ArtifactPanel`'s `Tab` union goes from `'templates' | 'jd' | 'verdict'` to `'templates' | 'jd' |
'jobmatch' | 'score'`, and the page's `MobileView` follows. `JobMatch.svelte` and the reworked
`AtsDelta.svelte` share one `ScoreCategoryRow.svelte` disclosure — the two category shapes differ in
their fields but not in how a row reads, and the DS convention (`$lib/ui` re-exports) keeps that
component out of the tailor folder only if it proves reusable elsewhere; it starts local.

The view-model logic — the earned÷possible arithmetic as the client re-states it for display, the
impact label thresholds, the signed-number rendering — lands in `jobmatch.ts` beside `atsdelta.ts`
and is unit-tested with vitest, matching how `atsdelta.ts` already keeps wording out of the
component.

**View Job** renders in the panel header (visible from every tab, not just Job description), links
to `resolve('/jobs/[slug]', { slug })`, and is omitted when the workspace has no job — the same
`job` prop that already drives the JD header.

## Risks / Trade-offs

**Two scores in one workspace read as competitors.** → They live in separate tabs with headings that
name their baseline, and the Job Match tab labels the frozen LLM verdict explicitly. This is the
single largest UX risk of the change and the tab split is the mitigation.

**Keyword coverage is visible in both tabs** (Keyword Match at weight 30; Keyword Strength inside the
delta at 40). → Both derive from one `skilltag.Parse` of one rendered text against one `job.Skills`,
so they can differ in weight but never in *which* skills are missing. Accepted rather than resolved:
removing Keyword Strength from `atscheck` would break the standalone CV ATS page and its spec.

**The requirement matcher will produce false "missing" for a skill our dictionary lacks an alias
for.** → The failure mode is bounded to naming a skill the candidate then adds by hand, and it is
the same dictionary the rest of the product is scored on. The unverifiable path covers requirements
with *no* skill; it cannot cover a skill the dictionary mis-parses. Mining the mismatches later is
the existing dict-expansion loop.

**A render per autosave.** → Autosave is already debounced at 800ms and the score is one render, not
two. It is fetched without `await` blocking the editor and a failure degrades to absence. If it
proves heavy in practice the cadence drops to "on idle" without any wire change.

**The mobile tab bar reaches eight tabs.** → It is already horizontally scrollable and gained
Settings without redesign; eight is the point at which grouping starts to be worth it, and this
change notes that rather than acting on it.

## Migration Plan

Additive throughout: one new endpoint, one additive field on an existing response, one new tab. No
migration, no persisted state, no backfill. The scorer is pure and I/O-free, so it is unit-testable
without Docker; the endpoint follows the existing `cv_ats_delta` integration-test harness.

Rollback is deleting the tab — the endpoint answers nothing anything else depends on.

## Open Questions

- Whether Requirements Coverage should count a requirement covered when the document names *most* of
  its skills rather than all. Deferred until there is a corpus to calibrate against; strict is the
  conservative default and errs toward telling the candidate to add something.
- Whether the Score tab should also refresh on autosave once the two-render cost is measured on
  prod-sized CVs.
