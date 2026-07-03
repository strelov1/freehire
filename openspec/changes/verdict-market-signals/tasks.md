## 1. Skill bundles (new pure package)

- [x] 1.1 Add `internal/skillbundle` — curated bundle dict (genai-core, cloud-ops, web-stack, data, ml) + `Coverage(cvSkills) []Bundle{name,label,covered,total,covered_bool}`; `BundleCoveredPct` const
- [x] 1.2 Table-driven tests: full/partial/threshold coverage, determinism

## 2. Adjacent status + advice (verdict)

- [x] 2.1 Curated adjacency dictionary (roleSkill → close candidate skills), conservative AI/backend seed
- [x] 2.2 Add `StatusAdjacent`; extend `classify` precedence strong→hidden→adjacent→missing; adjacent excluded from must-have/stack counts; carry the matched close skill
- [x] 2.3 Typed advice: adjacent names the close skill (reframe/ramp), hidden=surface, missing=learn+evidence
- [x] 2.4 Add `Bundle` rows to `Verdict` (thread CV `all` skills through `Compute`)
- [x] 2.5 Table-driven `verdict` tests: adjacent classification, adjacent not inflating coverage, advice text, bundle rows

## 3. ATS summary keyword-density (atscheck)

- [x] 3.1 Summary-section extractor + a keyword-density line item; re-balance the host category's item weights so Σmax stays 100
- [x] 3.2 `atscheck` tests: dense summary passes, generic summary recoverable, Σmax == 100

## 4. Contracts + frontend

- [ ] 4.1 Regenerate TS contracts (`Bundle`; status vocab additive)
- [ ] 4.2 `VerdictView.svelte`: `adjacent` badge colour + advice; compact "Skill bundles" section (covered vs partial)
- [ ] 4.3 `svelte-check` clean + `vite build` green

## 5. Verify

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` green
- [ ] 5.2 Calibrate bundle/adjacency seeds against the real CV (throwaway harness); confirm sane output
