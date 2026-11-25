## Context

See proposal.md for the full diagnosis. In short: `web/src/routes/my/profile/experience/+page.svelte`'s `offerRefreshAfterBankEdit` catches every failure from `api.resetBaseCvFromResume()` and always sets one hardcoded string, discarding the backend's own, more specific `ApiError.message` (already correct and already the JSON `error` field the handler sends — see `internal/api/handler/cv.go`'s `mapCVError`, `cvedit.ErrListCap` branch).

## Goals / Non-Goals

**Goals:**
- Show the backend's specific refusal reason when the caught error carries one.
- Keep the existing generic message as a fallback for a failure with no useful message of its own.

**Non-Goals:**
- No backend changes — the message the server sends is already correct.
- No new error-message component or pattern — this is a one-line change using a helper (`errorMessage` in `web/src/lib/utils.ts`) already established and already used by sibling catch blocks in the same file's parent component (`ExperienceBankView.svelte`).

## Decisions

**Use the existing `errorMessage(e, fallback)` helper rather than a new one.** It already implements exactly the wanted rule (`e instanceof Error ? e.message : fallback`) and is already the convention this feature area follows elsewhere. Introducing a second helper, or inlining the check without going through the shared one, would just be a second spelling of the same rule.

Alternative considered: special-case `ApiError` specifically (`e instanceof ApiError ? e.message : fallback`) to guarantee only a server-authored message is ever shown verbatim. Rejected — `errorMessage`'s broader `Error` check is what every sibling catch block in this file already uses, and any `Error` thrown by `api.ts`'s request path in practice carries a reasonable `.message` (network failures included, e.g. `TypeError: Failed to fetch`, which is not misleading, just less specific than the generic fallback). Matching the established local convention outweighs the marginal extra precision.

## Risks / Trade-offs

- [Risk] A future backend error path could start returning a low-quality or overly technical message that would now reach the candidate verbatim → Mitigation: none needed beyond ordinary review of new error messages at the point they are added — this fix does not introduce new backend messages, only stops discarding the ones that already exist.
