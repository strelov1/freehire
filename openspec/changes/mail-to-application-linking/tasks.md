## 1. Schema and generated DB access

- [x] 1.1 Add migration `0017_email_application_linking.sql`: new `emails` columns (`job_id` BIGINT NULL REFERENCES jobs(id), `suggested_job_id` BIGINT NULL REFERENCES jobs(id), `link_source` TEXT NULL, `match_confidence` REAL NULL, `status_signal` TEXT NULL, `classified_at` TIMESTAMPTZ NULL, `classification_model` TEXT NULL) + `email_classification_outbox` table (email_id, lease/retry bookkeeping) mirroring `enrichment_outbox`
- [x] 1.2 Add queries in `internal/db/queries/`: enqueue-on-insert, claim/lease a wave, set classification result + delete outbox row, list emails by application, confirm/reject/manual-link mutations
- [x] 1.3 Run `make sqlc` and commit generated `internal/db` changes

## 2. Deterministic matching (`internal/mailmatch`)

- [x] 2.1 Company-name extraction + normalization from `from_name`/`subject` (strip "Hiring Team"/"- Workday"/"LLC", drop ATS pseudo-names like "Greenhouse"/"Workday")
- [x] 2.2 Match cascade tiers 1–2 as pure functions: thread continuity, then normalized-name fuzzy match against the caller's open applications; return best candidate + confidence
- [x] 2.3 Unit tests with fakes over the 237-email patterns (ATS pseudo-name is not matched; single vs multi candidate; unlinked is valid)

## 3. LLM classification and disambiguation (`internal/mailclassify`)

- [x] 3.1 Status contract: the controlled vocabulary type + `Sanitize` (coerce out-of-vocabulary → `other`, bound text) as the persist and prompt-injection guard
- [x] 3.2 LLM prompt over the email body via `internal/llm`: return status signal (+ the disambiguation tier: pick one of the caller's applications or "none")
- [x] 3.3 Unit tests for `Sanitize` (out-of-vocabulary coercion, bounding) and the vocabulary→stage mapping

## 4. Classification worker (`cmd/classify-mail`)

- [x] 4.1 Idempotent enqueue-pending sweep keyed on `classified_at` (replaces the insert-time hook — no coupling into the two mail write paths)
- [x] 4.2 Worker (`internal/maillink` Runner + `cmd/classify-mail`): claim a wave with lease, run match → classify, apply confidence tiers (auto-link vs suggestion), apply monotonic-forward stage advancement (high confidence only, never backward, never auto-`rejection`), persist + advance-stage + delete outbox row in one txn; retry-then-dead-letter
- [ ] 4.3 `systemd` timer wiring (freehire-ops) + one-off prod backfill of the existing 237 emails — deferred to deploy (ops + prod mutation, needs sign-off)

## 5. API

- [x] 5.1 `GET /api/v1/me/tracking/:slug` — application detail with linked emails (status_signal, sender, subject, received_at), moderator-gated, 404 on untracked slug
- [x] 5.2 Email link endpoints under `/me/emails`: confirm suggestion (→ `job_id`, `link_source=manual`), reject suggestion (clear), manual link/unlink to a chosen application
- [x] 5.3 Extend the inbox email read shape with `job_id`/`suggested_job_id`/`status_signal`/`link_source`

## 6. Frontend

- [x] 6.1 New route `/my/tracking/[slug]`: job + interaction + linked emails with per-email status badges
- [x] 6.2 Inbox reading pane: inline link-confirmation chip for a suggested match (confirm/not-this), application link when linked, manual-link affordance when unlinked
- [x] 6.3 Link from an inbox email to its application detail page

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./... && go test ./...`; `gofmt`
- [x] 7.2 Ran on the real 237-email set on prod: distribution sane (ack 146/reject 39/other 35/interview 8/…), 40 auto-linked + 48 LLM-suggested, 0 out-of-vocabulary values. Caught+fixed a terminal-stage resurrection bug (9 rejected→applied), reverted from backup
- [ ] 7.3 Manual QA on `/my/inbox` (confirm/reject a suggestion, manual link) and `/my/tracking/[slug]` (linked emails render with badges); confirm rejection never auto-advances a stage
