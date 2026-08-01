## 1. Pin the defect and the contract before touching the interface

- [x] 1.1 Unit-test the search predicate against the shape it has to serve — an application with a posting, one without (employer known only as a slug), a query matching the role, a query matching neither, and a query in the wrong case. **RED first:** the predicate does not exist yet, and its answers are the part of this change with real choices in them
- [x] 1.2 Unit-test the row-id decoder: `a<digits>` resolves to an application, anything else is read as a posting slug, and a malformed value is not mistaken for either

## 2. Writes addressed by the row the listing served

- [x] 2.1 Add the id decoder to `internal/handler`, with the doc comment that says why two forms exist and where they come from (`internal/jobtracking/repository.go:209` and `:243`)
- [x] 2.2 Register `PATCH /api/v1/me/applications/:id` reusing the existing track validation — the stage vocabulary, the partial update, the `400` on an unknown stage and on an empty body
- [x] 2.3 Register `DELETE /api/v1/me/applications/:id` and `DELETE /api/v1/me/applications/:id/stage`
- [x] 2.4 Make every one of the three answer `404` with the body a missing row produces, for a malformed id and for another user's row alike — one answer, not two
- [x] 2.5 Integration test: a stage change on an application whose posting was pruned is recorded. This is the case the slug-addressed route cannot serve, and the reason these routes exist
- [x] 2.6 Integration test: the slug-addressed `PATCH /api/v1/jobs/:slug/track` still behaves exactly as before — the CLI and MCP address postings and must not move

## 3. The card carries no controls

- [x] 3.1 Strip `BoardCard` to indicators: remove the stretched open overlay, the follow-up button and the rehearse button; keep the stage badge, the silence marker, the mail count and the notes mark
- [x] 3.2 Make the card one `role="button" tabindex="0"` element that opens the application on click and on Enter/Space. A card that cannot be opened from the keyboard is a regression, not a detail
- [x] 3.3 Drop the now-unused `onfollowup`/`onrehearse` props from `BoardCard` and `BoardColumn`, and the `canRehearse` stage gate from `web/src/lib/rehearsal.ts`
- [x] 3.4 Confirm in a browser: the card drags from its body, its badge row and its title alike, and a click still opens the application. The defect was invisible to every existing test and cannot be signed off by one

## 4. The opened application carries the actions

- [x] 4.1 Add the action row to `JobDrawer`, below the meta pills and above the tabs, so it shows on every tab and does not fight `View job` for the corner
- [x] 4.2 Wire Rehearse (moved from `JobBoard.startRehearsal`, offered at any stage) and Follow up (opening the existing `FollowUpDialog`, keeping its `canFollowUp` gate)
- [x] 4.3 Wire Analyze to switch to the existing Job Match tab, and Tailor CV to navigate to `/tailor/[slug]`
- [x] 4.4 Hide Rehearse, Analyze and Tailor CV when `item.job` is null — absent, not disabled, as `View job` already is. Follow up stays: the chase is addressed to the employer, which the application knows by itself

## 5. Point the board's writes at the new routes

- [x] 5.1 Add the client calls to `web/src/lib/api.ts` and switch `JobBoard`'s `persistMove`, `setStage`, `saveNotes` and `remove` onto them
- [ ] 5.2 Verify by hand that a posting-less application now moves between columns and stays put — before this change it reverted, which is the bug the routes were added for. **Folded into 8.2:** it needs the same local stack, and standing that up twice buys nothing

## 6. The list view

- [ ] 6.1 Rename the route `web/src/routes/my/tracking/[slug]` to `[id]` and fix its `load` and its callers — it addresses a row, not a posting
- [ ] 6.2 Add the List tab to `web/src/routes/my/tracking/+layout.svelte`, between Board and Pipeline
- [ ] 6.3 Add `web/src/routes/my/tracking/list/` with the same server load the board uses — no second fetch path
- [ ] 6.4 Build the row: employer, role, stage control, days silent, mail count; ordered by last activity, newest first; opening the same `JobDrawer`
- [ ] 6.5 Exclude saved-only rows, the way the board excludes them — a bookmark is not an application

## 7. Search over both views

- [ ] 7.1 Implement the predicate in `web/src/lib/board.ts` against the tests from 1.1
- [ ] 7.2 Add the search field, shared by the board and the list, synchronised to `?q=` — linkable and surviving a reload
- [ ] 7.3 Verify that clearing the field restores every application and drops `q` from the URL

## 8. Finish

- [ ] 8.1 `go build ./... && go vet ./... && go test ./...`, and the web checks
- [ ] 8.2 Walk the whole surface in a browser: drag a card, open an application and use each of its four actions, switch to the list, search from both views, and reload on a searched list
- [ ] 8.3 Offer a `/blog` changelog entry — the drag repair and the list view are both user-facing
