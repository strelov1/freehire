## Context

`internal/companyfeedback` already lets a signed-in user post a 1-5 star rating
+ closed category + free text about a company, shown under their site-wide
pseudonymous persona. `interview` is already one of the seven categories
(`vocab.CompanyFeedbackTypeValues`). What it doesn't do: capture the shape of
an interview process (how many rounds, what each covered), or reduce the
friction of writing one from scratch.

Separately, `internal/assistant`'s `debrief` preset already exists for a
candidate to reflect on a completed interview with the in-app assistant,
bound to one `(user, job)` via `assistant_sessions.job_id` — the same binding
the `interview`/rehearsal preset uses. It writes structured achievements into
`internal/experience` (the CV-facing "experience bank"), but nothing in that
bank is publishable outside the candidate's own CV — every atom stays private
to its owner.

This spec connects the two: a "share as review" step at the end of a debrief
session that drafts an interview-specific review from the conversation and
publishes it through `companyfeedback`, once the candidate edits and confirms
it. It is deliberately scoped to *this* increment — see Non-Goals for two
adjacent ideas that came up during design and were deliberately deferred.

## Goals / Non-Goals

**Goals:**

- A `company_feedback` row with `feedback_type = 'interview'` can carry
  structured interview detail: the role title, an ordered list of rounds
  (short label each), a difficulty rating, a process-honesty rating, and an
  outcome — in addition to the existing rating/body/category fields every
  feedback type already has.
- A candidate in a `debrief` session can turn that conversation into a
  prefilled draft of this shape with one action, edit every field, and
  publish it — always under their existing pseudonymous persona, never their
  name.
- A job posting page shows an aggregate interview-experience summary
  (difficulty avg, honesty avg, outcome distribution) scoped to reviews whose
  role matches this posting's role, with a link to the full list.
- Published text can never carry an accidental self-identifying detail
  (name, exact unique dates, contact info) — checked before, not after,
  publish.

**Non-Goals:**

