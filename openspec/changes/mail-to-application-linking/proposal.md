## Why

The `/my/inbox` mail feature and the `/my/tracking` application tracker are two disconnected surfaces: recruiter mail lands in a flat inbox with no relationship to the application it belongs to, and an application shows no trace of the correspondence it generated. A user reconstructing "where does this application stand?" has to eyeball the inbox by hand. Real inbox data (237 messages) shows the signal is there to close the gap automatically — every message names its company in the sender name or subject, and ~1 in 8 is an interview invitation worth acting on — but nothing links the two today.

## What Changes

- Introduce a background classification pipeline (transactional-outbox + cron worker, the project's existing `enrich` idiom) that, for each inbox email, resolves **which application it belongs to** and **what status it signals**.
- Match an email to one of the caller's own open applications via a deterministic cascade — thread continuity → company name extracted from `from_name`/`subject` (normalized, ATS-pseudonym-filtered) fuzzy-matched against the caller's tracked companies → LLM disambiguation of the tail. Sender-domain matching against `companies.domains` is explicitly **not** used (0/237 hits — the domains are ATS relays, not companies).
- Classify each email into a controlled status vocabulary (`acknowledgement / screening / interview_invitation / assessment / offer / rejection / info_request / other`), sanitized to the vocabulary before persistence (same "never persist an out-of-vocabulary value" invariant as enrich; also the prompt-injection guard for untrusted email bodies).
- Link by **confidence tier**: a high-confidence match auto-links (`link_source=auto`, undoable); a lower-confidence match is stored as a *suggestion* and confirmed inline in the `/my/inbox` reading pane ("Похоже на Acme — Привязать / Не то"); no candidate stays unlinked with a manual search fallback.
- Advance the application `stage` **monotonically forward only, at high confidence** from a classified email; otherwise surface it as a suggestion (never auto-move backward — e.g. a `rejection` never silently closes the pipeline).
- Add an application **detail page** `/my/tracking/[slug]` listing the application's linked emails with status badges, and a per-email link back to its application from the inbox.
- **Roadmap (not in this change):** an interview-prep durable workflow (`interview_invitation` → extract date → durable wait → company brief → send email) is documented in design as the follow-on sub-project where a durable-workflow engine (Inngest/Temporal) is evaluated. This change deliberately stops at classification + linking, which needs no such engine.

## Capabilities

### New Capabilities
- `email-application-linking`: resolve an inbox email to one of the caller's applications and classify its status; the outbox+worker pipeline, the deterministic-then-LLM match cascade, the status vocabulary, the confidence-tiered auto/suggested linking, and the monotonic-forward stage advancement.

### Modified Capabilities
- `email-inbox`: an email now carries its resolved/suggested application and status signal; the reading pane renders an inline link-confirmation chip and a link to the application.
- `user-job-tracking`: a new per-application detail view exposes the application's linked emails with status badges; a high-confidence classified email may advance the application stage forward.

## Impact

- **Schema:** new columns on `emails` (`job_id`, `suggested_job_id`, `link_source`, `match_confidence`, `status_signal`, `classified_at`, `classification_model`); new `email_classification_outbox` queue table; migration ~`0017` (apply to prod manually before deploy, per the migrations convention).
- **New code:** `internal/mailmatch` (deterministic match cascade, unit-tested), `internal/mailclassify` (LLM status contract + `Sanitize`), `cmd/classify-mail` (cron worker draining the outbox), plus enqueue-on-insert wired into the Gmail-sync / hosted-ingest write paths.
- **API:** `GET /api/v1/me/tracking/:slug` (application detail with linked emails); email link confirm/reject and manual-link endpoints under `/me/emails`; the inbox email read shape gains link/status fields. All moderator-only, matching the existing inbox gate.
- **Frontend:** new `/my/tracking/[slug]` route; inbox reading-pane link chip; application-link affordance on emails.
- **Dependencies:** reuses `internal/llm` (`LLM_*`), the existing outbox/worker/`systemd`-timer machinery, and `user_jobs`/`jobs`/`companies` reads. No new infrastructure.
