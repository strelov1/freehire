## Context

Gmail synchronization finds incoming hiring-shaped messages, expands their threads, then discards self-authored siblings. Its timestamp cursor cannot discover later outgoing replies without an incoming anchor.

## Goals / Non-Goals

Capture outgoing Gmail messages in known hiring threads, including older threads and aliases, and expose direction and recipients. Do not import unrelated sent message bodies, send email, add model calls, implement the digest, or change hosted email transport.

## Decisions

Use Gmail's SENT label as the authoritative direction, retaining a primary-address fallback for existing readers. Persist direction (incoming/outgoing) and To/Cc headers. Existing messages default to incoming. Outgoing mail is stored read and excluded from classification queues, employer events, stage writes and invitation selection.

Add a separate sent synchronization cursor to gmail_connections. At its initial zero value expand the 50 newest stored Gmail threads once to backfill their sent replies; older threads keep no pre-feature replies, because an unbounded first pass over every mailbox was rate-limited by Google and read as revoked grants (#4610). A rate-limit answer (429, or 403 with a limit/quota reason) stops the run without touching the grant. On later runs page through sent message identifiers and thread identifiers since the previous successful run, fetching bodies only for known hiring threads. Use run-start time with a short overlap so an in-flight send or same-second timestamp is retried safely. Preserve both cursors after transient failures; revoked-grant handling stays shared. Upserts remain idempotent and refresh provider direction/recipient metadata on conflict. A previously misidentified alias reply loses its employer classification, pending work and live employer-reply event when recognized as outgoing. Historical stage choices are not reversed automatically. Permanently missing messages/threads are skipped; drafts are never stored. Incoming watermark advances only on incoming messages.

Expose direction and recipients in inbox list/detail and display Sent with recipients in the web inbox. Preserve incoming defaults for all other sources. Outgoing messages do not receive employer-status actions.

Application mail views derive an unattached outgoing reply's membership from its Gmail conversation at read time, so an incoming message linked after synchronization also reveals earlier replies. Infer only within the same user, source and nonempty thread, with one unambiguous linked incoming job. Conflicting jobs and application-only incoming links prevent inference; explicit manual and agent attachments take precedence. This projection persists no link or employer activity. Recall selection and suggestion writes accept incoming mail only, including when alias repair occurs during recall. Follow-up drafts choose the newest incoming sender rather than the candidate's own later reply.

## Risks / Trade-offs

- First backfill traverses the 50 newest existing threads once; later passes read sent metadata and fetch only relevant content.
- A missing SENT label is handled for the primary connected address; Gmail labels identify aliases.
- Read-only synchronization cannot prove what a candidate communicated outside this mailbox or in another thread.

## Migration Plan

Apply the additive migration before deploying binaries. Old binaries ignore the new columns; rollback binaries safely without dropping data. No production deployment is part of this change.
