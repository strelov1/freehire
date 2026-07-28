## Why

The mail stack produces link suggestions that nothing can consume. On a real
account, 74 classified emails carry a pending `suggested_job_id` and 31 emails
carrying progress signals (`interview_invitation`, `screening`, `assessment`,
`offer`) are attached to no application at all — in every one of those cases the
employer is present in the catalog, but no tracked application exists to attach
the mail to.

Two things cause it, and neither is the matcher:

- The inbox listing cannot be filtered by link state. It filters by source,
  unread, classification label, and search, so "show me the mail awaiting my
  confirmation" cannot be asked for — a caller must page the whole mailbox and
  sort it client-side.
- Mail that belongs to an application the user never recorded has no path
  forward. Linking requires an existing tracked application, so the only route is
  to search the catalog, mark the job applied by hand, and then link — three
  round-trips, with the application's date silently becoming "now".

The consequence is that measured link coverage understates what the matcher can
actually do, and any analysis built on `emails.job_id` inherits that
understatement.

## What Changes

- The inbox listing accepts a **link-state filter** — `linked`, `suggested`,
  `unlinked` — composing with the existing source, unread, label, and search
  filters and reflected in the pagination total. `suggested` is the confirmation
  queue; `unlinked` is the queue for mail with nowhere to go yet.
- A new action **creates a tracked application from an email and links the email
  to it in one step**, given the email and the public slug of a catalog job. The
  new application's `applied_at` is taken from the email's `received_at` rather
  than the current time, because the application demonstrably existed by the time
  the employer replied.
- Creating an application this way counts as an application everywhere an
  ordinary apply does — it is the same act, recorded late.

Not in this change: the `freehire-cli` commands that consume these endpoints
(`inbox confirm`, `inbox reject`, the link-state filter flag, and the
create-and-link action) live in the sibling `freehire-cli` repository and are
tracked there. The API is the prerequisite.

Also not in this change: silence and ghosting analysis, parked as
`application-ghosting-signal` until link coverage is measured again on labelled
data.

## Capabilities

### New Capabilities
- `application-from-mail`: recording a tracked application directly from a piece
  of employer mail, dated by that mail, and linking the two in a single action.

### Modified Capabilities
- `agent-inbox-surface`: the agent-shaped listing gains the link-state filter, so
  the confirmation queue and the orphaned-mail queue are addressable the way the
  unclassified work queue already is.
- `email-inbox`: mark-all-read honours the link state alongside the filters it
  already honours — a filter that narrows the view but not the action would mark a
  whole mailbox read on a caller who meant to clear a queue of three.

## Impact

- `internal/handler/inbox.go` — one new query parameter on the listing, validated
  against a closed vocabulary (an unknown value is a `400`, matching how the
  label filter already behaves).
- `internal/db/queries` — the listing query and its count gain the link-state
  predicate; both must move together or pagination lies.
- `internal/handler/inbox_linking.go` — the create-and-link action, alongside the
  existing link/unlink/confirm/reject.
- `internal/jobtracking` — creating an application at a supplied timestamp,
  reusing the existing apply path and its `applied_count` locking rather than
  writing a second insert.
- No migration: `emails.job_id` and `emails.suggested_job_id` already exist, and
  `emails_job_id_idx` covers the linked case.
- **`freehire-cli`** (sibling repo) — the consuming commands, out of scope here.
