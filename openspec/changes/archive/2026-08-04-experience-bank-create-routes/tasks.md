## 1. Backend: POST /me/experience/employments and /atoms

- [ ] 1.1 Extend `experienceBankOwner` in `internal/handler/me_experience.go` with
  `AddAtom(ctx, userID, experience.Atom) (experience.Atom, error)` and
  `CreateEmployment(ctx, userID, experience.Employment) (experience.Employment, error)` —
  both already implemented on `*experience.Store`.
- [ ] 1.2 Add `AddEmployment` and `AddAtom` handler methods: parse the body, for the atom
  handler force `Provenance: experience.ProvenanceManual` before calling the store (mirror
  `UpdateAtom`'s existing pattern), map `experience.ErrEmptyClaim` /
  `ErrInvalidProvenance` / `ErrEmptyEmployment` / `ErrInvalidKind` through the existing
  `experienceError` helper, respond `201` with `{"data": <created>}`.
- [ ] 1.3 Register `POST /me/experience/employments` and `POST /me/experience/atoms` under
  `mw.key` (the same middleware `GET /me/experience` already uses — full scope or cookie,
  no new scope).
- [ ] 1.4 Tests in `internal/handler/me_experience_test.go` (or wherever the existing
  `/me/experience` tests live): creating a valid employment/atom persists and returns 201;
  an invalid one is refused and nothing persists; a caller-supplied non-manual provenance on
  an atom is silently overridden to `manual`; a cookie-only or full-scope-key caller can
  reach both routes (matching the existing GET's auth test, if one exists) — a request with
  no credential is refused.

## 2. freehire-cli: experience list / employments add / atoms add

- [x] 2.1 `internal/client/experience.go` (new, in `github.com/strelov1/freehire-cli`):
  `ListExperience`, `CreateEmployment`, `CreateAtom`, following the shape of
  `internal/client/cv.go`'s existing methods (`c.do(...)`, typed params structs).
- [x] 2.2 `internal/cli/experience.go` (new): `experience` command group with `list`,
  `employments add`, `atoms add` subcommands, following `internal/cli/jobs_authoring.go`'s
  `add`-verb convention and flag style (`mustString`/`mustBool` helpers). `atoms add` takes
  `--claim` (required), `--context`, `--metric` (repeatable), `--skill` (repeatable),
  `--employment` (id, optional). `employments add` takes `--kind` (job|project, default
  job), `--company`, `--role`, `--location`, `--start`, `--end`, `--current`, `--summary`.
- [x] 2.3 Tests in `internal/cli/experience_test.go` and `internal/client/experience_test.go`
  (`httptest.Server`, matching the existing `cv_test.go` pattern in both packages).
- [x] 2.4 Updated the CLI's own README (`## Use` command list + an "Experience bank"
  explainer paragraph). The version bump / release (new tag, cross-built binaries, `gh
  release create`) is deliberately NOT done here — that is a real deploy of the public
  installer real users pull from (`hire-cli-release-ops`), left for when the change is
  actually ready to ship, not bundled into this commit.

## 3. Verify

- [x] 3.1 Backend: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`,
  `go test ./...` — clean. Also `go test -tags=integration ./internal/handler/...`.
- [x] 3.2 `openspec validate --all --strict` — 213/213.
- [x] 3.3 freehire-cli: `go build ./...`, `go vet ./...`, `go test ./...` in that repo — clean.
- [x] 3.4 Manual smoke: done against real prod (2026-08-04), after `release.sh` deployed
  freehire#1574/#1575 and freehire-cli v0.17.0 was cut and downloaded fresh (not the
  pre-existing installed binary). `POST /me/experience/employments` and
  `POST /me/experience/atoms` verified unauthenticated-401 (not 404) post-deploy, then a
  real `experience employments add` + `experience atoms add --employment <id>` against the
  live server returned `"provenance":"manual"` and showed up correctly grouped in
  `experience list`. Did NOT chain into `cv edit --evidence <id>` against a real CV — that
  would have written test text into the account's actual CV revision history, which the
  bare bank entries don't; the create→list→provenance chain already covers what `cv
  edit`'s own evidence-gate integration tests independently prove. Test entries
  (`"TEST — delete me"`) left for the account owner to remove via the site, since delete
  stays cookie-only by design (see this change's own design.md).
  effect of archiving.
