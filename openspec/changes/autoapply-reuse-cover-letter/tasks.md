## 1. Field recognition

- [x] 1.1 Add an `isCoverLetterTextField` (or equivalent) check in `internal/api/atsapply`, mirroring `isResumeField`'s id-then-label shape, matching the known `cover_letter_text` id and a narrow label fallback.
- [x] 1.2 Unit test: a field with id `cover_letter_text` is recognized; a field labeled "Cover Letter" with an unrelated/opaque id is recognized via label fallback; an ordinary free-text question (e.g. "Why do you want to work here?") is not.

## 2. Letter lookup

- [x] 2.1 Add a narrow `LetterReader` interface to `internal/api/atsapply` (mirroring `AtomReader`'s shape), matching `coverletter.Store.Get`'s exact signature — `Get(ctx, userID, jobID int64) (*coverletter.Stored, error)`.
- [x] 2.2 No adapter needed — confirm `*coverletter.Store` satisfies `LetterReader` directly (the same structural fit `*experience.Store` already has for `AtomReader`), so this task is "verify, don't build."
- [x] 2.3 Unit test (in the calling code from §3, since there is no new adapter type of its own to unit test here): a nil `*Stored` (no letter) and a real read error from a fake `LetterReader` are handled distinctly and neither panics.

## 3. Resolution wiring

- [x] 3.1 In `ResolveWithDrafting` (or the per-field loop it drives), route a cover-letter-semantic unmapped field to the letter-reuse path before falling through to `Drafter.Draft`.
- [x] 3.2 When a letter exists: use its body verbatim as the field's answer (no truncation — this package's form model carries no field-length constraint to bound against); run it through the same `matchOption` check every other answer gets.
- [x] 3.3 When no letter exists, or the lookup hits a real read error: fall through to the existing `Drafter.Draft` path unchanged (log the read error the same way `buildGroundingContext`'s failure is logged in client.go, don't fail the attempt).
- [x] 3.4 Wire the real `LetterReader` adapter into `Client` (client.go) alongside the existing `AtomReader`, passed through to `ResolveWithDrafting`.

## 4. Tests for the added spec requirements

- [x] 4.1 Scenario test: a cover-letter field with an existing tailored letter resolves to that letter's content and never calls the drafter (assert on a call-counting fake `Drafter`, mirroring `TestSubmit_BrowserUseFallback_UnresolvedPlanNeverCallsBrowserUse`'s pattern).
- [x] 4.2 Scenario test: no letter exists for the (user, job) pair — the field is resolved by the existing generic drafter exactly as before this change.
- [x] 4.3 Scenario test: an unrelated free-text field is unaffected by this change (still goes to `Drafter.Draft` regardless of letter existence).
- [x] 4.4 Run `go vet -tags=integration ./...` and the full package test suite for `internal/api/atsapply` and `internal/candidate/coverletter`.
- [x] 4.5 Found on review (CodeRabbit): a stored letter with a blank body was being treated as a valid answer (`matchOption` accepts an empty string for a free-text field with no options), silently answering a required field with nothing instead of falling through to the drafter. Fixed in `coverLetterAnswer`; added `TestCoverLetterAnswer_ABlankStoredBodyIsNotAnAnswer` and `TestResolveWithDrafting_ACoverLetterFieldWithABlankStoredBodyFallsBackToTheGenericDrafter`.
- [x] 4.6 Found on review (CodeRabbit): `isCoverLetterTextField`'s ID/label match had no `Kind` restriction, so a `select`/`radio` field whose label happened to match (e.g. a "Cover Letter" yes/no dropdown) was routed to letter-reuse too — the letter's prose is never one of the platform's own option labels, so `matchOption` would just park the field, where the generic drafter has a real chance of picking a valid option. Restricted reuse to `Kind == "text"`/`"textarea"` in `answerFor`; added `TestResolveWithDrafting_ACoverLetterMatchingSelectFieldIgnoresTheStoredLetter`.

## 5. Wrap-up

- [x] 5.1 Update `internal/api/atsapply/AGENTS.md` (and `internal/candidate/coverletter/AGENTS.md` if it documents consumers) to note that auto-apply reuses an existing letter for the cover-letter form field, falling back to the generic drafter.
- [x] 5.2 `gofmt -l .`, `go vet ./...`, `go test ./...` clean before commit.
