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

- [ ] 4.1 Once the sibling `freehire-cli` commands ship, drain the pending-suggestion queue on the live account and record how many resolve to a link
- [ ] 4.2 Record applications for the July 2026 orphaned progress mail whose employers are in the catalog (Derq, Codurance, Unpack Holdings, Zipdev, Akvelon, Devsu, ioet); leave the 2023–2024 archive alone
- [ ] 4.3 Re-measure link coverage and hand the number to the parked `application-ghosting-signal` change, which was written against 43%
