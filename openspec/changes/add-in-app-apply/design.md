## Context

Employer accounts (`internal/ingest/employer`, the unarchived `add-employer-company-accounts`
change) let a verified employer publish vacancies, but every vacancy is minted through
`moderation.Service.Create`, whose `CreateInput.Validate` requires an absolute http(s) URL and
whose `derive` uses that URL as `external_id` — the `(source, external_id)` dedup key. So a
vacancy that is not hosted elsewhere cannot exist today.

What already exists and is reused:

- `apply_forms` (job_id → `applyform.Form` JSONB) and `GET /jobs/:slug/apply-form`.
  `applyform.FieldType` already names `text`, `boolean` and `select`.
- `cv.Store.Get` + `cv.Renderer.Render` (Typst) render a user's CV to PDF on the API server;
  the handler already builds a shared renderer, nil when `TYPST_BIN` is absent.
- `emailnotify.Message` carries `ReplyTo` and `Attachments`; `GroupEssential` is the
  non-silenceable group. `handler.Register` already builds the SES client `ec`.
- `jobtracking.Service.MarkApplied(ctx, userID, slug, source)`.
- `answerbank.Store.Sendable` (topic → candidate-provided answer) and `Save`, keyed by
  `answertopic.Of(question)`; `screeninganswers.Store.Get` for the six typed facts.
- Email verification gating is done in SQL against `users.email_verified`, mapped to a domain
  error (`CreateAPIKey`, `CreateGhostReport`).

## Goals / Non-Goals

**Goals:**
- An employer vacancy in `freehire` apply mode, with up to ten typed questions.
- A candidate application, validated against the form, delivered once by email to the
  employer's confirmed address, retried rather than duplicated on failure.
- Tracking and answer-bank side effects; a per-day cap; the job page and `directApply`.

**Non-Goals:**
- An applicant inbox or statuses for the employer (seam: `direct_applications` already holds
  every application; a dashboard reads it later).
- Payment for posting; auto-apply onto these forms; file-upload questions; staff-authored
  (`source='manual'`) vacancies in this mode.

## Decisions

### 1. Apply mode is a property of the external id, not a new column
A `freehire`-mode vacancy is minted with `external_id = 'freehire:' + uuid` and `url = ''`.
`jobs.apply_mode` would mean an `ALTER` on a 12.7M-row table for a value true of a handful of
rows, and a separate marker written after the mint (e.g. "has a `freehire` apply_forms row")
could disagree with the row if the second write failed. The prefix is written in the same
statement as the row, so the two cannot disagree, and the mode is immutable by construction —
the external id is never recomputed. One predicate owns it: `job.IsDirectApply(source,
externalID)` (`source == "employer"` and the prefix), used by jobview, directapply and the SQL
guard alike (the SQL repeats it as `source = 'employer' AND external_id LIKE 'freehire:%'`).

`moderation.CreateInput` gains `ExternalID string`: when set, `URL` must be empty and
`Validate` skips the URL check; when unset, behaviour is unchanged. Submissions never set it.
Alternative considered: a fake URL pointing at our own job page — rejected, because the slug
depends on the external id (circular) and every reader of `jobs.url` (liveness probe, ghost
crosscheck, link resolution) would fetch our own site as if it were the employer's.

`url = ''` is the honest value. The audit of every reader of `jobs.url` (task 1.3) found no
crash and no dedup collision — `(source, external_id)` is the key and `url` carries no unique
index — and these consequences, each handled:

- `cmd/liveness`'s orphan probe DID reach employer rows (`employer` is no ATS provider) and
  would request `""` on every run: `SelectOrphanLivenessCandidates` now skips `url = ''`.
- `FindOpenJobByURL` matched a link normalizing to `''` against such a row: it now matches
  nothing for an empty normalized link.
- OJCP reported an `external_redirect` with no URL (or, once a `freehire` form exists, an
  `ats_direct` path naming `freehire` as an ATS): it now reports `platform_native` pointing at
  the posting's page, and the manifest declares that type.
- The web apply CTA, the job card's Apply shortcut and the "did you apply?" prompt would
  render a button going nowhere; the employer edit form refuses to save with an empty url.
  Both belong to the web tasks (6.2, 6.3, 6.4).
- `employer.CreateVacancy`'s `Owner(url)` race guard is meaningless for a minted id and is
  skipped in that mode (task 2.2) — a UUID cannot collide.
- Left as is: the saved-search webhook payload, `ListJobCopies.apply_url` and the enrichment
  prompt's `URL:` line already carry an empty value for crawled rows stored without one, and
  the webhook and copies both carry the slug beside it.

### 2. Questions live in `apply_forms` with provider `freehire`
The same `applyform.Form` shape, so the existing display path keeps working. Absent row means
zero questions. A question's `Field.ID` is server-minted (`q1`…`qN` in submitted order)
so answers reference ids, never labels. Validation and construction
(`applyform.FreehireForm`) are pure and run before the mint, so a malformed form never leaves
a vacancy behind. They live in `applyform`, the package that owns the form shape, and not in
`directapply`: `directapply` imports `employer` (for the account), and `employer` needs the
form builder, so placing it in `directapply` would be an import cycle. The form is written after the mint in a separate statement; if that
write fails, the vacancy just minted is closed and the request returns 500. Closing matters
because there is no URL to upsert on: the employer's retry mints a second vacancy, and the
first would otherwise stay live beside it asking none of the questions. A failed question
edit returns 500 with the content change already saved; the employer saves again.

