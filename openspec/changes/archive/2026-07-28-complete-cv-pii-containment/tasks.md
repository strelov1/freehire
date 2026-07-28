## 1. matchanalysis → structured-only

- [x] 1.1 Feed the fit chain the structured résumé with contact fields (`full_name/email/phone/links`) excluded, and stop sending raw CV: drop `writeCV`, make `writeStructured` the CV context, adjust the stage prompts/labels accordingly; tests assert the raw CV text never appears in any stage prompt and contacts are absent
- [x] 1.2 Remove the per-analysis PII machinery now that no raw CV is sent: delete `redact.go` (redactingEmit/restoreAnalysis/contactsFromStructured), drop the `detector` field + restore path, revert `NewAnalyzer` to `(client)`; update tests
- [x] 1.3 Degrade to no analysis when the structured résumé is absent/stale (no raw-CV fallback); test the no-structured path returns no analysis

## 2. atscheck → structured input

- [x] 2.1 Change `atscheck.Analyzer.Analyze` to take the de-identified structured résumé (contact-stripped) instead of raw `cvText`; build the review prompt from its highlights/summary/skills; tests assert no raw CV / no contacts in the prompt
- [x] 2.2 Update `handler/ats_report.go` to pass the stored structured résumé (contacts excluded) and degrade to the deterministic score when it is absent; update tests

## 3. Tailoring agent gets a contact-stripped CV

- [x] 3.1 Distinguish the tailoring key (agent) from the owner's cookie on the CV read/patch path (auth-context flag); test the flag is set correctly for each auth mechanism
- [x] 3.2 On a tailoring-key read of a CV, omit `Header.{full_name,email,phone}`; the owner's cookie read and the PDF render keep them; tests on both paths
- [x] 3.3 Reject a tailoring-key patch that targets `full_name`/`email`/`phone`; the stored value is unchanged; test

## 4. Wiring cleanup

- [x] 4.1 Drop the `PIIDetector` argument from `matchanalysis.NewAnalyzer` and `atscheck.NewAnalyzer` call sites in `handler`/`cmd`; `resumeextract` keeps its detector; `go build ./...` green

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` and `go vet -tags=integration ./...` green; confirm the only remaining raw-CV→LLM path is `resumeextract` (grep the `GenerateJSON*` callers)
