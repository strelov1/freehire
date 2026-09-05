# Auto-apply queue-drain conventions

## Scope
The domain logic behind `cmd/auto-apply`: claim a wave of `auto_apply_queue` rows, assemble
each candidate's known answers, ask a browser driver to resolve and maybe submit the
application, and record the outcome. Pgx/Fiber-free — `Store`, `AnswerSource` and
`SidecarClient` are ports; `cmd/auto-apply` supplies the real implementations
(`dbStore`, `assemblerAnswerSource`, `internal/atsapply.Client`).

## Always true
- **Submits only when every required question is answered.** `SidecarClient.Submit` returns
  `StatusApplied` or `StatusParked` (with the unmapped reasons); there is no partial-fill
  outcome. A required field with no known answer parks the whole attempt — nothing here ever
  guesses.
- **`Park` is not a retry.** A parked attempt needs new data, not another try — it is
  excluded from reclaim (`blocked_at`) rather than counted against `MaxAttempts`. Only a
  genuine transient failure (`Fail`) spends the retry budget.
- **Two outcomes are dead-lettered immediately rather than retried normally, both through
  the shared `deadLetterImmediately` (`Fail(..., maxAttempts=1)`):** a real submission that
  fails to record locally (`recordApplied`, when `Store.Submit` errors after the sidecar
  already reported `StatusApplied`), and an unconfirmed submission (`StatusUnconfirmed` —
  the sidecar pressed submit but could not tell whether the employer accepted it). Both
  share the same reasoning: the ordinary retry path would eventually re-arm the row for
  reclaim and risk calling the browser driver again for a job that may already have been
  applied to, which the "never submit twice" invariant forbids outright. The second case was
  found by code review, not the original design — an earlier version mapped an unconfirmed
  result to a plain `error`, which took the ordinary (retryable) path.
- **`SidecarClient.Submit` takes the whole `Claimed`, not its individual fields** — mirroring
  `internal/applyform.Fetcher.Fetch`'s own reasoning: what a submission needs is not the same
  for every provider (Greenhouse/Ashby need `ExternalID`, not just `JobURL`), so the seam
  should not grow a parameter every time a provider needs one more piece of the claim.
- **`AnswerSource` supplies identity/work-authorization facts only (Tier A/B).** A question
  outside that set parks unless `internal/atsapply`'s own drafting fallback answers it (see
  its AGENTS.md) — `AnswerSource` itself is unaware of drafting either way. The real
  implementation (`cmd/auto-apply`'s `assemblerAnswerSource`) wraps
  `internal/candidateprofile.Assembler`, the same one the browser extension's autofill path
  reads, so a value a person sees in a form and a value this worker resolves against can
  never diverge.
- **`process` always assembles answers before calling `Submit`, even for a row that
  `SidecarClient` will immediately park (Lever's captcha, e.g.) without ever touching them.**
  A known, accepted inefficiency, not an oversight: `answers` is `Submit`'s argument (not a
  lazy source `SidecarClient` could pull from only if it turns out to need them), and
  `process` has no ATS-specific knowledge — deliberately, per `SidecarClient`'s own doc
  comment — so it cannot know in advance which rows will discard the answers unused. Fixing
  this would mean either leaking provider/captcha knowledge into this generic queue-drain
  layer, or reworking `SidecarClient.Submit` to pull answers lazily — both costlier than the
  handful of avoidable DB reads per already-doomed-to-park row that this saves.

- **`Store.Claim` only ever returns a reviewed-and-approved entry.** `Claimed.TailoredCVID`
  is the candidate's approved tailored CV for the vacancy — never the zero value for a row
  this package sees, because the real `Claim` (`ClaimAutoApplyBatch`) requires
  `tailored_cv_id IS NOT NULL AND review_decision = 'approved'` (openspec/changes/
  auto-apply-tailored-resume). An entry with no tailored CV yet, or one the candidate has
  not reviewed, simply sits in the queue unclaimed; a decline parks it the same way an
  unresolved form field does, through its own review endpoint (`internal/api/handler`), not
  through anything in this package.

## How it works
`Run` wires `outbox.RunPool` over `Store.Claim`, mirroring `internal/applyform`'s own
`cmd/capture-apply-form` runner shape. `process` per claimed item: assemble answers → call
`SidecarClient.Submit` → map the result to `Store.Submit` (success; composes
`LockJobForApply` + `MarkJobApplied` + queue retirement in one transaction, in
`cmd/auto-apply/store.go`) / `Store.Park` (unresolved) / `Store.Fail` (transient error).
`RunStats.Degraded` treats a parked attempt as healthy (the system correctly declined to
guess, not a fault) — only a dead letter or a run that failed everything counts.

What populates `auto_apply_queue` is out of scope for this package entirely — see
`openspec/changes/auto-apply-worker/design.md`. `Run` only ever claims what is already
there.
