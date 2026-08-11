## Why

On `/my/profile` → Profile, name, phone, and links disappear after a résumé re-upload while extraction is still pending (or failed). The local case: a current CV upload with a **stale** `resume_structured` stamp, seven banked employments, and a stored structure that still holds contacts — `GET /me/resume` serves bank experience in a Structured shell with empty contact fields, so `ResumeStructuredView` renders jobs without identity.

## What Changes

- Reconcile the résumé read composition with the freshness rule: the Profile tab MUST NOT show a “parsed CV” that has experience but silently empty contacts.
- Through a stale/pending extract window, either keep `structured` null for the file-owned contact/summary/education sections (and keep banked career on the Experience surface), **or** compose provisional contacts from the last stamped structure only when that is explicitly marked pending — pick one coherent contract in design.
- Ensure a re-upload’s background extract is expected to complete (or the UI shows a clear pending/failed state) so the window is temporary, not a permanent blank header.
- Keep `GET /me/profile`’s `cv` block contact-free (API-key / assistant path unchanged).

## Capabilities

### New Capabilities

- (none)

### Modified Capabilities

- `resume-structured-profile`: clarify how `GET /me/resume` and the profile Profile tab behave when the structure stamp is stale but the experience bank has rows — contacts must not vanish into an unlabeled empty header.
- `experience-bank`: align the “bank survives the stale window” read with the résumé surface so banked experience does not imply a contact-stripped Structured on the profile parse view.

## Impact

- `internal/handler/resume.go` (`GetResume` composition)
- Profile UI (`ResumeStructuredView` / pending copy) if a pending flag or null-vs-partial contract changes
- Tests in `resume_experience_test.go` and profile structured rendering
- Local data: re-extract or wait for stamp match after fix; no migration required
