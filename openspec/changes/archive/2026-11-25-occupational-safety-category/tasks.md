## 1. The category joins the vocabulary

- [x] 1.1 Add a failing test in `internal/dict/vocab/vocab_test.go` asserting
      `occupational_safety` is a `CategoryValues` member, a `NonTechCategories` member
      and a `NonTechCraftCategories` member, and NOT a `TechCategories` member
- [x] 1.2 Add `occupational_safety` to the three lists in
      `internal/dict/vocab/vocab.go`, with a comment recording what the category means
      (the compliance/controls seat, sibling of `industrial_engineering` not a slice of
      it) and why craft membership is load-bearing (prune's business rule SUBTRACTS that
      set, so without it a retired oil & gas board deletes the employer's catalogue)
- [x] 1.3 Run `go test ./internal/dict/vocab/...` and confirm the existing partition and
      craft-subset assertions stay green alongside the new test

## 2. HSE titles resolve the category

- [x] 2.1 Add failing tests in `internal/dict/classify/classify_test.go` for the acronym
      spellings → `occupational_safety`: `EHS Specialist`, `HSE Officer`,
      `HSEQ Advisor`, `QHSE Coordinator`, `HSSE Manager`, `SHES Coordinator`
- [x] 2.2 Add failing tests for the spelled-out and Russian forms:
      `Environmental Health and Safety Specialist`, `Health & Safety Advisor`,
      `Occupational Health and Safety Manager`, `Инженер по охране труда`
- [x] 2.3 Add failing tests for the SOC reported titles: `Safety Coordinator`,
      `Safety Specialist`, `Safety Officer`, `Industrial Hygienist`,
      `Risk Control Consultant`, and `Safety Engineer` (which moves off
      `industrial_engineering`)
- [x] 2.4 Add the alias block to `internal/dict/classify/dictionaries.go` placed ABOVE
      the bare `{"manager", "management"}` entry, and remove the now-superseded
      `{"safety engineer", "industrial_engineering"}` line. Comment records the
      precedence reason and the SOC/ISCO placement argument
- [x] 2.5 Run `go test ./internal/dict/classify/...` and confirm green with no existing
      test regressing

## 3. The block outranks the bare manager fall-through

- [x] 3.1 Add failing tests asserting `EHS Manager`, `HSE Manager`,
      `Regional EHS Manager` and `Senior EHS Manager` → `occupational_safety`, not
      `management`
- [x] 3.2 Add a regression test asserting a manager title with no recognised function
      (e.g. `Business Manager`) still resolves `management`, unchanged
- [x] 3.3 Confirm the placement from 2.4 satisfies both; adjust ordering if not
- [x] 3.4 Run `go test ./internal/dict/classify/...` and confirm green

## 4. Lookalike words stay out

- [x] 4.1 Add failing regression tests asserting NO category resolves
      `occupational_safety` for `Software Engineer (she/her)`, `Risk Analyst`,
      `Credit Risk Manager`, `Patient Safety Attendant` and `Public Safety Officer`
- [x] 4.2 Add failing tests asserting the qualified SHE forms DO resolve:
      `SHE Manager`, `SHE Advisor`
- [x] 4.3 Confirm the dictionary carries no bare `she`, `risk` or `safety` alias, with a
      comment beside the qualified forms naming the live-title collisions that keep the
      bare words out
- [x] 4.4 Run `go test ./internal/dict/classify/...` and confirm green

## 5. The deletion veto

- [x] 5.1 Add a failing test in `internal/dict/classify/nontech_test.go` asserting
      `ConfirmedNonTech("Инженер по охране труда", false)` is false — the title matches
      the existing `"охране труда"` non-tech term, and the veto must spare it
- [x] 5.2 Add a failing test asserting `ConfirmedNonTech("EHS Specialist", false)` is
      false
- [x] 5.3 Add a regression test asserting the veto does NOT widen: a title that resolves
      no category (e.g. `HVAC Technician`, `Warehouse Janitorial Cleaner`) is still
      confirmed non-technical
- [x] 5.4 Widen the `ConfirmedNonTech` veto in `internal/dict/classify/nontech.go` to
      the craft categories rather than the single `engineering_design` name, so a third
      craft category cannot be added later and silently miss it
- [x] 5.5 Check the prune business rule's own exclusion path reads
      `NonTechCraftCategories` (not a category named inline) and covers the new member;
      fix it if it does not
- [x] 5.6 Run `go test ./internal/dict/classify/... ./cmd/prune/...` and confirm green

## 6. The skill vocabulary

- [x] 6.1 Add failing tests in `internal/dict/skilltag` for the core mined terms:
      `osha`, `iso-45001`, `iso-14001`, `emergency-response`, `root-cause-analysis`,
      `risk-assessment`, `incident-investigation`, `industrial-hygiene`, `epa`,
      `environmental-compliance`, `safety-management-system`, `corrective-action`,
      `personal-protective-equipment`
- [x] 6.2 Add failing tests for the secondary terms: `waste-management`, `first-aid`,
      `hazardous-waste`, `toolbox-talks`, `safety-audit`, `hazard-identification`,
      `contractor-safety`, `lockout-tagout`, `confined-space`, `nfpa`,
      `fall-protection`, `machine-guarding`, `hot-work`, `near-miss-reporting`,
      `process-safety-management`, `hazop`, `permit-to-work`, `job-safety-analysis`,
      `rcra`, `behavior-based-safety`, `working-at-height`
- [x] 6.3 Add failing tests for the EHS platforms: `enablon`, `intelex`, `velocityehs`,
      `sphera`, `cority`
- [x] 6.4 Add the terms and their aliases to `internal/dict/skilltag/dictionaries.go`
      and their display strings to `labels.go`, each comment carrying the measured
      document frequency from the mined corpus
- [x] 6.5 Run `go test ./internal/dict/skilltag/...` and confirm green

## 7. Acronym collisions

- [x] 7.1 Add a failing test asserting `BBS` resolves Behaviour-Based Safety ONLY when
      the caller supplies the `occupational_safety` category, and resolves nothing under
      `software_engineering`
- [x] 7.2 CSP/CIH/CHMM do NOT go here: skilltag's own invariant rejects a scoped acronym
      that would create a new canonical, so they moved to the certification dictionary
      in group 8 and the skilltag test asserts they resolve nothing here
- [x] 7.3 Add a regression test asserting `PSM` under `project_management` still resolves
      `professional-scrum-master`, unchanged
- [x] 7.4 Add a failing test asserting the spelled-out `process safety management`
      resolves, and that no HSE term resolves from text containing only "aspects",
      "aspiring" or the word "dot"
- [x] 7.5 Add the four scoped acronyms to `categoryScopedAcronyms` and the spelled-out
      phrase to the term table, with a comment recording why `PSM` is phrase-only (the
      map holds one canonical per key; acronym and phrase each measured at 2%)
- [x] 7.6 Run `go test ./internal/dict/skilltag/...` and confirm green

## 8. Certifications

- [x] 8.1 Add failing tests in `internal/dict/certification/certification_test.go` for
      `nebosh`, `iosh`, `csp`, `cih`, `chmm` and `hazwoper`
- [x] 8.2 Add the credentials and their aliases to
      `internal/dict/certification/certification.go`
- [x] 8.3 Run `go test ./internal/dict/certification/...` and confirm green

## 9. Serving surfaces

- [x] 9.1 Run `cmd/gen-contracts` to regenerate `web/src/lib/generated/contracts.ts`
      with the new category
- [x] 9.2 Add `occupational_safety: 'Health & Safety (HSE)'` to `CATEGORY_LABELS` in
      `web/src/lib/labels.ts` and to `extension/lib/labels.ts`
- [x] 9.3 Add `occupational_safety: 'Quality & Security'` to `CATEGORY_GROUP` in
      `web/src/lib/filterSections.ts`, with a comment recording why that group (QHSE and
      HSEQ bundle Quality with HSE in the profession's own naming) rather than
      `Engineering`
- [x] 9.4 Run the web type-check and `web/src/lib/labels.test.ts`; confirm the
      exhaustive `Record<Category, …>` types are satisfied and the suite is green
- [x] 9.5 Run the extension lint/build and confirm green

## 10. Coverage measured against live titles, not the list

- [x] 10.1 Extend the classify corpus probe (`corpus_probe_test.go`) or add an
      equivalent so coverage of this family is measured against real titles rather than
      against the same list that produced the dictionary
- [x] 10.2 Record what the probe reports — which live HSE spellings still resolve
      nothing — in the change, so the gap is visible rather than assumed closed

## 11. Finish

- [x] 11.1 Run the `simplify` skill over the whole diff
- [x] 11.2 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and the web suite; confirm
      clean, and name any pre-existing unrelated failure explicitly
- [x] 11.3 Request and act on one review pass over the whole diff
- [x] 11.4 Record the operational follow-up in the change: `cmd/backfill-derive` at
      `BACKFILL_CONCURRENCY` 2–3, then `systemctl stop freehire-reindexw.timer` before
      `cmd/reindex`. Verify by facet count on the live site, not by unit test
