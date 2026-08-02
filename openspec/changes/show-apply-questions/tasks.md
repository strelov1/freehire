## 1. The display projection

- [ ] 1.1 Add the display shape and projection to `internal/applyform`: a pure function from a stored `Form` to the questions a reader sees, carrying the provider, each question's published text, whether it is required, and the word naming its answer kind
- [ ] 1.2 Cover the exclusions: the standard name/email/phone/CV controls collapsed into one entry, every control the platform marked demographic dropped, and every non-question (`hidden`, `info`) dropped
- [ ] 1.3 Cover the answer-kind vocabulary, including that an unnormalized kind yields no word while the question is still shown

## 2. The endpoint

- [ ] 2.1 Add the query loading a job's stored form by job id, then `make sqlc`
- [ ] 2.2 Add `GET /jobs/:slug/apply-form` following `JobCopies`: resolve the slug to an id, load the form, respond `{"data": ...}`; an unknown slug and a posting with no stored form both 404, distinguishably
- [ ] 2.3 Integration-test the three cases against a real database: a posting with a form, a posting without one, an unknown slug

## 3. The page

- [ ] 3.1 Add the API client call and wire it into `+page.server.ts` as a fourth parallel request, degrading to nothing on any failure like `similar` and `copies` already do
- [ ] 3.2 Add the component rendering the questions, and cover that it renders nothing when there is no form
- [ ] 3.3 Place it on the job page beside the apply action and check it visually against a posting that has a form

## 4. Finish

- [ ] 4.1 Run `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, both Go test passes, and the web gates (`lint` and `check` catch different things)
- [ ] 4.2 Note the new endpoint in `internal/handler/AGENTS.md` and the reader in `internal/applyform`'s package doc, so the store no longer reads as write-only
