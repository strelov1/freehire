## 1. Link-state filter on the inbox listing

- [x] 1.1 Add the closed link-state vocabulary (`linked`, `suggested`, `unlinked`) with validation, mirroring how the classification label filter validates its value
- [x] 1.2 Add the link-state predicate to the listing query **and** its pagination count in the same task, so a filtered page can never report an unfiltered total
- [x] 1.3 Accept and validate the query parameter in the listing handler; an unknown value is a 400 before any DB touch
- [x] 1.4 Cover composition with the existing source/unread/label/search filters
- [x] 1.5 Cover the partition invariant: the three link-state totals sum to the caller's unfiltered total and no message appears in two listings

## 2. Recording an application from mail

- [x] 2.1 Extend the apply use case in `internal/jobtracking` to accept a supplied `applied_at`, keeping `LockJobForApply` and the single-increment `applied_count` transition rule in the one existing place
- [x] 2.2 Add the create-and-link action: resolve the slug, refuse an email carrying a pending suggestion, create-or-reuse the interaction, link the email as `manual`
- [x] 2.3 Cover the date rule — `applied_at` is the email's `received_at`, and an interaction that already carries one keeps it
- [x] 2.4 Cover counting — first recording increments `applied_count`, a repeat does not
- [x] 2.5 Cover the refusals: unknown slug 404, another user's email 404, pending suggestion rejected with the suggestion named, unauthenticated 401
- [x] 2.6 Cover idempotency — invoking the action twice for the same email and job changes nothing the second time

## 3. Wiring and verification

- [x] 3.1 Register the route with `mw.key` so a full-scope API key reaches it exactly as a session does
- [x] 3.2 Regenerate the API contract — no diff: the generated contracts cover Go wire structs, and this change adds a route and a query parameter, not a field
- [x] 3.3 Update `internal/handler/AGENTS.md` and `docs/agents/mail-stack.md` with the link-state filter and the create-and-link path
- [x] 3.4 Run `go build ./...`, `go vet ./...`, `go test ./...` and the mail integration tests

## 4. Labelling the real mailbox

- [x] 4.1 Drained the pending-suggestion queue on the live account: 74 → 0. 67 confirmed, 7 rejected as wrong matches (Backblaze, Hostinger and Better Health all pointed at one Gramian role; Avail→Avra and TeamViewer→TeamEx were near-name collisions; one was a Wellfound job alert, not an application at all)
- [x] 4.2 Recorded applications from the July 2026 orphaned progress mail: Codurance (6 messages, one application), Unpack Holdings, Devsu, Zipdev, ioet, Akvelon. The 2023–2024 archive was left alone as decided. Two could not be recorded and are documented below
- [x] 4.3 Re-measured link coverage: **43.2% → 63.7%** (77→79 of 124 applications carry mail; 95→174 linked messages), with no change to `mailmatch` or `mailclassify`. Hand this to the parked `application-ghosting-signal` change, whose thresholds were drafted against 43%

## 5. What the labelling could not resolve

- [x] 5.1 **Derq** — the message names "Full-Stack Engineer (Scalable Systems) role at Derq" verbatim, and that posting is absent from the catalog: seven other Derq roles are ingested, a title search across every company finds only unrelated postings, and `pruned_jobs` holds nothing. An ingest coverage gap, not a matching failure
- [x] 5.2 **Cal.com "2 Hour Meeting"** — not an application at all. The body shows the caller as organiser and a private gmail address as the invitee, with notes about practising array/two-pointer problems: a peer mock-interview session the classifier had labelled `interview_invitation`. Re-triaged to `other`
- [ ] 5.3 Consider whether a hidden-name `getmatch` listing should carry the employer once mail reveals it — the Naranja X message resolved only because the caller had exactly one application to an unnamed fintech, which will not generalise across the 112 such listings in the catalog
