# Paid gate for user-initiated AI

## Status

Implemented. Design approved by the product owner on 2026-10-14.

## Goal

Every model call made for a user's action in freehire requires an active Pro or Ultra entitlement. Free accounts receive no AI usage, including no free daily allowance and no environment-variable override that can restore one.

The gate is server-side and covers calls regardless of which signed-in product surface initiated them. A refusal must happen before the model call, including before any streaming response starts.

## Scope

The gate covers every user-attributed model call, including the current metered actions and the currently unmetered call groups:

- Existing plan-metered actions: match analysis, assistant, CV tailoring, cover letters, dictation, and auto-apply.
- Other user-requested AI: structured CV extraction, ATS qualitative review, natural-language search intent, mail recall, and autofill planning.

The audit seam is `internal/ai/llmkey` and the user-call sites that bind a model call to a user. The implementation must make a missed gate difficult to introduce when another user-facing model call is added.

## Behavior

- A live Pro or Ultra entitlement may proceed, subject to the existing daily limits on actions that already use the plan ledger. This change does not invent new paid-tier quotas.
- A Free or expired entitlement receives the existing payment-required response (HTTP 402); no model call is made and no AI result is written.
- If the plan cannot be read, the request fails closed with HTTP 503; an entitlement lookup failure must never silently authorize model spend.
- Recomputing or reading an already-cached AI result does not require a new entitlement check unless that request would make a model call.
- Deterministic features remain available to Free accounts, including ordinary search, deterministic CV/ATS scoring, and the public CV roast. The public roast does not call a model.

## Explicit non-goals

- Background catalogue enrichment, embeddings, and other ownerless worker calls are unchanged.
- The model provider, gateway, model routing, API credentials, and billing-provider integration are unchanged.
- This does not add daily usage counters or new Pro/Ultra limits for the previously unmetered actions.
- It does not hide read-only cached results from a Free user.

## Configuration

Free usage is a product invariant, not an operator-adjustable quota. `PLAN_FREE_DAILY_<FEATURE>` must stop changing runtime behavior and be removed from operator-facing configuration and documentation. The plan decision itself must reject Free regardless of a positive Free allowance supplied programmatically, so configuration drift cannot reopen access. Test-only helpers may continue to construct edge-case configurations where useful, but may not weaken the production invariant.

Pro/Ultra daily-limit overrides and the future-feature `PLAN_ENFORCE` mechanism remain unchanged where still applicable.

## Failure and response handling

The paid-entitlement refusal uses the established plan/payment error shape so the web and extension surfaces can present their existing upgrade path. It must occur before SSE headers or any other irreversible response output. A database or entitlement read failure is distinct from a Free refusal and returns 503 without falling back to the service credential.

Existing per-action ledger reservations and releases continue to behave as they do today. The paid entitlement check supplements those limits; it does not replace action accounting.

## Verification

- Unit tests prove Free is refused even when a positive Free allowance is configured, Pro and Ultra pass, and expired entitlements resolve to Free.
- Tests prove plan-read failures do not permit a model call.
- Handler/route tests cover every user-facing model-call group, asserting Free is rejected before the model fake is invoked and paid users retain current behavior.
- Streaming paths prove the refusal status is sent before the stream begins.
- Regression tests prove cached reads and deterministic CV/search paths remain available to Free.
- A structural coverage check or equivalent centralized seam prevents a new user-attributed model call from bypassing the paid check.
- Background enrichment and embedding paths remain unaffected.

## Design choices

Three shapes were considered:

1. Add independent paid checks to each handler. This is locally straightforward but leaves every new call site vulnerable to omission.
2. Add a daily ledger quota for every AI call group. This gives more product controls, but requires inventing new Pro/Ultra limits and is broader than the request.
3. Use a shared server-side paid-entitlement check for all user-attributed model calls, preserving existing per-action ledgers where already present. This is the selected approach: it enforces the invariant once at the shared user-call boundary without inventing new quotas.