- No change to `internal/userjob`'s stage model (still one flat `interview`
  stage, not round-aware). Raised during design as a plausibly-related
  follow-up; tracked as an open question on
  [#1625](https://github.com/strelov1/freehire/issues/1625), not designed or
  scoped here.
- No aggregate application-funnel statistics (applicant counts, stage
  timing) on the job page — that is #1625's scope, sourced from
  `internal/userjob` tracking data, no free text involved. This spec's job-page
  section and that one are visually distinct per #1625's tracking comment,
  revisited once both exist.
- No "share as review" entry point outside a live debrief session (e.g. from
  session history after the fact). If a candidate closes the session without
  sharing, they can only redo it from a new debrief session for now.
- No human moderation queue for this content beyond what `companyfeedback`
  already has (`Report`/`Hide`, `internal/handler/company_feedback.go`) — the
  new LLM pre-publish check (Decision 4) is additive to that, not a
  replacement.
- No new `assistant` tool, no change to the debrief preset's tool registry or
  system prompt, and no new SSE event type. The "share as review?" prompt is
  a client-side UI affordance, not a model-generated turn — this keeps
  `TestTheDebriefCarriesTheRehearsalsTools` (which asserts debrief and the
  interview-rehearsal preset carry the identical tool set) untouched.

## Decisions

### 1. Interview-specific fields live as nullable columns on `company_feedback`

**Choice:** Extend the existing table rather than add a child table:

```sql
ALTER TABLE public.company_feedback
    ADD COLUMN role_title text,
    ADD COLUMN interview_rounds jsonb,
    ADD COLUMN interview_difficulty smallint,
    ADD COLUMN interview_honesty smallint,
    ADD COLUMN interview_outcome text,
    -- role_category/role_seniority: internal/classify's dict-only projection of
    -- role_title, populated only when it resolves (see Decision 2) — the job
    -- page's aggregate (Decision 5) filters on these, never on role_title text.
    ADD COLUMN role_category text,
    ADD COLUMN role_seniority text;

ALTER TABLE public.company_feedback
    ADD CONSTRAINT company_feedback_interview_fields_check CHECK (
        feedback_type = 'interview'
        OR (role_title IS NULL AND interview_rounds IS NULL
            AND interview_difficulty IS NULL AND interview_honesty IS NULL
            AND interview_outcome IS NULL AND role_category IS NULL
            AND role_seniority IS NULL)
    ) NOT VALID;

ALTER TABLE public.company_feedback
    ADD CONSTRAINT company_feedback_interview_difficulty_check
        CHECK (interview_difficulty IS NULL OR interview_difficulty BETWEEN 1 AND 5) NOT VALID;
ALTER TABLE public.company_feedback
    ADD CONSTRAINT company_feedback_interview_honesty_check
        CHECK (interview_honesty IS NULL OR interview_honesty BETWEEN 1 AND 5) NOT VALID;
ALTER TABLE public.company_feedback
    ADD CONSTRAINT company_feedback_interview_outcome_check
        CHECK (interview_outcome IS NULL OR interview_outcome = ANY (ARRAY[
            'offer', 'rejected', 'no_response', 'withdrew'
        ]::text[])) NOT VALID;

-- Then, in a follow-up statement (can run same migration, after a moment —
-- see hire-migration-add-constraint-not-valid convention):
-- VALIDATE CONSTRAINT company_feedback_interview_fields_check; (etc.)
```

`interview_rounds` is a plain JSON array of strings (each a short label like
`"Take-home — API design exercise"`), order = array order. No round count/date
columns — that granularity was explicitly ruled out as `internal/userjob`
territory (Non-Goals).

**Why:** One row per `(user, company)` is already `company_feedback`'s shape
(enforced by `company_feedback_user_company_uniq_idx`), and every existing
read path (`List`, `Mine`, the materialized `feedback_count`/
`feedback_rating_avg` counters) already assumes one row = one review. A
sibling `interview_review_details` table one-to-one with `company_feedback`
would require every read to join or N+1, for no isolation benefit — nothing
about these columns needs independent existence, lifecycle, or access
control from the row they describe. `NOT VALID` + a later `VALIDATE
CONSTRAINT` follows the existing convention for adding a CHECK to a table
already in use, so the validation scan doesn't hold a blocking lock (per
`hire-migration-add-constraint-not-valid`).

**Alternatives considered:** Reuse the existing `body` field to hold the
round list as formatted text (e.g. markdown), parsed back out for display.
Rejected: round labels need to render as a structured list (per the approved
mockup) and be independently editable in the form (add/remove/reorder) — text
parsing round-trips would be fragile for something this structured, and the
existing `body` field still has a job (the free-text pull-quote/summary).

### 2. `role_title` is a copied string, not a foreign key

**Choice:** `role_title` is plain text, defaulted from the debrief session's
bound job's title at draft time, editable, and never validated against — or
linked to — any `jobs` row.

**Why:** A published review must stay meaningful after the job posting it
came from closes, reposts, or is retitled — a `job_id` FK would either cascade
the review away (`ON DELETE CASCADE`) or need `ON DELETE SET NULL`, losing
the role context entirely. This matches the earlier product decision that a
review shows a role title, never a link to a specific vacancy.

