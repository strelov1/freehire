## 1. The two caps

- [x] 1.1 Add to `internal/notify/notify_test.go` a `Digest.Listed()` test: a digest
  of 67 jobs returns 10, a digest of 3 returns all 3, a digest of 0 returns nothing,
  and the returned slice's backing array cannot be used to append into `Jobs`. Fails.
- [x] 1.2 Add `const ListLimit = 10` and `func (d Digest) Listed() []DigestJob` to
  `internal/notify/notify.go`, returning a three-index slice so a renderer appending
  to the result cannot scribble over `Jobs`. Passes.
- [x] 1.3 Add a `buildDigest` test asserting a 67-job match set produces
  `Total == 67` and `len(Jobs) == 67` under the default config, and that a set larger
  than `SnapshotCap` is truncated to it with `Total` still naming the full count.
  Fails.
- [x] 1.4 Rename `Config.DigestCap` → `Config.SnapshotCap`, default 200, and update
  its doc comment to say what it actually bounds now (the recorded snapshot, not the
  message). Passes.
- [x] 1.5 Add a `digestJobsSnapshot` test asserting a 67-job digest marshals 67
  entries — the regression this change exists to prevent. Should already pass once
  1.4 lands; if it doesn't, the snapshot is still reading a truncated list.

## 2. The notification id

- [x] 2.1 Change `RecordNotification` in `internal/db/queries/notifications.sql` to
  `:one` with `RETURNING id`, add `DeleteNotification :exec` (by id), and run
  `make sqlc`.
- [x] 2.2 Absorb the new return value at the `internal/reminder` and
  `internal/nudge` call sites (`_, err :=`) and in their `Store` interfaces, plus
  any test fakes. `go build ./...` is green.
- [x] 2.3 Add `NotificationID int64` to `notify.Digest`, documented as "zero when the
  in-app recording failed — the channel falls back to a generic destination".

## 3. Delivery ordering

- [x] 3.1 Add a `deliverOne` test (fake store + fake notifier) asserting the digest
  the notifier received carried the id `RecordNotification` returned, and that no
  `DeleteNotification` was issued. Fails.
- [x] 3.2 Move the `RecordNotification` call in `internal/notify/deliver.go` above
  the `Send`, assign the returned id onto the digest, and keep the recording failure
  non-fatal (log, continue with id zero). Passes.
- [x] 3.3 Add a test asserting a failing `Send` issues `DeleteNotification` for the
  id just recorded, still records a delivery failure, and does not mark matches
  notified. Fails.
- [x] 3.4 Add the delete-on-failure branch, logging (not returning) a delete error —
  a phantom history row is strictly better than dropping a delivery. Passes.
- [x] 3.5 Add a test asserting a `Send` that fails with `ErrChannelNotConfigured`
  (the soft-skip path) also removes the record, since no digest was delivered.
  Fix if it fails.
- [x] 3.6 Add a test asserting a failing `RecordNotification` still delivers, with
  the digest carrying id zero. Fix if it fails.

## 4. Email

- [x] 4.1 Add to `internal/emailnotify/notifier_test.go`: a 67-job digest renders 10
  job rows in the HTML and 10 in the text, a "57 more" tail in both, and a subject
  naming 67. Fails.
- [x] 4.2 Add: a digest carrying `NotificationID: 42` puts
  `/my/notifications/42/jobs` in both tails; a digest carrying zero puts
  `/my/notifications`. Fails.
- [x] 4.3 Render `d.Listed()` instead of `d.Jobs` and add `viewAllURL()` alongside
  the existing `manageURL()`, keeping the `?utm_source=email` tag the job links
  already carry. Passes.
- [x] 4.4 Confirm the existing tests still pass unchanged — especially the
  single-job digest and the escaping test.

## 5. Telegram

- [x] 5.1 Add to `internal/telegramnotify/notifier_test.go`: a 67-job digest
  itemizes 10 lines and its `+ 57 more` tail is an anchor to
  `/my/notifications/42/jobs`; with id zero it anchors to `/my/notifications`.
  Fails.
- [x] 5.2 Render over `d.Listed()`, and make `moreLine` take the URL and emit an
  `<a>`. Keep the tail-reserve computation honest: it must reserve the *linked*
  tail's length, which is longer than the bare text it reserves today. Passes.
- [x] 5.3 Confirm the existing overflow test still passes — the length guard must
  still fire when job lines are long enough, now measured against the wider tail.

## 6. Docs and verification

- [x] 6.1 Update the `Digest.Jobs is capped` bullet in `docs/agents/notifications.md`
  to describe the two bounds and the record-then-send ordering, and note that the
  in-app row exists if and only if a digest was delivered.
- [x] 6.2 `gofmt -l .` prints nothing; `go vet ./...`, `go test ./...`, and
  `go vet -tags=integration ./...` are green.
- [x] 6.3 `go test -tags=integration ./internal/notify/` — the notification-center
  integration test touches the recording path this change reorders.
- [x] 6.4 `openspec validate digest-lists-ten-with-view-all --strict`.

## 7. Review follow-up

- [x] 7.1 Add a `deliverOne` test asserting that a claimed set larger than
  `SnapshotCap` marks only the delivered jobs notified and releases the rest, and
  that a claimed id with no job row is still marked notified. Fails.
- [x] 7.2 Add `deferOverflow` ahead of `buildDigest` and drop `buildDigest`'s
  `limit` parameter, so `Total == len(Jobs)` by construction. Passes.
- [x] 7.3 Restate the withdrawal as best-effort in the spec, the design, and
  `docs/agents/notifications.md` — a failed `DeleteNotification` leaves a phantom
  row, so "if and only if" was a promise the code does not keep.