### 3. A new package `internal/ingest/directapply`
The candidate's application is not an employer capability, so it does not join
`employer.Service`'s two halves. It sits in `ingest` (layer 7) because it needs the employer
account (`employer.Service.ActiveAccount` and the confirmed work email), which `application`
(layer 6) cannot import. It imports `candidate` (cv, answerbank), `application/jobtracking`
and `ingest/screeninganswers`, all below or beside it. Mail goes through a port,
`directapply.Mailer`, implemented in `emailnotify` and wired in `handler.Register` exactly as
`employer.ClaimMailer` is — `ingest` and `engage` share a layer and cannot see each other.
Added to `internal/platform/arch/layering/blocks.go`.

### 4. `direct_applications`: record first, stamp sent after
```
id bigserial PK, job_id → jobs ON DELETE CASCADE, user_id → users ON DELETE CASCADE,
cv_id uuid NOT NULL, note text NOT NULL DEFAULT '', answers jsonb NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(), sending_at timestamptz NULL,
sent_at timestamptz NULL,
UNIQUE (job_id, user_id)
```
`answers` is a snapshot `[{id, label, type, answer}]`, so the record still reads correctly
after the employer edits the questions. Flow of `Apply`:
1. Load the vacancy; `IsDirectApply` and open, else not found / closed. Its own employer
   applying is answered not found.
2. `ActiveAccount` for the vacancy's `created_by`, else refused.
3. Load the user: verified email, else `ErrUnverified`.
4. Validate answers against the current form (pure: required, boolean, select options,
   unknown ids, note ≤ 2000).
5. A cheap `State` read refuses an application already sent or in flight before anything
   is rendered. Then render the chosen CV (`CVs.PDF`) — not owned is not found. It is the
   same render as the CV page's own download, tracer links included when the candidate
   turned them on: an employer opening an application is exactly what they exist to report. Rendering before the record
   adds no exposure: a signed-in user can already render their own CV at will through
   `GET /cvs/:id/pdf`.
6. `UpsertDirectApplication` records the row and CLAIMS it (`sending_at = now()`):
   `INSERT … ON CONFLICT (job_id, user_id) DO UPDATE SET cv_id, note, answers, sending_at
   WHERE sent_at IS NULL AND (sending_at IS NULL OR sending_at < now() - 2 minutes)
   RETURNING id`. No row back is read through `GetDirectApplication`: sent →
   `ErrAlreadyApplied` (409), a live claim → `ErrInProgress` (409, a double click or a client
   retrying a slow response, which would otherwise email the employer twice), else the cap. The daily cap is checked in the same
   statement's CTE as `count(*) WHERE user_id = $1 AND created_at >= date_trunc('day', now()
   AT TIME ZONE 'UTC')`, excluding this job's own row so a retry never counts twice.
7. Send the mail; on error release the claim and return `ErrDelivery` (502), `sent_at`
   still null.
8. `MarkDirectApplicationSent` stamps `sent_at` and drops the claim. A claim abandoned by a
   request that died mid-send expires after two minutes.
9. `MarkApplied(…, appevent.SourceUser)`; `answerbank.Save` per answer with
   `AuthorCandidate`, errors logged, never returned.

A crash between 7 and 8 sends twice on the next retry. Accepted: the opposite order (stamp
then send) loses an application silently, which is worse than a duplicate email.

### 5. The cap is an env knob read once at startup
`DIRECT_APPLY_DAILY_LIMIT`, default 30, read in `internal/platform/config` and rejected at
startup when set but not a positive integer. It is not a `plan` feature: `internal/ai/plan`
meters AI actions, and an application is not one.

### 6. The read endpoint carries the prefill
`GET /jobs/:slug/direct-apply` (cookie session optional — `optionalCookie`, never an API
key, since the response carries the caller's banked answers): company name, questions with id, type,
options, required; and for a signed-in user, `prefill` (field id → answer) and `applied`
(sent_at set). Prefill resolves each question's `answertopic.Of(label)` against
`answerbank.Sendable` — the candidate's own answers only, never an agent's suggestion. A
banked "Yes"/"No" prefills a yes/no question; a banked value prefills a list question only
when it is one of its options. The six typed screening answers are not consulted: their
mapping is keyed by ATS field identifiers our questions do not carry, and the bank already
holds what the candidate answered on earlier on-site applications. The existing
`/apply-form` endpoint stays for captured ATS forms.
`POST /jobs/:slug/direct-apply` (cookie only) submits.

### 7. Wire and page
`jobview` gains `direct_apply: true`, omitted (not `false`) for every other posting so the
search index carries nothing new for the rest of the catalogue. For such a vacancy `url` is
`""` and the web CTA opens `DirectApplyForm.svelte` instead of a link; the badge reads "Only
on freehire". `jobPostingJsonLd` adds `directApply: true` only when `job.direct_apply`.

## Risks / Trade-offs

- [A reader of `jobs.url` mishandles `''`] → task 1.3 audits them before anything mints one.
- [Double-submit of the create form mints two vacancies, since there is no URL to upsert on]
  → the dashboard disables submit while in flight; the employer can close the extra one.
- [SES sandbox or a bounce loses an application] → `sent_at` stays null and the candidate sees
  the failure; the record survives for an operator to inspect.
- [Spam through the employer's inbox] → verified-email gate plus the daily cap; the employer
  can close the vacancy.
- [Typst missing on a host] → `Apply` refuses with 501 before recording anything, the same
  rule `GetCVPDF` follows.

## Migration Plan

One additive migration (`direct_applications`). `release.sh` migrates before the flip.
Rollback: no vacancy can be in `freehire` mode until the dashboard offers it, so reverting
the binary is enough; the table can stay.
