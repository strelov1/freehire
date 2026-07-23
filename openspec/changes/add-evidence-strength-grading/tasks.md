## 1. Wire contract & vocabulary

- [x] 1.1 Add `EvidenceStrength string json:"evidence_strength"` to `Requirement` in `internal/matchanalysis/matchanalysis.go`.
- [x] 1.2 Add the ordered vocabulary constants (`StrengthMetric`, `StrengthScope`, `StrengthResponsibility`, `StrengthKeyword`) and a `validEvidenceStrength` map next to `validStatus`.
- [x] 1.3 Add a `coerceEvidenceStrength(status, strength string) string` helper: positive status → lower-case + coerce unknown/empty to `keyword`; `missing-*` status → `""`.

## 2. Sanitize

- [x] 2.1 In `sanitizeRequirements`, set `EvidenceStrength: coerceEvidenceStrength(status, r.EvidenceStrength)` on the emitted requirement.
- [x] 2.2 Unit-test the coercion: `metric`/`scope`/`responsibility`/`keyword` pass through on positive statuses; unknown/empty → `keyword`; any value on `missing-have`/`missing-gap` → `""`.

## 3. Stage-1 prompt

- [x] 3.1 Extend the Stage-1 requirement instruction in `analyzer.go` to request `evidence_strength` on `covered`/`synonym-only` items, defined by the four tiers and grounded in the cited `evidence`.
- [x] 3.2 Prompt test asserting the Stage-1 prompt names `evidence_strength` and the four tiers.

## 4. Stage-3 audit

- [x] 4.1 Broaden the Stage-3 system-prompt demotion sentence: for `required` requirements, a `synonym-only` match OR a `covered` match graded `keyword` is thin support and must not alone sustain a high `skills_coverage` score.
- [x] 4.2 Update `writeRequirements` to render `- [priority/status/strength] text` for positive statuses (status-only for `missing-*`), so Stage 2 and Stage 3 both see the grade.
- [x] 4.3 Prompt test asserting Stage-3 mentions keyword-strength demotion and that `writeRequirements` renders the strength.

## 5. Contracts & frontend

- [x] 5.1 Regenerate the TS wire shape via `cmd/gen-contracts` (`evidence_strength` on the requirement type).
- [x] 5.2 Render a small per-requirement strength cue for positive statuses in `web/src/routes/match/[slug]/` (no cue for `missing-*`).

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./internal/matchanalysis/`.
- [x] 6.2 `openspec validate add-evidence-strength-grading`.
- [x] 6.3 Drive one real analysis (or the SSE reducer test) end-to-end to confirm the field flows to the served payload and the match page renders the cue.