**Job-page matching:** rather than exact/substring text matching between a
review's `role_title` and the currently-viewed job's title (fragile — "Sr.
Backend Engineer" vs "Senior Backend Engineer" wouldn't match), run
`role_title` through the existing `internal/classify` dict-only
title→(category, seniority) classifier at write time and store the resulting
facets alongside (two more nullable columns:
`role_category text`, `role_seniority text`, populated only when the
classifier resolves — dict-only, never guessed, same rule every other facet
in this codebase follows). The job page's aggregate section (Decision 5)
filters reviews by matching the *viewed job's own* already-computed
`category`/`seniority` facets, not by string comparison. A `role_title` the
classifier can't place still displays on the company page as free text; it
just doesn't surface in any job page's aggregate.

### 3. Draft generation: one on-demand LLM call, triggered by a client-side UI affordance

**Choice:** No new `assistant` tool (per Non-Goals). Instead:

- Frontend: after a debrief session has a handful of exchanges, show a
  dismissible banner in the session view: *"Share this as an anonymous public
  review?"* → **Share as review**. Exact turn-count threshold is an
  implementation detail, not a design decision — it only affects when the
  banner appears, never whether the flow works.
- Clicking it calls a new endpoint, `POST
  /assistant/sessions/:id/interview-review-draft`, restricted to sessions
  with `preset = debrief` (403 otherwise, same guard shape as the existing
  opening/tool endpoints).
- The handler reads the session's full transcript (`Store.Messages`, already
  used to render the chat) and its bound job's title/company (`Session.JobID`
  → existing job/company lookup, the same one the debrief preset's opening
  brief already uses), then calls a new `companyfeedback.DraftInterviewReview`
  helper: one `internal/llm` `GenerateJSON` call with a schema-constrained
  response shaped like the new columns
  (`role_title`, `rounds []string`, `body`, `difficulty`, `honesty`,
  `outcome`), on the caller's own gateway credential (`internal/llmkey`,
  `.As(ctx, userID)`) tagged `feature:company_review_draft` — the same
  per-user attribution every other in-app-assistant-adjacent LLM call
  follows.
- The response is returned to the client to prefill the form (Decision 6). It
  is **never written to the database** at this step — a candidate who closes
  the tab loses nothing but the draft.

**Why this shape over an assistant tool:** considered letting the assistant
itself propose the share step mid-conversation (a new tool call, rendered as
a special chat card). Rejected: it would need a new debrief-only tool,
breaking the deliberate tool-set equality the existing
`TestTheDebriefCarriesTheRehearsalsTools` enforces between `debrief` and the
interview-rehearsal preset — a decision that test's own history says was
made deliberately, not accidentally. A plain endpoint called from a
client-side affordance gets the same outcome (a drafted review, prefillable
in one click) without touching assistant internals at all.

**Failure mode:** if the draft call fails (LLM outage, timeout), the client
opens the form empty rather than blocking the flow — drafting is a
convenience, not a gate. Contrast with Decision 4, which is a hard gate.

### 4. Publish-time moderation is a second LLM call, fail-closed

**Choice:** `companyfeedback.Service.Upsert` (extended to accept the new
optional interview fields) runs one additional `internal/llm` call before
the existing DB write, when `feedback_type == "interview"`:
`ModerateInterviewReview(ctx, userID, draft) (cleaned Feedback, ok bool,
reason string, err error)`, tagged `feature:company_review_moderate` on the
same per-user credential. It checks the combined text (rounds + body) for:
spam/off-topic content, and any apparent self-identifying detail (real name,
exact company-internal detail, contact info) — returning either an approval
or a rejection reason surfaced back to the submitter so they can edit and
resubmit.

If the call itself fails (not "rejected" — errors out, e.g. LLM
unreachable), `Upsert` returns an error and **does not publish** — the same
fail-closed posture `internal/pii` already takes for CV→LLM redaction. An
unmoderated review is worse than a delayed one; nothing about this content is
time-sensitive enough to justify publishing without the check.

**Why a second call instead of one combined draft+moderate call:** the draft
step runs on an unedited transcript the candidate hasn't seen yet; the
moderation step must run on what they're *actually about to publish*, after
their edits — a candidate could rewrite the draft into something the first
pass never saw. Two calls at two different trust boundaries, not one call
serving both purposes.

**Alternatives considered:** reuse `internal/pii`'s `Redactor`. Rejected per
the codebase's own prior finding on this: it round-trips CV text through an
LLM call and restores placeholders — built for masking-then-restoring inbound
CV content, not for a one-way accept/reject/clean judgment on outbound
candidate prose. Building a second, purpose-fit check is less code than
bending `Redactor`'s restore-half to a use case it was never shaped for.

### 5. Job page: a new aggregate section, company page: an extended card style

**Choice:**
- Company page (`web/src/routes/companies/[slug]/`): the existing feedback
  dialog's list gains a card variant for `feedback_type === 'interview'` —
  role badge, numbered round list, italic pull-quote (`body`), then a
  difficulty/honesty/outcome footer row. Non-interview categories render
  exactly as today (plain rating + body).
