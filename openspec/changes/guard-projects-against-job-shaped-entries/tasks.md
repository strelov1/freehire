## 1. The detection + sentinel error

- [x] 1.1 RED (via `Commit`, not a standalone regex test — matches this package's own
      convention of testing `listcap.go`/`policy.go` through `Commit` behavior rather than
      their internals): `internal/candidate/cvedit/projectshape_test.go` asserted
      `ErrProjectLooksLikeJob`/the refusal behavior before either existed; confirmed it failed
      to compile (`undefined: ErrProjectLooksLikeJob`) for the right reason.
- [x] 1.2 GREEN: implemented `internal/candidate/cvedit/projectshape.go` —
      `ErrProjectLooksLikeJob`, `dateRangeInName`, `projectPathIndex`,
      `refuseIfProjectLooksLikeJob(ops []Op, applied State) error`.
- [x] 1.3 Covered by the same test file: job-dated names refused (3 cases), plain names and a
      single bare year allowed (3 cases), a bullet mentioning a year allowed (bullets are
      never checked).
- [x] 1.4 (folded into 1.2 — one GREEN pass implemented both the regex and the function).

## 2. Wire the guard into the editor, actor-scoped

- [x] 2.1 `TestCommitRefusesAgentJobDatedProjectName`: `Actor: ActorAgent` + a job-dated
      `projects[0].name` → `errors.Is(err, ErrProjectLooksLikeJob)`, project name unchanged.
- [x] 2.2 `TestCommitAllowsCandidateJobDatedProjectName`: the same text via
      `Actor: ActorCandidate` → no error, text is written as-is.
- [x] 2.3 GREEN: hooked `refuseIfProjectLooksLikeJob(ch.Ops, applied)` into `commit()` in
      `editor.go`, right after `refuseIfSanitizeDropsContent`, gated on
      `ch.Actor == ActorAgent`.
- [x] 2.4 `TestCommitRefusesWholeBatchWhenOneProjectIsJobDated`: a batch of two project-name
      sets, one job-dated — both projects end up unchanged.
- [x] 2.5 Passed without further changes, as expected.

## 3. Surface the error on the CLI-edit REST path

- [x] 3.1 `internal/api/handler/cv_project_shape_test.go`:
      `TestMapCVErrorProjectLooksLikeJobIsUnprocessableWithItsOwnMessage` — RED confirmed
      (`mapCVError` returned the raw wrapped error, not a `*fiber.Error`).
- [x] 3.2 GREEN: added the `cvedit.ErrProjectLooksLikeJob` case to `mapCVError` in
      `internal/api/handler/cv.go`, mapping to 422 with the error's own sentence.

## 4. Simplify, verify, review

- [x] 4.1 Reviewed the diff against `listcap.go`/`policy.go` conventions — no duplication or
      dead code found; no edits needed.
- [x] 4.2 `go test ./internal/candidate/cvedit/... ./internal/api/handler/...` — both green.
- [x] 4.3 `go vet ./internal/candidate/cvedit/... ./internal/api/handler/...` — clean.
- [x] 4.4 Code review requested (subagent) on the diff.
- [x] 4.5 Review found an Important bug: `refuseIfProjectLooksLikeJob` resolved an op's path
      index against the FINAL `applied.Projects`, which is wrong whenever a later op in the
      same batch inserts/removes ahead of it and shifts that index — a job-dated `set` followed
      by an `insert` at the same path slips through uncaught. RED: added
      `TestCommitRefusesJobDatedNameEvenWhenALaterInsertShiftsItsIndex`, confirmed it failed
      against the original implementation. GREEN: rewrote the check to read each op's own
      `Value` directly (the string for a `.name` path, or the `name` field of a whole-project
      value) instead of re-deriving position from the final document — this sidesteps index
      correspondence entirely. Dropped the now-unneeded `applied State` parameter and the
      `projectPathIndex` regex. Re-ran the full suite: still green, new regression test passes.
