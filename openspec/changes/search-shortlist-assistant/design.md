## Context

The product scope and written design were approved by the user. The canonical design is [the approved design](../../../docs/superpowers/specs/2026-11-02-search-shortlist-assistant-design.md).

## Decisions

Reuse the chat preset with a durable optional search binding. Preserve the original query independently of transcript history, and persist a bounded selection brief. Share request-independent search assembly between the listing and scoped tools. Scoped presentation accepts only returned candidates and at most five jobs. Reuse the existing dock and cards; authenticate through the current sign-in flow.

Page filter changes never rebind an existing conversation. Start a new conversation explicitly. Unknown or degraded constraints stop shortlisting instead of silently widening it. Existing assistant metering, ownership checks, turn bounds, retry, and cancellation apply.

## Risks

Listing-specific personalized filters and search degradation must survive the helper extraction. Resumption must be account-scoped. Model-authored selection notes remain untrusted explanations, never candidate facts or authorization.

## Migration and verification

Apply the additive session-context migration before deploying readers. Generate SQL through make sqlc. Verify parity, ownership, immutable scope, selection-brief persistence, candidate presentation, panel lifecycle, and existing search regressions, then run required Go/frontend checks and a browser walkthrough. See the approved design for acceptance details.
