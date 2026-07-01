## 1. Backend — resume skill extraction

- [x] 1.1 Add `github.com/ledongthuc/pdf` to `go.mod`/`go.sum` (`go get`), confirm `go build ./...`
- [x] 1.2 Implement resume text extraction in `internal/handler/resume.go`: a helper that takes a PDF `io.Reader` (+size) → text via `ledongthuc/pdf`, and a text path that passes through; unit tests with a small PDF fixture and a plain-text case
- [x] 1.3 Implement `ExtractResumeSkills` handler for `POST /api/v1/me/resume/extract`: dispatch by `Content-Type` (multipart `file` PDF vs JSON `{text}`), reject undecodable/empty input (400), run `skilltag.Parse`, respond `{"data":{"skills":[...]}}` (empty text→[]); oversize is handled by the server's global `BodyLimit` (413, framework-level); table-driven handler tests (PDF, text, empty→[], malformed→400)
- [x] 1.4 Wire the route under `RequireAuth` in `handler.Register`; assert unauthenticated → 401

## 2. Frontend — gap computation and API client

- [x] 2.1 Add pure `computeGap(marketSkills: string[], profileSkills: string[], n: number)` returning `{ expected, have, missing, coverage }` with `denominator = min(n, marketSkills.length)`; colocate with a small unit check
- [x] 2.2 Add `extractResumeSkills(input: File | string)` in `web/src/lib/api.ts` → `POST /me/resume/extract` (multipart for File, JSON for string), returns `string[]`
- [x] 2.3 Add a helper to fetch a profile's market skills: call `facetCounts` with the profile's specializations as `category` filters, return the `skills` facet sorted by count desc

## 3. Frontend — SearchProfilesView UI

- [x] 3.1 Add "Upload resume" control (PDF file input + paste-text affordance) with `idle → analyzing (disabled) → done` states; on success merge (union, dedup) extracted skills into the form's skills field without wiping existing entries
- [x] 3.2 Add the skill-gap block on each profile card (only when it has ≥1 specialization): show coverage `X/N`, a progress bar, and missing skills as chips; missing chips link to `/jobs?category=…&skills=…` (last, cuttable item)

## 4. Verify

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` green
- [x] 4.2 `svelte-check` clean for touched files; manual run of the upload → merge → gap flow (per project convention, web has no test runner)
