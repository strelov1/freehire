# PII-masking conventions

## Scope
Fail-closed masking of personally-identifiable information in CV text before it reaches an
LLM: a regex floor (email, URL, @handle, phone) unioned with name/address spans from the
co-located privacy-filter model (`services/pii-filter`, reached over HTTP), producing a
reversible `Redactor` of numbered placeholders. Used on the CV→LLM path that survives: the
structured-CV parse in `internal/candidate/resumeextract`, entered via `cmd/server`,
`cmd/backfill-experience`, `cmd/backfill-resume-structured`.

## Always true
- **Fail-closed, twice.** `Build(ctx, text, detector)` (redactor.go:50) returns an
  ERROR on a nil detector or a detector error rather than a partial regex-only redactor —
  callers refuse to send the CV at all. Then a post-build self-check (redactor.go:112-120)
  masks the source text and refuses the whole redactor if any detected value survives, so a
  boundary quirk we did not foresee fails safe instead of leaking.
- **The detector is the seam, not the model.** `Detector` (client.go:13) returns spans
  already mapped to our `Kind` vocabulary (`Span`, `Kind*` consts, detect.go:9-23);
  `NewHTTPDetector` (client.go:25) posts to `PII_FILTER_URL` and any non-2xx is an error.
  Tests inject a fake. The package owns no model and no vocabulary negotiation.
- **`PII_FILTER_URL` empty disables the features entirely.** cmd/server treats an unset URL
  as "fail the CV→LLM paths closed" (cmd/server/main.go:245), not as "send unmasked" — the
  analysis buttons degrade rather than leak.
- **Build reads one text and masks what it detected there.** It takes no caller-supplied
  contact list: `Build` once accepted one so `matchanalysis` could mask a structured JSON
  blob with the redactor built from the raw CV, and that consumer was deleted — the
  surviving caller passed an empty set, which made the whole known-contacts path a proven
  no-op. Restoring it means restoring the second text it existed for.
- **Word-boundary masking is the exception, plain replacement the rule.** Only the "wordy"
  kinds (NAME, ADDRESS) get `\b`-anchored regexes, and only when EVERY detected occurrence
  of the value is boundary-complete; one occurrence abutting a word char makes the value
  unsafe and it is masked plainly (redactor.go:72-77, 101-108). Emails/phones/links are
  always plain — they never occur inside a real word, so over-redaction is not a risk but
  leaking is. Replacements run longest-first so a value contained in another masks first.
- **Contact fields are extracted deterministically, never by the LLM.** `Redactor.Contacts()`
  (redactor.go:34) recovers first name/email/phone/location and all links from the detected
  spans; `fillContact` (redactor.go:132-156) is defensive on purpose — the model mis-tags
  handles as people and its URL spans swallow neighbours, so only well-formed values
  (`isPlausibleName`, `isCleanLink`) become stored contact fields.
- **The phone regex knows about employment ranges.** A bare `YYYY-YYYY` is explicitly
  excluded (detect.go:33-35), and a phone span overlapping an already-detected email/URL/
  handle is dropped so digits inside a link are never mis-read as a phone (detect.go:38-58).
  The @handle regex never fires inside an email (detect.go:29-30).
- **Restore is exact.** Placeholders are unique per kind (`[REDACTED_KIND_n]`), so
  `Restore` maps model output back to the original values regardless of order — the LLM
  sees and emits only placeholders.
- **`PlaceholderPrefix`/`ContainsPlaceholder` are the marker's one spelling.** A consumer
  that PERSISTS model output has to be able to recognise a placeholder the model copied
  into a field it was not asked for — `resumeextract` blanks those (scrub.go). Restore is
  the wrong tool for that: it would put the real value back into a field a de-identified
  reader is about to see.

## Consumers
- `internal/candidate/resumeextract` — redacts the stored CV before the structured-parse
  prompt, fills contact fields from `Contacts()`.
- `internal/api/handler/handler.go`, `cmd/server` — wire the detector from config.
- `cmd/backfill-experience`, `cmd/backfill-resume-structured` — same detector for batch
  re-parses; both log and exit early when `PII_FILTER_URL` is unset.
