Every task runs the spec-driven-tdd micro-cycle: RED (a failing test first) → GREEN →
REFACTOR → simplify → only then `[x]`.

## 1. Education-level dictionary (`internal/dict/edulevel`)

- [x] 1.1 New package `internal/dict/edulevel`. `ForRequirement(text string) string`:
      extract the existing possessive-framed regex/alias matching verbatim from
      `internal/job/jobfacts.go`'s `matchBachelor`/`matchMaster`/`matchPhD`-style checks
      (jobfacts.go:101-106). Port `jobfacts_test.go`'s existing education-level cases as
      this package's own tests first (RED against the new package before the extraction
      lands), confirming byte-identical behavior to today's `jobfacts.EducationLevel`.
- [x] 1.2 Add `ForDegree(text string) string` to `edulevel` for CV-style shorthand: "BSc",
      "B.Sc", "BS", "Bachelor of Science", "MSc", "M.Sc", "MBA", "PhD", "Ph.D",
      "Doctorate", "Master of ..." forms. Returns `""` for anything that doesn't match
      (e.g. "Certificate", a bare institution name). Tests: each supported shorthand
      resolves to the right one of `bachelor`/`master`/`phd`; unresolved input returns
      `""`; confirm it does NOT return `"none"` for anything (that value is
      requirement-only).
- [x] 1.3 Refactor `internal/job/jobfacts.EducationLevel` to delegate to
      `edulevel.ForRequirement(hardRequirementText(description))`. `hardRequirementText`
      stays in `jobfacts`/`optional.go` unchanged (depends on `internal/job/reqextract`,
      which `dict` may not import). Existing `jobfacts_test.go` cases must stay green
      unmodified — this step is behavior-preserving, not behavior-changing.
- [x] 1.4 `go vet ./...` clean; confirm `internal/platform/arch/layering`'s test still
      passes (new package `internal/dict/edulevel` correctly placed in the layering table,
      `internal/dict/AGENTS.md`'s block table updated to list it).

## 2. Certification dictionary (`internal/dict/certification`)

- [x] 2.1 New package `internal/dict/certification`: `Canonicalize(tokens []string)
      []string`, modeled on `internal/dict/skilltag.Canonicalize`'s shape. Seed a curated
      alias→canonical map covering common certifications (AWS Certified — Solutions
      Architect/Developer/SysOps at minimum, Azure/GCP equivalents, PMP, CKA, CKAD, CISSP,
      CompTIA Security+/Network+, a Scrum certification). Tests: known aliases resolve to
      their canonical form, duplicate input dedups, unresolved input is dropped (not
      passed through, not erroring).
- [x] 2.2 Add `internal/dict/certification` to the layering table
      (`internal/platform/arch/layering/blocks.go`) and its `AGENTS.md`.

## 3. Public card projection (`internal/candidate/talentnetwork`)

- [x] 3.1 Add `EducationEntry{Level string; Year *perioddate.PeriodDate}` and
      `CandidateCard.Education []EducationEntry` / `.Certifications []string` fields in
      `card.go`.
- [x] 3.2 Implement `cardEducation(education []resumeextract.Education)
      []EducationEntry` in `card.go`: for each entry, resolve `edulevel.ForDegree(e.Degree)`
      and skip the entry when it returns `""`; otherwise emit `{Level, Year: e.Year}`. Wire
      into `ProjectCard`.
- [x] 3.3 Wire `certification.Canonicalize(s.Certifications)` into `ProjectCard` for the
      new `Certifications` field.
- [x] 3.4 Update `card.go`'s header comment (the "languages, certifications and education
      carry no dictionary this block can reach" paragraph) to state the new, narrower
      withholding: only languages, institution name, certification issuer/date and field
      of study remain withheld; education level/year and certifications are now carried.
- [x] 3.5 Tests in `card_test.go`: an education entry with a resolvable degree appears
      with level+year and no institution field anywhere in the marshalled struct; an
      entry with an unresolvable degree is absent, while the candidate's other resolvable
      entries still appear; a resolvable certification appears canonicalized; an
      unresolvable one is absent.

## 4. Frontend

- [x] 4.1 Run `make gen-contracts` after step 3.1 lands, so
      `web/src/lib/generated/contracts.ts` picks up `CandidateCard`'s two new fields.
      Commit the regenerated file alongside the Go change that produced it.
- [x] 4.2 Add `EDUCATION_LEVEL_LABELS` (`bachelor` → "Bachelor's degree", `master` →
      "Master's degree", `phd` → "PhD") and `CERTIFICATION_LABELS` (canonical code →
      display name, covering every entry the curated dictionary from step 2.1 defines) to
      `web/src/lib/labels.ts`.
- [x] 4.3 Add "Education" and "Certifications" sections to
      `web/src/routes/talent/[handle]/+page.svelte`, following the existing `{#if
      card.skills.length}` / `Chip` pattern used by the "Skills" section. An education chip
      reads "Bachelor's degree · 2019" when `Year` is present, "Bachelor's degree"
      otherwise. Both sections are hidden when their array is empty.

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `go test ./...` green (this machine needs
      `SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk` set — see
      `~/.zshrc`). Confirmed: only pre-existing, unrelated failure is
      `cmd/billing-sync`'s `TestTheStoreProviderAloneKeepsTheWorkerRunning` (no billing
      provider configured in this environment — present before this change, untouched by
      it).
- [x] 5.2 `pnpm check` (or the equivalent frontend build/typecheck) green after the
      `contracts.ts` regeneration and the new Svelte sections. `pnpm run check` in `web/`:
      0 errors, 39 pre-existing warnings unrelated to this change.
- [ ] 5.3 Manual browser pass: open a Talent Network card for a member whose CV has
      education/certification data, confirm the new sections render, and confirm the
      page's network response never contains an institution name, a certification issuer,
      or a certification date. **Not done automatically**: this machine's shared Docker
      host port 5432 is already bound by another concurrent worktree's Postgres
      (`mentor-directory-filters-db-1`), so `make up` would either conflict or require
      reusing that other session's live database — both unsafe to do unattended. The
      unit-level equivalent (`TestProjectCard_CarriesResolvedEducation`,
      `TestProjectCard_DropsUnresolvedEducation`,
      `TestProjectCard_KeepsOnlyResolvedCertifications`, and the existing
      `TestProjectCard_LeaksNothingFromTheCV` suite) covers the same guarantee at the Go
      layer; run this step by hand once a free `make up` stack is available.
