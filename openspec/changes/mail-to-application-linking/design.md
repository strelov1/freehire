## Context

Two shipped, moderator-only surfaces are disconnected: `/my/inbox` (the `email-inbox` capability — a flat two-pane list over the `emails` table, fed by Gmail sync and a dormant hosted mailbox) and `/my/tracking` (the `user-job-tracking` capability — one `user_jobs` row per `(user_id, job_id)` with a controlled `stage` vocabulary). Recruiter mail names an application it belongs to, but nothing links the two.

A read of the live inbox (237 messages) grounded this design in real data:

- **Sender domains are ATS relays, not employers.** `ashbyhq.com` (45), `us.greenhouse-mail.io` (42), `hire.lever.co` (24), `smartrecruiters.com`, `myworkday.com`, `teamtailor`, `recruitee`, `icims`, `gem.com` dominate. Matching `from_addr` domain against `companies.domains` yields **0/237** — that path is dead.
- **Company identity lives in `from_name`/`subject`.** "Block Labs Hiring Team", "Sardine Hiring Team", "Thank you for applying to Hyperproof", "Your Application to Nametag". Subjects are near-templated.
- **Threads are mostly singletons** (237 emails / 221 `thread_id`s) — thread continuity is exact but rare.
- **Class balance** (rough keyword tally): `acknowledgement` ~116 (dominant auto-replies), `rejection` ~30, `interview_invitation` ~28, `assessment` ~6, plus ~9 non-application emails (sign-in codes). Interview invitations are ~1 in 8 — enough to justify the roadmap follow-on.

The project already has the right idiom for this shape of work: transactional-outbox queues (`enrichment_outbox`, `semantic_outbox`, `telegram_posts`) drained by run-once-and-exit cron workers over the shared `internal/llm` client, with a `Sanitize`-to-vocabulary persist guard. This change reuses that idiom rather than introducing new infrastructure.

## Goals / Non-Goals

**Goals:**
- Automatically resolve each inbox email to at most one of the caller's own applications and classify its status, using deterministic matching first and the LLM only for the tail.
- Keep the user in control: auto-link only at high confidence (undoable), suggest otherwise, and advance application stage only forward at high confidence.
- Surface the linkage both ways: linked emails on a new application detail page, and an application link + confirmation chip in the inbox reading pane.
- Fit the existing outbox+cron+`systemd`-timer machinery with zero new infrastructure.

**Non-Goals:**
- The interview-prep durable workflow (extract date → durable wait → company brief → send email). It is documented below as the follow-on sub-project and is where a durable-workflow engine (Inngest/Temporal) is evaluated. This change deliberately stops at classification + linking, which needs no such engine.
- Live company web-research (reuses `company_info` only, when B is built).
- Any change to the Gmail/hosted ingest crawl itself beyond the enqueue-on-insert hook.
- Lifting the moderator-only gate.

## Decisions

**Reuse the enrichment outbox idiom instead of a workflow engine.** The task is stateless, fast, low-volume (one LLM call per email, ~237 total), and needs at-least-once with retry + dead-letter — exactly what `enrichment_outbox` already provides. Alternative considered: adopt a durable-workflow engine now. Rejected for this change — its value (durable timers, waits, sagas) does not apply to a single classification step, and it would break the cron run-once-and-exit deploy model on host2. The engine question is deferred to the follow-on (B), where durable waits genuinely apply.

**Match against the caller's own applications, not the global company directory.** The target set is the handful of companies the caller applied to (`user_jobs → jobs → company`), not all of `companies`. This is more accurate, cheaper, and unit-testable. The cascade: (1) thread continuity; (2) company name extracted from `from_name`/`subject`, normalized (strip "Hiring Team", "- Workday", "LLC"; drop ATS-platform pseudo-names like "Greenhouse"/"Workday") and fuzzy-matched to the caller's tracked companies; (3) LLM disambiguation of the zero/multi-candidate tail; (4) unlinked. `internal/mailmatch` holds the deterministic tiers as pure functions; the LLM tier is a thin adapter. Alternative considered: LLM-only matching. Rejected — wasteful and untestable when a normalized name match is exact.

