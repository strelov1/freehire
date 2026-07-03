## Why

The verdict now scores a CV against live market demand, but treats every skill in
isolation and every gap as a hard binary miss. Mining the `career-ops` toolkit (a
MIT job-search project that analysed 895 real AI-engineering vacancies and
independently derived the same ≥50% must-have threshold we use) surfaced four
higher-signal, market-grounded ideas that make the verdict more actionable while
staying deterministic and dictionary-first.

## What Changes

- **Skill-bundle coverage:** the market expects skill *combinations*, not isolated
  skills (career-ops: GenAI+Ops appears in 72% of AI roles, pure-GenAI in 1.4%).
  Add a small **curated bundle dictionary** (genai-core, cloud-ops, web-stack,
  data, ml) and report, from the CV's parsed skills, which core bundles the
  candidate covers vs partially covers.
- **Transferable / adjacent skill status:** a missing role skill is often *adjacent*
  to one the candidate has (FastAPI ≈ REST APIs, PyTorch ≈ TensorFlow). Add a
  **curated adjacency dictionary** and a fourth skill status, `adjacent`, between
  `hidden` and `missing`: the candidate lacks the exact skill but holds a close one.
- **Actionable, typed gap advice:** enrich the per-skill advice so each status
  carries a concrete next step — `adjacent` names the close skill to reframe around;
  `hidden` says surface it; `missing` says learn + evidence.
- **ATS summary keyword-density check:** recruiters scan the summary in ~6 seconds,
  so a keyword-dense professional summary matters. Add a deterministic line item to
  the ATS Content/Section scoring that rewards a summary section carrying the CV's
  concrete skills.

All four are deterministic and dictionary-first (no LLM, no guessing), consistent
with `internal/location`/`skilltag`/`classify`.

## Capabilities

### New Capabilities
- `skill-bundles`: a curated dictionary grouping skills into market-recognised
  bundles, and the coverage of each bundle by a CV's parsed skills.

### Modified Capabilities
- `resume-verdict`: add per-skill `adjacent` status (curated adjacency), typed
  status advice, and the skill-bundle coverage rows to the verdict.
- `cv-ats-score`: add a deterministic summary keyword-density line item.

## Impact

- **Backend:** new `internal/skillbundle` (+ adjacency data — likely in `skilltag`
  or a small `internal/skilladjacent`); `internal/verdict` (`SkillRow.status` gains
  `adjacent`, new `Bundle` rows, advice); `internal/atscheck` (summary-density
  line item); handler `resume_verdict.go` (thread the CV `all` skill set for bundle
  coverage).
- **Contracts:** `cmd/gen-contracts` → `contracts.ts` (adds `Bundle`, extends the
  status vocabulary — additive).
- **Frontend:** `VerdictView.svelte` (adjacent badge colour + a compact bundle
  section), `ATSReportView.svelte` (unchanged shape; the new line item just appears).
- **No DB migration.**
