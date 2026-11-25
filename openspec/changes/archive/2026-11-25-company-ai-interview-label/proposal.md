## Why

A candidate cannot find out that an employer screens with an AI interviewer until
they are already inside one. The fact is knowable — somebody always goes first —
but nothing in the catalogue carries it, so every candidate rediscovers it alone,
one 30-minute session at a time.

This is not a hypothetical. micro1 states it runs 3,000+ AI interviews a day and
its candidate privacy notice reserves the right to train its own models on the
audio, video, screen recording and proctoring telemetry those sessions produce;
Mercor publishes the opposite commitment for the same mechanic. Both companies are
in this catalogue. A candidate choosing between them today has no way to see the
difference from a job card, and the platforms that could tell them have no
incentive to.

Nothing here judges the practice. Plenty of people prefer an AI screen — it is
fast, it runs at 2am, and it does not sigh at a gap in a CV. The point is that it
is a **material fact about how an employer hires**, and material facts belong on
the card rather than in a thread somebody has to find.

## What Changes

- **The report dialog's picker gains `AI interview`, and it files evidence rather
  than a moderation report.** The dialog already splits on `isEvidenceReason`:
  `no_response` goes to its own endpoint and accumulates into the ghost signal
  instead of reaching a moderator whose only lever is closing the job.
  `AI interview` takes that same branch for the same reason — there is nothing for
  a moderator to *do* about a true statement. The moderation endpoint's own
  `reason` vocabulary is untouched.
- **The evidence attaches to the company, not the posting.** An AI interviewer is
  a property of an employer's hiring process. Filed per-posting the signal would
  never reach useful coverage: micro1 alone has 203 open postings, each of which
  would need its own reporter. Filed per-company, one report covers all of them.
- **A new `company_process_reports` table**, modelled on `ghost_reports`: one row
  per `(user, company, kind)`, retraction rather than deletion, and a `kind`
  column constrained to a controlled list. One value ships (`ai_interview`); the
  column exists because the same shape covers the other facts candidates
  repeatedly discover too late — an unpaid test task, government ID demanded
  before an offer — and three near-identical tables would be the worse answer.
- **A materialised counter on `companies`**, recomputed in the writing
  transaction, the same pattern `feedback_count` and `upvote_count` already use.
- **One report is enough to show the label.** `no_response` needs a contributor
  gate because one silence may be bad luck; "a bot conducted my interview" is an
  observation, not an inference. The count ships beside the label so a reader can
  weigh one report against forty.
- **A neutral badge** — `AI interview · N reports` — on the job card, the job page
  and the company page. Not a warning, not a colour that reads as one.
- **A search filter** that excludes employers carrying the label, reachable from
  the filter modal rather than only from the API.

## Capabilities

### New Capabilities

- `company-ai-interview-label`: candidates report that an employer screened them
  with an AI interviewer; the reports accumulate on the company, surface as a
  neutral counted badge wherever that company's jobs appear, and can be filtered
  out of search. Covers the evidence store, the retraction rule, the one-report
  threshold, the counter, the wire shape and the facet.

### Modified Capabilities

None.

`job-report` looks like a candidate and is not one. Its spec governs
`POST /api/v1/jobs/:slug/reports` and the moderator queue behind it; the
controlled `reason` vocabulary it enumerates belongs to that endpoint. This change
does not add a value to it — the evidence is company-scoped and goes to its own
endpoint, exactly as `no_response` already goes to its own (`reportGhostJob`)
rather than through the moderation route. What gains an entry is the report
dialog's picker, which is UI no spec currently governs, so the new capability's
spec owns it.

## Impact

**Database.** One new table (`company_process_reports`) and one new column on
`companies`. Both are additive. Per `migrations/AGENTS.md` conventions the
migration must be applied to prod **before** the binary that reads the column
rolls out, or every company read answers 42703 → 500.

**Backend.** A new `engage`-block service owning the company-level evidence and
its counter, `internal/api/handler` (two routes), `internal/job/jobview` (wire
shape), `internal/search/search` (a filterable attribute), `internal/platform/db`
(queries + `make sqlc`). `internal/engage/report` is **not** touched: its reason
vocabulary and moderation queue are unchanged.

**Search — two known traps, both load-bearing.**

1. The new filterable attribute's settings patch MUST reach the **live** index
   before the binary that queries it ships. A settings patch replaces
   `filterableAttributes` wholesale, so a gap turns every request for the new
   value into a Meili 400 — which the search package maps to a 500 for *every*
   caller of the affected filter, not only the one who ticked the new box. The
   `search-settings-drift` worker exists to make this gap visible; it does not
   close it.
2. A **full reindex** is required after the first labels land. Incremental
   `search_outbox` pushes only send documents whose `content_hash` moved, and this
   field is not part of that hash — the same trap `is_tech` and
   `requires_clearance` both fell into.

**Frontend.** `ReportDialog.svelte` (one reason, one branch), the job card and job
page badge, the company page badge, the filter modal, and `$lib/reports.ts` where
the reason vocabulary and the evidence split are pinned by a test.

**Not touched.** `company_feedback` is deliberately not reused: it is a 1–5 star
review with free text, one row per user per company. A star rating forces a
judgement where this needs a fact, and its uniqueness bound would mean a user who
already reviewed the compensation could never report the interview.
