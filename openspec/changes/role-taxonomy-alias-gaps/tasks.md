## 1. Agile/PM aliases (`project_management`)

- [x] 1.1 Add "agile coach" / "agile-coach" aliases → `project_management`
- [x] 1.2 Add "release train engineer" alias → `project_management`
- [x] 1.3 Add "agile transformation lead" / "agile transformation manager" aliases →
      `project_management`, placed ABOVE the terminal `{"manager","management"}` fall-through
- [x] 1.4 Add "scaled agile framework" / "safe practitioner" aliases → `project_management`
      (no bare "safe"; "safe scrum master" needs no entry — already resolves via the
      existing "scrum master" alias)

## 2. Security aliases (`security`)

- [x] 2.1 Add "iam" / "identity and access management" aliases → `security`
- [x] 2.2 Add "grc" alias → `security` (dropped the punctuated "governance, risk and
      compliance" long form — the matcher is literal-substring, so a comma/"&"-variant
      would silently never match; bare "grc" is unambiguous and does the real work); no
      bare "compliance"
- [x] 2.3 Add "vulnerability management" / "vulnerability analyst" aliases → `security`,
      placed ABOVE the terminal `{"analyst","data_analytics"}` fall-through
- [x] 2.4 Add "incident response" alias → `security`
- [x] 2.5 Add "red team" / "red teamer" / "blue team" aliases → `security`
- [x] 2.6 Add "penetration tester" / "penetration testing" / "pentester" / "pentest"
      aliases → `security`, placed ABOVE the QA block's bare "tester" fall-through
      (discovered during RED: "Penetration Tester" was being claimed by qa)
- [x] 2.7 Add "threat intelligence" / "threat intel" aliases → `security`
- [x] 2.8 Add "ciso" / "chief information security officer" aliases → `security`
- [x] 2.9 Add "devsecops" alias → `security`

## 3. Data/DevOps aliases (`data_engineering` / `data_analytics` / `devops`)

- [x] 3.1 Add "data platform" alias → `data_engineering`
- [x] 3.2 Add "data governance" alias → `data_engineering`, placed ABOVE the terminal
      `{"manager","management"}` fall-through (covers "Data Governance Manager")
- [x] 3.3 Add "data steward" alias → `data_engineering`
- [x] 3.4 Add "mlops" / "ml ops" aliases → `devops`
- [x] 3.5 Add "analytics engineer" alias → `data_analytics`, placed ABOVE the terminal
      `{"analyst","data_analytics"}` fall-through region so ordering stays consistent
- [x] 3.6 Add "platform engineering" alias (gerund/discipline form) → `devops`, alongside
      the pre-existing "platform engineer" alias

## 4. Classify tests

- [x] 4.1 Add a resolution test per new alias cluster (title → expected category) for all
      entries in tasks 1–3 (`TestParse_AliasGapFill`)
- [x] 4.2 Add fall-through-guard tests: "Vulnerability Analyst" not stolen by bare
      `analyst`→`data_analytics`; "Agile Transformation Manager" and "Data Governance
      Manager" not stolen by bare `manager`→`management`; "Penetration Tester" not stolen
      by qa's bare `tester`
- [x] 4.3 Add negative tests: bare "Safe Driving Instructor" does not resolve to
      `project_management`; bare "Customs Compliance Specialist" does not resolve to
      `security`

## 5. Skill-tag acronyms (SAFe/CSM/PSM/PMP)

- [x] 5.1 Add `project_management`-scoped acronyms CSM, PSM, PMP, SAFe (matched as the
      literal case-sensitive surface "SAFe", the framework's own stylization) to the
      category-scoped acronym allow-list in `internal/skilltag`, following the existing
      `RAG` (`ai_engineering`/`ml_ai`-scoped) pattern
- [x] 5.2 Add unscoped phrase aliases: "Certified ScrumMaster"/"Certified Scrum Master",
      "Professional Scrum Master", "Scaled Agile Framework", "Project Management
      Professional"
- [x] 5.3 Add skilltag tests: each acronym resolves only when category is
      `project_management`, does not resolve for other categories or with no category
      supplied; unscoped phrase forms resolve regardless of category
      (`TestParse_AgilePMCertificationAcronyms`)

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` all green
- [x] 6.2 `go vet -tags=integration ./...` compiles clean

## 7. Re-derive existing data

- [ ] 7.1 Run `go run ./cmd/backfill-derive` on host-2 to re-classify existing rows
- [ ] 7.2 Run `make reindex` to re-index the re-derived categories/skills (never stack with
      `reindex-companies`)
- [ ] 7.3 Spot-check a sample of the researched title clusters against
      `/api/v1/jobs/facets` and live `category=`/`skills=` filters post-reindex

## 8. OpenSpec validation

- [x] 8.1 `openspec validate role-taxonomy-alias-gaps --strict` passes