**Split deterministic matching from probabilistic classification.** `internal/mailmatch` (deterministic, unit-tested with fakes) is separated from `internal/mailclassify` (the LLM status contract + `Sanitize`), mirroring the `internal/location` (dictionary) vs `internal/enrich` (LLM) ACL split. Each is testable in isolation.

**Store hypothesis and truth separately.** `emails.job_id` is the accepted link (auto or user-confirmed); `emails.suggested_job_id` is a pending hypothesis; `link_source` (`auto`|`manual`) records provenance so an auto-link stays visibly undoable. This mirrors the `enrichment` (accepted) vs raw-LLM-output (raw) separation and yields a free calibration signal from user confirm/reject. Alternative considered: a single nullable `job_id` with a boolean "auto". Rejected — loses the pending-suggestion state the inline chip needs.

**Confidence gates both linking and stage advancement.** One match output, two thresholds: above the auto-link threshold → set `job_id`; below → set `suggested_job_id`. Stage advancement is monotonic-forward-only at high confidence (never backward, never `rejection`→closed automatically), else a suggestion. Server owns the vocabulary→stage mapping and clamps model output — the same "never persist an out-of-vocabulary value" invariant and prompt-injection guard as enrich.

**Sanitize is the persist and injection guard.** Email bodies are untrusted input to the prompt. `mailclassify.Sanitize` coerces any out-of-vocabulary status to `other` and bounds text before persistence, so a crafted body cannot inject an out-of-vocabulary value or unbounded content.

## Risks / Trade-offs

- **Dirty company-name extraction** (ATS pseudo-names, localized subjects, "Hiring Team" suffixes) → normalize deterministically and fall through to the LLM; leave unresolved emails unlinked (a valid state) rather than mis-linking.
- **Misclassification advancing a stage wrongly** → auto-advance only forward and only above threshold; `rejection` never auto-applies; every auto action is undoable and shows `link_source=auto`.
- **Prompt injection via email body** → `Sanitize` to the controlled vocabulary and bounded text, identical to the enrich guard.
- **Budget/latency of per-email LLM calls** → deterministic tiers resolve the majority (thread + exact name match) with no LLM; only the tail hits the model, under the per-call timeout used elsewhere.
- **LLM unconfigured/failing** → best-effort: deterministic matches still apply, emails stay viewable, unresolved emails remain unlinked, no inbox error.
- **New long-lived worker on host2** → none; `cmd/classify-mail` is a run-once-and-exit cron worker on a `systemd` timer, same as the other drainers.

## Migration Plan

1. Add migration ~`0017`: new `emails` columns (`job_id`, `suggested_job_id`, `link_source`, `match_confidence`, `status_signal`, `classified_at`, `classification_model`) and the `email_classification_outbox` table. Apply to prod manually as role `hire` (live-table migration) before deploy, per the migrations convention.
2. Ship `internal/mailmatch`, `internal/mailclassify`, `cmd/classify-mail`, the enqueue-on-insert hook, the `/me/tracking/:slug` and email-link endpoints, and the SPA changes.
3. Backfill: run `cmd/classify-mail` once to classify the existing 237 emails (idempotent — re-running is safe).
4. Rollback: the feature is additive and gated; unlinking is a no-op path and the columns default null. Reverting the binary leaves the columns inert.

## Open Questions

- Exact auto-link and stage-advance confidence thresholds — to be tuned against the 237-email set during implementation; start conservative (favor suggestions over silent auto-actions).
- Whether the confirm/reject signal is persisted as calibration training data now or just applied — default: apply only, note the seam.

## Roadmap: interview-prep durable workflow (follow-on B, not in this change)

Once linking + classification land, the natural next sub-project is: `interview_invitation` → extract interview datetime → **durable wait** until `interview_at − lead` → gather `company_info` + LLM prep brief (reusing `jobfit.Input`: CV, structured resume, company info) → send the brief to the caller via SES (`mail.freehire.dev` + `notify.Router`) → optional post-interview follow-up. That workflow is stateful with a multi-day durable timer and human-relevant timing — the regime where a durable-workflow engine earns its keep. B is where Inngest and Temporal are spiked against a hand-rolled `fire_at`+cron baseline on four axes (Go SDK DX, durability/retries, observability, host2 fit), producing a VALIDATED/PARTIAL/INVALIDATED verdict. It gets its own proposal → design → tasks cycle.
