## 1. Check the three verdicts before acting

- [x] 1.1 #27: trace every writer of `emails.job_id`/`suggested_job_id`/`application_id` to see
      whether the divergent state is reachable. Record the answer either way.
- [x] 1.2 #29: confirm the verifier's refutation rather than inherit it — `GREATEST` ignoring
      NULLs is what makes the claimed NULL `last_activity_at` unreachable.
- [x] 1.3 Confirm the three day-math copies are behaviourally identical before merging them.

## 2. One predicate (#27)

- [x] 2.1 The board asks `e.application_id IS NULL` like the other three; the comment says why the
      old spelling was equivalent only by coincidence. Regenerate.

## 3. One membership test (#28)

- [x] 3.1 Export `notify.ValidChannel`; delete both hand-built allowlists.
- [x] 3.2 Correct `notifications.md`: the "adding a package, not touching notify or reminder"
      claim, and the emailnotify table row that says it implements both interfaces.
- [x] 3.3 Do NOT collapse the Router/Notifier pair — record why, so the deferral is a decision.

## 4. One day count (#29)

- [x] 4.1 `userjob.DaysSilent` beside `SilenceStateFor`; three call sites.
- [x] 4.2 Test it, including the future-timestamp case each copy guarded separately.

## 5. Verify and close

- [x] 5.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 5.2 Mark #27/#28/#29 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
