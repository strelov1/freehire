## ADDED Requirements

### Requirement: Classification pipeline is queue-driven

The system SHALL enqueue every not-yet-classified inbox email into an `email_classification_outbox` queue and a run-once-and-exit worker (`cmd/classify-mail`) SHALL drain that queue, mirroring the enrichment outbox idiom (reference-only rows, lease + retry, dead-letter after a bounded number of attempts). Enqueue is an idempotent pending sweep keyed on the `classified_at` stamp, so re-running never duplicates work.

#### Scenario: Unclassified emails are enqueued

- **WHEN** the classify-mail worker runs its enqueue sweep
- **THEN** every email without a `classified_at` stamp gets exactly one `email_classification_outbox` entry, and already-classified or already-queued emails are left untouched

#### Scenario: Worker drains the queue

- **WHEN** `cmd/classify-mail` runs and a pending outbox row exists for an email
- **THEN** the worker resolves the email's application match and status signal, persists the result, and deletes the outbox row in one transaction

#### Scenario: Transient failure retries then dead-letters

- **WHEN** classifying an email fails transiently
- **THEN** the row is retried up to the configured maximum and dead-lettered afterward, without blocking other emails

### Requirement: Email-to-application match cascade

The system SHALL resolve an email to at most one of the caller's own open applications using a deterministic-first cascade: (1) thread continuity, (2) company name extracted from `from_name`/`subject`, then (3) LLM disambiguation of the remainder, falling back to unlinked.

#### Scenario: Thread continuity wins

- **WHEN** an email shares a `thread_id` with an email already linked to an application
- **THEN** the email is matched to that same application without further steps

#### Scenario: Company name matches a single tracked application

- **WHEN** the company name extracted and normalized from `from_name`/`subject` matches exactly one of the caller's open applications' companies
- **THEN** the email is matched to that application

#### Scenario: Ambiguous or missing deterministic match falls to the LLM

- **WHEN** deterministic matching yields zero or more than one candidate
- **THEN** the LLM is given the email and the caller's open applications and selects one application or "none"

#### Scenario: No candidate leaves the email unlinked

- **WHEN** no application can be resolved
- **THEN** the email is left unlinked, which is a valid state, and remains available for manual linking

### Requirement: Sender-domain matching against the company directory is not used

The system SHALL NOT match an email to an application by comparing the sender address domain against `companies.domains`, because inbox sender domains are ATS relay domains rather than employer domains.

#### Scenario: ATS relay domain does not drive a match

- **WHEN** an email arrives from an ATS relay domain such as `ashbyhq.com` or `us.greenhouse-mail.io`
- **THEN** the match cascade ignores the sender domain and relies on thread, extracted company name, and the LLM instead

### Requirement: Status classification into a controlled vocabulary

The system SHALL classify each email into exactly one status signal from the controlled vocabulary `acknowledgement`, `screening`, `interview_invitation`, `assessment`, `offer`, `rejection`, `info_request`, `other`, and SHALL sanitize any out-of-vocabulary model output to `other` before persistence.

#### Scenario: In-vocabulary signal is persisted

- **WHEN** the model returns a recognized status such as `interview_invitation`
- **THEN** that status signal is persisted on the email with the model stamp and classification timestamp

#### Scenario: Out-of-vocabulary output is coerced

- **WHEN** the model returns a status outside the controlled vocabulary
- **THEN** the value is coerced to `other` and no out-of-vocabulary value is ever persisted

#### Scenario: Non-application email is classified as other

- **WHEN** an email is unrelated to a job application (for example a sign-in code)
- **THEN** it is classified as `other` and not force-linked to any application

### Requirement: Confidence-tiered linking

The system SHALL decide linking by match confidence: a high-confidence match auto-links (`job_id` set, `link_source=auto`), a lower-confidence match is stored as a suggestion (`suggested_job_id` set, `job_id` null), and no candidate leaves both null.

#### Scenario: High confidence auto-links

- **WHEN** the resolved match confidence is at or above the auto-link threshold
- **THEN** `job_id` is set with `link_source=auto` and the email displays as linked

#### Scenario: Lower confidence is stored as a suggestion

- **WHEN** the resolved match confidence is below the auto-link threshold but a candidate exists
- **THEN** `suggested_job_id` is set, `job_id` stays null, and the email awaits user confirmation

### Requirement: Inline suggestion confirmation and manual linking

The system SHALL let the caller confirm or reject a suggested link and manually link or unlink an email to any of their applications, and confirming a suggestion SHALL set `job_id` with `link_source=manual`.

#### Scenario: Confirming a suggestion promotes it

- **WHEN** the caller confirms a suggested link on an email
- **THEN** `job_id` is set to the suggested application with `link_source=manual` and `suggested_job_id` is cleared

#### Scenario: Rejecting a suggestion clears it

- **WHEN** the caller rejects a suggested link
- **THEN** `suggested_job_id` is cleared and the email is left unlinked

#### Scenario: Manual link overrides an auto-link

- **WHEN** the caller manually links an email to a different application
- **THEN** `job_id` is set to the chosen application with `link_source=manual`

### Requirement: Monotonic-forward stage advancement

The system SHALL advance a linked application's `stage` from a classified email only forward in the pipeline order and only at high confidence; otherwise it SHALL surface a stage-change suggestion without mutating the stage, and it SHALL never move a stage backward automatically.

#### Scenario: High-confidence forward signal advances the stage

- **WHEN** a high-confidence `interview_invitation` email links to an application currently at an earlier stage
- **THEN** the application stage advances to the corresponding forward stage

#### Scenario: Backward or low-confidence signal does not auto-move

- **WHEN** a classified email would move the stage backward, or its confidence is below threshold
- **THEN** the stage is left unchanged and a suggestion is surfaced for the caller to apply

#### Scenario: Rejection never silently closes the pipeline

- **WHEN** an email is classified as `rejection`
- **THEN** the application stage is not auto-set to a rejected/closed state; the change is only ever applied by the caller

### Requirement: Best-effort degradation without an LLM

The system SHALL treat classification and LLM disambiguation as best-effort: when the LLM is unconfigured or failing, deterministic matching still runs, emails remain viewable, and no error is surfaced to the inbox.

#### Scenario: Unconfigured LLM leaves the inbox usable

- **WHEN** the LLM is not configured
- **THEN** emails are still stored and displayed, deterministic matches still apply, and unresolved emails simply remain unlinked
