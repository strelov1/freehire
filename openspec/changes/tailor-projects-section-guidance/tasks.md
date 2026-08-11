## 1. Agent guidance

- [x] 1.1 Add tailor-prompt bullets: section headings are template-owned; do not invent heading/title/section fields; portfolio/side projects go in `projects[]` (`projects[i].…`), not `experience[]`
- [x] 1.2 Update `cv_edit` tool Description to mention `projects[i]` paths for portfolio work and that templates render the Projects heading when the array is non-empty
- [x] 1.3 Unit tests asserting the tailor prompt (and/or tool description) contains the heading-ownership and projects-placement guidance

## 2. Base seed / projects placement

- [x] 2.1 Verify bankedSeeder / structure seed already maps projects into `projects[]`; fix any gap that leaves base `projects` empty when structure or bank has portfolio entries
- [x] 2.2 Seed unit tests covering structure and/or bank project → base `projects` (extend existing seed tests if present)
- [x] 2.3 Optional ops check: if local base still lacks Sandrock while tailored copies have it and bank/structure lack it, document that the agent (or user) must add via bank/`cv_edit` on base — no silent invent

## 3. Verification

- [x] 3.1 `go test` for affected packages; `go vet -tags=integration ./...`
