## Why

Live prod-DB research (open jobs, `closed_at IS NULL`) found ~9,700 open postings whose
title clearly names an IT sub-role that already has a home category — `project_management`,
`security`, `devops`, `data_engineering`, `data_analytics` all exist and are well populated —
but the title-alias dictionary in `internal/classify` never learned the specific phrase, so
the job falls through to an empty (unclassified) category. Examples: "Agile Coach" (8.5%
categorized), "IAM Engineer" (42.6%), "MLOps Engineer" (47.6%), "Analytics Engineer" (28.1%),
"DevSecOps" (28.1%). These are not new disciplines needing new categories (unlike the recent
`expand-role-taxonomy` change) — they are known disciplines whose common title phrasing was
simply never added as an alias. Filling the gap makes category/skill filters usable for
candidates and companies in these sub-roles without touching the category vocabulary itself.

## What Changes

- Add title aliases to `internal/classify/dictionaries.go` for ~20 IT sub-role phrases,
  routed to their existing category, each placed ABOVE the terminal bare fall-throughs
  (`{"analyst","data_analytics"}`, `{"manager","management"}`, `{"engineer",...}` if any)
  per the alias-ordering doctrine established by `expand-role-taxonomy`:
  - `project_management`: "agile coach", "release train engineer", "agile transformation
    lead/manager", "scaled agile framework", "safe practitioner", "safe scrum master" (NOT
    bare "safe" — common English word, false-positive risk; NOT bare "compliance" — sampled
    live titles, dominated by non-IT banking/legal/customs compliance, a GTM-style word-trap)
  - `security`: "identity and access management"/"iam", "governance, risk and compliance"/
    "grc", "vulnerability management"/"vulnerability analyst", "incident response", "red
    team(er)", "blue team", "penetration tester"/"pentest", "threat intelligence"/"threat
    intel", "ciso"/"chief information security officer", "devsecops"
  - `data_engineering`: "data platform", "data governance", "data steward", "mlops"/"ml ops"
  - `data_analytics`: "analytics engineer"
  - `devops`: additional "platform engineer" phrasing not yet covered (8.5% of its own
    volume still falls through despite 92%+ coverage today)
- Add the missing agile/PM certification acronyms to `internal/skilltag` — SAFe, CSM
  (Certified ScrumMaster), PSM (Professional Scrum Master), PMP (Project Management
  Professional) — using the existing category-scoped acronym mechanism (already used for
  `RAG`) so the ambiguous bare forms only resolve on job text already categorized
  `project_management`, never elsewhere.
- Re-derive existing prod rows: `cmd/backfill-derive` then `make reindex`, following the
  same playbook `expand-role-taxonomy` used (tasks 7.2–7.4 there, not yet run).

## Capabilities

### New Capabilities
- `role-category-alias-coverage`: the coverage contract for title-alias phrasing within
  already-existing role categories — which sub-role clusters the classify dictionary must
  resolve, the alias-ordering doctrine that keeps them from being stolen by generic
  fall-throughs, and the word-trap exclusions (bare "safe", bare "compliance") kept out
  deliberately.

### Modified Capabilities
- (none — `skill-tag-matching`'s category-scoped acronym mechanism already covers adding
  new acronyms like SAFe/CSM/PSM/PMP as data, not a new normative rule; `tech-classification`
  is unaffected since no category's tech/non-tech partition membership changes)

## Impact

- **Code**: `internal/classify/dictionaries.go` (alias table), `internal/classify/dictionaries_test.go`
  or equivalent (fall-through-guard tests), `internal/skilltag/dictionaries.go` (category-scoped
  acronyms), `internal/skilltag/skilltag_test.go`.
- **Data/ops**: `go run ./cmd/backfill-derive` then `make reindex` (never stack with
  `reindex-companies`) to re-classify and re-index the ~9,700 affected existing rows.
- **Not in scope**: new category values, changes to the tech/non-tech partition, the
  `compliance`/bare-`safe` word-traps identified and deliberately rejected during research.
