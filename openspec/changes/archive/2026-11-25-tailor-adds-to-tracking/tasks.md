## 1. Wire tracking into CV handlers

- [x] 1.1 Add a narrow save dependency on `cvHandlers` (e.g. interface with
      `SaveJob(ctx, userID, jobID)`) and pass the shared `jobtracking` repository
      or service from `Register` / `newCVHandlers` — no post-construction setter.
- [x] 1.2 Update unit/integration test constructors that build `cvHandlers` /
      `newTailorAPI` so they compile with the new dependency (nil-safe or a stub
      save for tests that do not assert tracking).

## 2. Save on tailor bootstrap

- [x] 2.1 In `TailorCV`, after the tailored CV and session are ready, call the
      save path for `(userID, job.ID)`. Log failures; do not change the success
      response. Do not set `applied_at` or `stage`.
- [x] 2.2 Ensure the save runs on both first create and idempotent resume (not
      gated on `justCreated`).

## 3. Tests

- [x] 3.1 Integration test: first successful `POST /me/cvs/tailor` leaves
      `user_jobs.saved_at` set for that vacancy and leaves `applied_at` / `stage`
      unset from the bootstrap.
- [x] 3.2 Integration test: resume bootstrap for an existing tailored CV with no
      save mark sets `saved_at` (heal path).
- [x] 3.3 Integration test (or unit with stub): when save returns an error after
      CV/session creation, bootstrap still returns 201 with the CV and session
      ids.
- [x] 3.4 Run `go vet -tags=integration ./...` (and the relevant
      `go test -tags=integration` tailor handler packages) before considering
      done.

## 4. Verify

- [x] 4.1 Manually or via integration: after tailor, the vacancy appears under
      Tracking board (`filter=board` / saved). Confirm unsave clears the board
      card without deleting the tailored CV.
