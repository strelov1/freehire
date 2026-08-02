## 1. The display projection

- [x] 1.1 Add the display shape and projection to `internal/applyform`: a pure function from a stored `Form` to the questions a reader sees, carrying the provider, each question's published text, whether it is required, and the word naming its answer kind
- [x] 1.2 Cover the exclusions: the standard name/email/phone/CV controls collapsed into one entry, every control the platform marked demographic dropped, and every non-question (`hidden`, `info`) dropped
- [x] 1.3 Cover the answer-kind vocabulary, including that an unnormalized kind yields no word while the question is still shown

## 2. The endpoint

- [x] 2.1 Add the query loading a job's stored form by job id, then `make sqlc`
- [x] 2.2 Add `GET /jobs/:slug/apply-form` following `JobCopies`: resolve the slug to an id, load the form, respond `{"data": ...}`; an unknown slug and a posting with no stored form both 404, distinguishably
- [x] 2.3 Integration-test the three cases against a real database: a posting with a form, a posting without one, an unknown slug

## 3. The page

- [x] 3.1 Add the API client call and wire it into `+page.server.ts` as a fourth parallel request, degrading to nothing on any failure like `similar` and `copies` already do
- [x] 3.2 Add the component rendering the questions. NOT unit-tested: vitest here runs in plain Node with no Svelte compilation (`vitest.config.ts`), so component tests are not a thing in this repo — the "renders nothing without a form" branch is verified visually in 3.3 instead
- [x] 3.3 Place it on the job page beside the apply action and check it visually against a posting that has a form

## 4. Finish

- [x] 4.1 Run `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, both Go test passes, and the web gates (`lint` and `check` catch different things)
- [x] 4.2 Note the new endpoint in `internal/handler/AGENTS.md` and the reader in `internal/applyform`'s package doc, so the store no longer reads as write-only
