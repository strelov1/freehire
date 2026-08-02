## 1. Source resolution

- [x] 1.1 Write failing tests for the ordered source list against a fake for each tier: base
  CV answers over a structured résumé stating different values; structured résumé answers
  when there is no base CV; a base CV stating no contact value at all is passed over; the
  chosen source answers for the whole block (a CV name plus no phone yields an empty phone,
  not the résumé's); tailored CVs are never a source.
- [x] 1.2 Implement the resolution: the first source that states at least one contact value
  wins, whole-block. Reads go through `cv.Store.BaseCV` and `resume.Store.Structured`.
- [x] 1.3 Write failing tests for the account-email backstop — no source at all yields email
  only; a source stating no email is completed with the account address — then implement it
  over `GetUserByID`.
- [x] 1.4 Delete the two `pool.QueryRow` calls and the hand `json.Unmarshal`; assert
  `internal/handler` holds no raw SQL (a test over the non-test sources of the package, so
  the rule is enforced by behaviour rather than by review).

## 2. Wiring

- [x] 2.1 Hoist `cv.NewStore(...)` to a `Register` local, pass it into `newCVHandlers`, and
  replace the `cvH.cvStore` reach-in at the referral construction site.
- [x] 2.2 Add `autofillHandlers` holding the CV store, the résumé store, the queries, the
  browser-tool hub and the LLM binding, with a `register` mounting
  `GET /me/autofill-profile` and `POST /me/autofill/run` behind `keyAuth`.
- [x] 2.3 Move `AutofillProfile` and `RunAgentAutofill` onto it; drop `llmBinding` from
  `API` and its two-phase assignment, keeping `browserTools` on `API` for `/tools/ws` and
  the assistant.
- [x] 2.4 Confirm the existing agent-autofill tests still pass unchanged — both entry points
  must go on sharing one assembly.

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && go test ./...`, then `gofmt -l` clean.
- [x] 3.2 Run the integration pass too (`go test -tags=integration ./internal/db/`) — the
  plain run does not see build-tagged tests.
- [x] 3.3 `openspec validate --strict` for this change.
- [ ] 3.4 After deploy, compare the two colours on production for a key-holding account with
  a structured résumé and no CV: the inactive colour returns email-only, the active one
  returns the parsed name and phone.
