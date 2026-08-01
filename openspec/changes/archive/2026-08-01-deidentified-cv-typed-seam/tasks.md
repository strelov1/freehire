## 1. The proof test — a new field must reach no model

- [x] 1.1 RED: add a test asserting that a `resumeextract.Structured` carrying a contact-bearing
      field NOT named by `Professional` never appears in the ATS review's user prompt. Write it
      against the seam as it will be, so it fails on today's `stripContacts`.
- [x] 1.2 RED: the same assertion for the fit chain's candidate context, which passes today —
      it pins the behaviour the ATS path is being brought up to, and must keep passing.
- [x] 1.3 Confirm both tests state that today's byte output is unchanged: the projection's
      complement is exactly `full_name`, `email`, `phone`, `links`.

## 2. Type the ATS seam

- [x] 2.1 GREEN: `atscheck.Analyze(ctx, resumeextract.Professional)`; `reviewUserPrompt` marshals
      and truncates. Delete `stripContacts` and every test that exercised it directly.
- [x] 2.2 Replace `handler.structuredResumeJSON` with a reader returning
      `(resumeextract.Professional, bool)`, and have `PostATSReport` skip the model call when
      `!ok` — next to the other "serve the deterministic report" exits.
- [x] 2.3 Check `atscheck`'s remaining nil-client guard still reads true, and that its doc
      comment no longer claims the analyzer strips anything.

## 3. Type the fit seam

- [x] 3.1 `matchanalysis.Input.StructuredResume` becomes `resumeextract.Professional`; update the
      field's doc comment to say the type carries the guarantee.
- [x] 3.2 Collapse `candidateContext` to marshal + `TruncateRunes`, deleting the unmarshal and
      the second `.Professional()` — a provable no-op once the field is typed.
- [x] 3.3 Update the two producers (`match_analysis.go`, `match_analysis_stream.go`) and
      `candidateProfileJSON`, which already builds a `Professional` and now returns it.
- [x] 3.4 Decide what "no analysis" looks like now that the empty string is gone — the chain's
      "empty candidate ⇒ no analysis" rule must keep firing on an empty bank.

## 3b. The fourth path, found while implementing

- [x] 3b.1 `buildHardConstraintInputs` takes `resumeextract.Professional`. Its blockers carry
      their reasons into the stage-1 prompt, so the CV side of a hard constraint is model-bound
      content and takes the same seam. `hardconstraint.CVEvidence` already reads only fields the
      projection carries, so nothing changes but the type.

## 4. Documentation

- [x] 4.1 Update `internal/matchanalysis/AGENTS.md`, which calls the field "the resumeextract wire
      shape (contact fields stripped)" — documented as a type it was not.
- [x] 4.2 Check `internal/atscheck` and `internal/resumeextract` docs for the same claim, and
      point the whitelist rationale at the seam now that it has one.

## 5. Verify and close

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 5.2 Confirm no caller can still hand a contact-bearing value to either model: grep for
      `Structured` in the two analyzer packages and their handler call sites.
- [x] 5.3 Mark S8 ✅ in `docs/reviews/2026-08-01-architecture-review.md` — shortlist row, the
      `S8` heading and the Progress table — noting anything the finding got wrong.
