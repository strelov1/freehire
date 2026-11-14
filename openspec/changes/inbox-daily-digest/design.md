## Context

The existing notify worker runs regularly and already owns SES setup, mail pause handling and unsubscribe links. Inbox data includes direction and Gmail thread identifiers. Notification settings own timezone-aware delivery preferences.

## Goals / Non-Goals

Send one useful opt-in daily email from already classified mail, without a new scheduler, model call, CV ranking, automated reply, or production deployment.

## Decisions

Add inbox_digest_enabled (default false) and its activation timestamp to notification_settings. Use the existing daily digest_time, even when search alerts use another cadence. Preserve the new setting when an older client omits it. Global notifications, selected email channel, activity-group consent and the standard mail_recipient eligibility all gate delivery.

Select incoming, classified non-other messages received since activation and within seven days; exclude deleted, future-dated, outgoing and already delivered messages. Group Gmail threads by source and provider thread id, with a per-message fallback for sources without one. Show the newest message per conversation, its source excerpt when available and a link to that exact inbox message. A later stored outgoing message is evidence of a subsequent reply, not proof that every request was resolved. Render three sections: recruiter outreach, requests in messages, application updates. Do not claim CV relevance or that a person has not answered.

Cap email contents at ten conversations, count overflow and link to the inbox. Retire all covered eligible messages on successful delivery so overflow cannot build an obsolete backlog. Keep source ids, not mail body snapshots, in the delivery ledger. Suppress late-classified older messages if a newer message in the same thread has already been delivered. Generated summaries carry an explicit mailmeta marker and Auto-Submitted header on SES and SMTP/raw paths; Gmail and hosted ingest skip the exact marker pair before persistence to prevent feedback loops.

Claim a unique (user, local calendar day) row before dispatch, rechecking consent. Empty snapshots claim nothing. After success mark sent; a failed database acknowledgement leaves the claim in place. Keep ambiguous delivery failures claimed for the day to avoid an immediate duplicate; messages not confirmed delivered become eligible on a later day. A known pre-dispatch SES pause releases its claim. A failed send is reported and does not stop other recipients. Select only due, eligible users before the per-run cap so non-due/empty accounts cannot starve others.

## Risks / Trade-offs

Classifier mistakes remain visible as detected labels, as in the inbox; this digest makes no stronger factual claims. A crash after send but before recording can repeat covered messages on a later day; SES has no idempotent send key. Conservative day claims prevent same-day replay after an ambiguous result. Body snippets may be empty for HTML-only mail; subjects and source links remain useful.

## Migration Plan

Apply additive migration 0257 after 0256 and before deploying readers. Consent remains off for every existing account. Old binaries ignore the new columns. Deploy the Gmail reader guard and restart the existing long-lived mail-ingest daemon before enabling the new notify behavior. Reuse the existing notify service/timer; no host configuration change is needed.
