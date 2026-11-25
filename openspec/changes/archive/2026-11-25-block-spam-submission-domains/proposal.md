## Why

The public job-submission form (`POST /api/v1/submissions`) accepts any well-formed
`http(s)` URL with no check on the host. It is being used to plant referral-link spam:
a submitted URL points at a throwaway wrapper domain (e.g. `gridnaut.site`) that
redirects to a real ATS listing (e.g. `work.mercor.com`) carrying the spammer's own
`referralCode`/`utm_campaign` query string. A moderator currently has no way to tell
this apart from a legitimate submission except opening the link, and rejecting one
occurrence does nothing to stop the same domain from being resubmitted the next day.

## What Changes

- Add a submission-domain blocklist: a small table of banned hosts, each recording who
  blocked it and why.
- `submission.Service.Submit` checks the submitted URL's host against the blocklist and
  refuses the submission (no row is written) when it matches, instead of queuing it for
  a moderator to discover. This check applies only to the public submission path — a
  moderator's own hand-authored vacancy (`moderation.Service.Create` called directly,
  not through `Submit`) is never blocked, since that caller is already trusted.
- Extend `POST /api/v1/submissions/:id/reject` with an optional `block_domain` flag: when
  set, rejecting a submission also adds its URL's host to the blocklist in the same
  action, so a moderator handles a recurring spam domain without a follow-up step or a
  code change.
- The blocklist only stops *future* submissions, so `block_domain` also rejects every
  other still-`pending` submission whose URL host matches, in the same action — a
  spammer who bulk-submitted dozens of postings from one host is cleared in one click
  instead of one row at a time.
- Host matching is exact (case-insensitive, a leading `www.` ignored on both sides) —
  no wildcard/suffix matching, kept for a later change if spammers rotate subdomains.

## Capabilities

### Modified Capabilities

- `job-submission`: adds a blocklist check to the submit path and a `block_domain` option
  to the reject action.

## Impact

- `internal/ingest/submission`: `Submit` gains a blocklist lookup; `Reject` gains the
  `block_domain` parameter, an insert into the blocklist, and a bulk-reject of every
  other pending submission sharing the blocked host.
- `internal/ingest/moderation`: unaffected — the blocklist is not part of
  `CreateInput.Validate`, so it never applies to moderator-authored creates.
- `internal/api/handler/submissions.go`: `RejectSubmission` reads the new
  `block_domain` field from the request body.
- New migration: a `submission_domain_blocklist` table (host, blocked_by, reason,
  created_at).
- `web`: the moderation queue's reject control gains a "block this domain" checkbox.