- Job page (`web/src/routes/jobs/[slug]/` or wherever the posting view lives):
  a new "Interview experience" section under the description — difficulty
  avg, honesty avg, offer-outcome %, and a "See N reviews →" link that opens
  the company's feedback list pre-filtered to this job's `(category,
  seniority)` facets (Decision 2). Requires a new aggregate read:
  `companyfeedback.InterviewSummary(ctx, companySlug, category, seniority)
  (Summary, error)` — count + three averages, computed from visible rows
  only (same `status = 'visible'` filter `List`/`Count` already apply).
- No section renders when a job's `(category, seniority)` has zero matching
  reviews — no empty-state clutter on the common case (most postings, most of
  the time, at launch).

**Why:** both surfaces were validated against mockups of the real company and
job pages during design (round-by-round card, "Interview experience" block
placement). Filtering the job-page aggregate to facet match rather than
company-wide avoids a Community Manager posting showing Engineering
interview difficulty.

## Risks / Trade-offs

- **[Two LLM calls per publish attempt]** → draft (Decision 3) is
  best-effort and cheap to skip on failure; moderation (Decision 4) is
  mandatory and fail-closed. Total added LLM cost is one draft + one
  moderate per *published* review (edits/retries only re-run moderation, not
  drafting, since the draft step is a one-time prefill).
- **[`internal/classify` can't place every role_title]** → accepted
  (Decision 2): those reviews still show on the company page, just don't
  contribute to any job page's aggregate. No manual override in this
  increment.
- **[Cold start]** → same as any review feature: zero interview reviews exist
  until candidates who go through `debrief` start sharing. No seeding
  planned; the job-page section simply doesn't render until a role clears the
  first review.
- **[Round list can still leak identity even after moderation]** → the LLM
  moderation check is a best-effort filter, not a guarantee (same caveat
  every LLM-based gate in this codebase carries). `Report`/`Hide` remain the
  backstop for anything that gets through, unchanged from today.

## Migration Plan

1. Ship the migration (Decision 1: new nullable columns + `NOT VALID`
   constraints, validated in a follow-up statement) manually before deploy,
   same convention as 0088/0089.
2. Ship `internal/vocab.CompanyFeedbackOutcomeValues` and the
   `companyfeedback` service/query changes (extended `Upsert`, new
   `DraftInterviewReview`/`ModerateInterviewReview`/`InterviewSummary`)
   together with the two new handler endpoints.
3. Regenerate frontend contracts (`cmd/gen-contracts`) for the new fields and
   vocab.
4. Ship the frontend: company page card variant, job page section, debrief
   session banner + prefilled form.
5. No backfill — this is new data going forward only.
6. Rollback: revert the Go/frontend changes; the new columns stay
   `NULL`-only and harmless (no code path writes them) until redeployed.

## Testing

- `internal/companyfeedback`: `Upsert` validation for the new fields (rating
  bounds already covered; add difficulty/honesty bounds, outcome vocab
  membership, round-count/round-length bounds, and the
  interview-fields-null-for-non-interview invariant matching the DB CHECK).
- `internal/vocab`: extend the existing vocab membership test pattern to
  `CompanyFeedbackOutcomeValues`.
- `internal/classify`: no new tests needed — reused as-is; a
  `companyfeedback`-side test confirms `role_category`/`role_seniority` are
  populated only when the classifier resolves and left `NULL` otherwise.
- `internal/handler` integration: the new draft endpoint 403s outside a
  `debrief`-preset session; a full-loop test drafts → edits → publishes →
  reads back via `List`; moderation rejection surfaces its reason and does
  not write a row; a simulated moderation-call error leaves no row (the
  fail-closed path).
- `internal/handler` integration: `InterviewSummary` returns zero-value
  (no section) for a `(category, seniority)` with no reviews, and correct
  averages once some exist; hidden (`status = 'hidden'`) rows are excluded
  from it, matching `List`/`Count`.
- Assistant package: confirm `TestTheDebriefCarriesTheRehearsalsTools` still
  passes unmodified — the whole point of Decision 3 is that this feature
  doesn't touch it.
