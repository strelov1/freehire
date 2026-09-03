## 1. Profile parsing (pure, no network)

- [x] 1.1 Create `internal/candidate/linkedinprofile` and register it in the `candidate` list in `internal/platform/arch/layering/blocks.go`; confirm `go test ./internal/platform/arch/layering/` passes with the new package present
- [x] 1.2 Implement masked-value detection: a string whose non-separator characters are all asterisks is absent. Cover the real shapes from the spike — `"****** ******** ********"`, `"**********"` — plus the negatives (`"C**"`, `"3*3"`, empty, ordinary text)
- [x] 1.3 Implement `Person`-node extraction from a page's `application/ld+json`: walk `@graph`, take the `Person` node, lift `name`, `description`, `address`, `worksFor[0].name`/`location`, `knowsLanguage`, running every lifted string through 1.2. Save the spike's real profile HTML as a testdata fixture and assert against it
- [x] 1.4 Cover the parse failure paths: no `ld+json` block, `ld+json` that is not valid JSON, a `@graph` with no `Person` node, a `Person` whose every field is masked — each yields a typed "could not read" outcome, never a partial struct with asterisks in it

## 2. Fetching (network, guarded)

- [x] 2.1 Implement URL validation ahead of any request: accept `linkedin.com` / `www.linkedin.com` / `<cc>.linkedin.com` with an `/in/<public-id>` path, accept a bare public id by expanding it, reject everything else (company URLs, other hosts, non-profile paths) with a typed error
- [x] 2.2 Implement the fetch over `safehttp`'s guarded client: no `Cookie`, no `Authorization`, a request timeout, and a body read capped at a fixed byte limit where exceeding the cap is a failure rather than a truncation
- [x] 2.3 Cover fetch failures against a test server: non-200, a body past the cap, a timeout — each surfaces as a typed import failure distinguishable in logs from a rejected URL

## 3. Endpoint

- [ ] 3.1 Add the handler for `POST /api/v1/me/linkedin/import`: validate + fetch + parse via `linkedinprofile`, then derive facets by calling the existing `resumeProfile(...)` (`internal/api/handler/resume.go`) with the headline and `location.Parse` with the address
- [ ] 3.2 Assert the response shape matches `/me/resume/extract`'s (`skills` and `categories` always arrays, `seniority` omitted when unresolved) and additionally carries the derived location and the display fields (name, headline, company)
- [ ] 3.3 Register the route behind cookie auth next to `/me/resume/extract`, and assert an anonymous caller is refused **before** any outbound request is made
- [ ] 3.4 Assert the import stores nothing: no CV is written, CV presence is unchanged, and no profile row is touched
- [ ] 3.5 Add per-account rate limiting and assert a throttled call makes no outbound request

## 4. Wizard UI

- [ ] 4.1 Add the client call and its response type (`web/src/lib/api.ts`, `web/src/lib/types.ts`)
- [ ] 4.2 Add the URL field beside the existing dropzone on the CV step of `web/src/routes/onboarding/+page.svelte`, with a pending state during the fetch
- [ ] 4.3 Merge imported values into the staged confirm/location values rather than replacing them; a field the import resolves nothing for leaves its staged value untouched
- [ ] 4.4 Render the permanent disclosure — work history is not imported, and `More → Save to PDF` into the dropzone beside it is how to bring it in — visible before any interaction
- [ ] 4.5 Handle the two non-success outcomes distinctly: a failed import shows an error and changes nothing staged; a successful import that resolved nothing reuses the CV step's existing "couldn't read details" wording
- [ ] 4.6 Assert the step stays skippable and that importing does not navigate the user off `/onboarding`

## 5. Published contract

- [ ] 5.1 Add the endpoint to `web/static/openapi.yaml` and regenerate `docs/API.md`; both carry no ratchet, so they must land in this change

## 6. Verification

- [ ] 6.1 `gofmt -l .` prints nothing; `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...` pass
- [ ] 6.2 Run the wizard against the real profile from the spike end to end and confirm the confirm/location steps pre-fill; confirm no asterisk string reaches any field
- [ ] 6.3 From the production host, confirm LinkedIn answers the plain fetch — if it does not, record it and resolve the proxy-egress open question before shipping the UI entry point
