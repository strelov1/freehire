## Context

`GET /me/resume` already returns `parse_status` (`pending`/`ok`/`failed`) and `parse_detail`
(`resumeMetaResponse`, `internal/api/handler/resume.go:267-279`), computed by
`resume.ResolveExtractStatus` from `users.resume_extract_status`/`resume_extract_detail`
(`owned.go:357-368`). The onboarding wizard (`web/src/routes/onboarding/+page.svelte:280-308`)
already reads it and shows "failed" to the candidate, with a comment explicitly distinguishing
it from `structure_pending` (which conflates pending-and-failed and would wait forever on a
failed extraction). No other page reads `parse_status` at all.

The upload path (`internal/api/handler/resume.go`'s `PutResume`/`ExtractResumeProfile`) stores
the raw upload bytes in blob storage and separately derives plain text from them before calling
`deriveResumeArtifacts(userID, text, uploadedAt)`. The derived text itself is never persisted —
only used for that one request. **`Store.Text(ctx, userID)` (`resume.go:360`) already does
exactly the re-derivation a retry needs**: it fetches the stored bytes and calls `extractText`
(`resume.go:417`), which checks the `%PDF` magic-bytes prefix and either runs `ExtractPDFText` or
returns the bytes as-is — no stored content-type column needed, and no new Store method either.
`Store.Status(ctx, userID)` (`resume.go:332`) already returns the upload's `Meta{Present,
UploadedAt}`, the second thing the retry handler needs to call `deriveResumeArtifacts` correctly.
The retry endpoint is therefore a thin handler over two methods that already exist — not new
storage-layer code.

## Goals / Non-Goals

**Goals:**
- Give the candidate the SAME signal the onboarding wizard already shows, wherever they actually
  manage experience (`/my/profile/experience`, Tailor's Experience tab) — not a new signal, the
  existing one, finally reaching the place that matters for this issue.
- Let a retry happen without asking the candidate to find and re-upload their résumé file again:
  the bytes are already stored, so the fix should use them.

**Non-Goals:**
- No change to the extraction's fail-closed behavior itself (PII-detector-unavailable stays
  fail-closed; LLM-unconfigured stays a no-op). This change is about VISIBILITY and RETRY, not
  about making extraction succeed more often or loosening what it requires to run at all.
- No persisted content-type column — unnecessary, since `extractText`'s `%PDF`-prefix check
  already answers this without one.
- No retry *limit* or cooldown in this change. A candidate repeatedly clicking Retry re-spends
  an LLM call each time, same cost as any other extraction attempt — no different from them
  re-uploading the same file repeatedly today, which the system already allows.

## Decisions

- **Reuse `Store.Text`/`Store.Status`/`deriveResumeArtifacts`, write no new storage code.** The
  retry handler is `Status` (for `UploadedAt`) + `Text` (for the re-derived text) +
  `deriveResumeArtifacts` (the same background call the upload path makes) — three existing
  calls, zero new `Store` methods. This also means a future fix to text re-derivation (a new
  format, an encoding edge case) fixes both the upload path and the retry path at once, since
  there is only one `extractText`.
- **The banner lives in `ExperienceBankView.svelte`, not a new component.** It already renders
  one dismissable-by-state banner (the unconfirmed-achievements one, lines ~477-488) with an
  established visual idiom (`rounded-lg bg-warning/5`); this is a second instance of the same
  idiom, not a new pattern.
- **Fetch `GET /me/resume` inside the bank view itself**, rather than threading it down as a
  prop from the two host pages (`/my/profile/experience`, Tailor's Experience panel). The
  component already owns its own data fetching (`load()` for the bank); adding one more fetch
  keeps both host pages ignorant of a résumé-extraction concept that is really the bank's
  business, not theirs.
- **Retry responds immediately (the derivation stays a background goroutine); the bank view
  polls afterward, not a single refresh.** `deriveResumeArtifacts` only ever launches a
  goroutine and returns — refreshing the bank right after the retry POST responds would show the
  SAME stale `failed` status, not the outcome. The bank view polls `GET /me/resume` with a short
  backoff after Retry is clicked, the same `pending` vs `failed` vs `ok` three-way distinction
  the onboarding wizard's own poll already makes (`parse_status`, never the two-valued
  `structure_pending`, for the same documented reason: waiting on a two-valued signal waits
  forever on a failed attempt). The backoff TABLE itself (`nextResumePollDelayMs`,
  `$lib/onboardingResumeWait.ts`) is reused as-is — its own doc comment calls it "a policy, not
  a mechanism," deliberately extracted so it can be read and reused without opening the wizard,
  and reinventing those tuned numbers here would just be a second table to keep in sync. The
  LOOP BODY (what to do with each state) is NOT shared: onboarding fills form fields, the bank
  view toggles one banner and reloads — different enough jobs that sharing the body would cost
  more in coupling than the few lines it saves.

## Risks / Trade-offs

- [Risk] A candidate whose stored upload was a scanned image PDF (no text layer) retries and
  gets the exact same "ok, no text" or empty result as the original upload, with no clearer
  explanation of why.
  → Mitigation: out of scope — `ExtractPDFText`'s own documented behavior for an image-only PDF
  is "clean run yielding no text", which the ORIGINAL upload handler already renders as a "scan
  or image" rejection; this change does not touch that path, and a true image-only PDF was never
  going to succeed on retry either. Not a regression this change introduces.
- [Risk] Clicking Retry repeatedly spends LLM calls with no cooldown.
  → Mitigation: accepted (see Non-Goals) — identical cost profile to the existing re-upload path,
  which has no cooldown either.
