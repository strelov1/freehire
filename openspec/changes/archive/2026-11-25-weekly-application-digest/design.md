# Design

Deliver Mondays at the account's existing digest time and timezone using cmd/notify, independently of search availability. Consent defaults off; omitted settings preserve it. Use the existing paid inbox digest eligibility policy.

Report applications submitted and recorded replies/interview/offer transitions over the preceding seven days, together with current stage totals. Preparing cards are not submitted applications. Use the application event ledger for historical activity, preserving its source semantics.

Show at most five applications that satisfy the existing silence rule. Pending unconfirmed suggestions do not justify a silence claim. Show the most recent follow-up date beside silence; never send employer mail. Links open the application's existing tracking surface. Empty activity and action snapshots consume no delivery claim.

Take a unique user/local-week claim before dispatch, rechecking live consent and eligibility. Keep uncertain delivery outcomes claimed, release definite pre-dispatch mail pauses, and finalize accepted sends with a bounded detached context. Include activity unsubscribe and generated-notification headers; inbound Gmail and hosted readers skip this exact digest kind.

Tests cover consent, entitlement, local scheduling, weekly claims, snapshot semantics, empty delivery, rendering safety and provider failure outcomes. No new timer or model call is needed.
