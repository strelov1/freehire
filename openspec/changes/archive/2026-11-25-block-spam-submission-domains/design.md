## Context

See proposal.md - Why. Relevant current state:

- `submission.Service.Submit` (`internal/ingest/submission/submission.go:112`) validates via
  `moderation.CreateInput.Validate` (`internal/ingest/moderation/moderation.go:75`) and then
  calls `Repository.Create`. `Validate` is shared with `moderation.Service.Create`
  (moderator-authored vacancies), so it is the wrong place for a submitter-only check.
- `submission.Service.Reject` (`internal/ingest/submission/submission.go:172`) loads the
  submission, checks it is `pending`, and calls `Repository.MarkRejected`.
- The moderation queue handler is `internal/api/handler/submissions.go`; the frontend is
  `web/src/lib/components/ModerationView.svelte` (queue tab).
- A one-off incident already produced 215 pending `gridnaut.site` rows, rejected by hand
  through the existing per-id reject endpoint before this change ships — this change is
  about the next occurrence, not a backfill of this one.

## Goals / Non-Goals

**Goals:**
- Refuse a submission from a known-bad host before it ever reaches the moderator queue.
- Let a moderator turn "reject this one" into "reject this one and every other pending
  submission from the same host, and block the host" in a single action.
- Never affect `moderation.Service.Create` (the trusted, moderator-authored path).

**Non-Goals:**
- No wildcard/suffix/registrable-domain matching (e.g. blocking `*.gridnaut.site` or
  catching a same-registrant domain under a different TLD). Exact-host matching (modulo a
  `www.` prefix) is enough for the observed pattern and keeps the check a single indexed
  lookup with no false-positive risk. Revisit if spammers start rotating subdomains.
- No retroactive scan of already-`approved` jobs. A submission is blocked before minting;
  a job already minted from a since-blocked host is not touched by this change.
- No general-purpose "content moderation" or spam-scoring system — this is a host
  denylist, nothing more.

## Decisions

**Blocklist storage: a new table, not a config list.** The proposal requires a moderator
to add an entry from the UI without a deploy, which rules out a static Go slice or an env
var. A table also gives natural attribution (`blocked_by`, `reason`, `created_at`) for free.

**Checked only in `submission.Service.Submit`, not in `moderation.CreateInput.Validate`.**
`Validate` is shared with moderator-authored creates; putting the check there would block a
moderator from manually re-adding a job whose original host happens to be on the list
(implausible in practice, but the layering is also just wrong — the blocklist is a
"who may I trust" decision about the *submitter*, not a property of the URL itself).

**Folded into the existing `submission.Repository` interface, not a separate
`HostBlocklist` port.** `Repository` gains `IsHostBlocked` and `RejectAndBlockHost` rather
than `Service` taking a third dependency. `RejectAndBlockHost` needs one Postgres
transaction spanning `submission_domain_blocklist` and `job_submissions` (the next
decision), so a separate port would still be implemented by the same adapter to get that
atomicity — it would document an isolation the code does not actually have. `Submit` and
`Reject` call `repo.IsHostBlocked`/`repo.RejectAndBlockHost` directly.

**Host normalization: lowercase, strip one leading `www.`.** Matches the reported case
exactly and is simple enough to unit test exhaustively. `net/url.Parse(in.URL).Host` already
strips scheme/path/query; a port suffix (rare for a spam wrapper) is left in place, so
`gridnaut.site:8080` would not match a stored `gridnaut.site` entry — acceptable, since
adding it would need its own normalization decision this change doesn't need to make yet
(no observed case has a port).

**`block_domain` bulk-rejects same-host pending rows in the same DB transaction as the
target reject and the blocklist insert.** Alternatives considered:
- *Separate "bulk reject by host" endpoint, called by the frontend after the single
  reject succeeds.* Rejected: two round trips, and a failure between them leaves the
  blocklist entry live while sibling spam rows are still sitting in the queue — exactly
  the inconsistent state this change exists to avoid.
- *Fire-and-forget background job.* Rejected: needless async complexity for an operation
  bounded by "however many rows one spammer submitted," which is small (this incident:
  215) and fast as a single `UPDATE ... WHERE status='pending' AND host = $1`.

**Blocklist insert is `ON CONFLICT (host) DO NOTHING`.** Matches the spec's
already-blocked-is-a-no-op requirement without a pre-check-then-insert race.

**No new logging/metrics.** A refused submission returns `403` synchronously to the
submitter; that is the whole feedback loop. `job_submissions` already lets an operator see
history via existing tooling, and the blocklist table itself is the record of what was
blocked and by whom — a metric would count something nobody currently needs paged on.

## Risks / Trade-offs

- [A legitimate site is blocked by mistake] → the blocklist is moderator-only,
  attributed, and `403`-visible to the submitter (not a silent drop), so a wrong entry
  surfaces quickly as a support question; removing it is a plain `DELETE` (no UI for
  un-blocking in this change — small enough backlog to do by hand, add a UI later if it
  comes up).
- [Exact-host matching is trivially evaded by a new subdomain or domain] → accepted
  per Non-Goals; the `block_domain` bulk-reject at least clears a whole incident in one
  action even though prevention is per-exact-host.
- [The bulk-reject `UPDATE` could be large for a bigger future incident] → bounded by
  "one spammer's pending backlog," which the existing 500-row queue cap already treats as
  the outer limit for this whole feature area; no separate cap needed.

## Migration Plan

- One migration adds `submission_domain_blocklist` (host, blocked_by, reason,
  created_at) with a unique index on `host`.
- No backfill: the table starts empty; this change does not need the 215 already-rejected
  `gridnaut.site` rows to be represented in it (nothing re-checks history), though a
  moderator will naturally add `gridnaut.site` on the next occurrence via `block_domain`.
- Rollout is a normal deploy; no feature flag needed — the check is additive (a host with
  no blocklist entry behaves exactly as today) and the new request field is optional.
- Rollback: revert the deploy; the migration is additive and safe to leave in place even
  if the code is rolled back.
