## 1. Resolution core (`internal/searchintent`)

- [x] 1.1 Define the package's types — `Request` (text, optional profile, optional previous
  result), `Result` (facets, query, scalars, summary, unresolved) and `Profile` — plus the
  package doc comment that states the grounding rule: no value reaches the filter
  unresolved, and every drop is reported.
- [x] 1.2 Resolve closed-vocabulary facets against `internal/vocab`; drop and report a value
  outside its list. Refuse a facet name outside `search.StringFacets` outright.
- [x] 1.3 Resolve open-vocabulary facets — skills via `skilltag.Canonicalize`, countries and
  cities via `internal/location`, domains via `industrytag.Canonicalize`, companies via the
  company-slug rule and its alias registry. Drop and report what no dictionary places.
- [x] 1.4 Bound the scalars (salary minimum, posted-within days, maximum years of
  experience, visa flag) to the ranges the search filter accepts; drop and report the rest.
- [x] 1.5 Carry the free-text query through only when no facet expresses the concept, and
  keep it separate from the facets in the result.

## 2. Model call

- [x] 2.1 Build the prompt: the closed vocabularies inline from `internal/vocab`, the
  open-vocabulary facets described as "write ordinary words", and the free-text rule.
- [x] 2.2 Define the structured-output schema (facets, query, scalars, summary) and the
  single-call `Interpret` entry point that returns a resolved `Result`.
- [x] 2.3 ~~Seed an interpretation from a saved profile.~~ **Dropped, with reason:** the
  product already does this. `filtersFromProfile` (`web/src/lib/facetModel.ts`) is the
  pure client-side mapping behind "Apply my profile", and it is more careful than a
  re-implementation would be — it gates the home address on whether the person accepts
  on-site work, and lets a wanted skill win over an avoided one. A profile is written in
  the filter's own vocabulary, so this needs no model at all; building a second version
  server-side would have spent a model call to do worse, and left two sets of rules to
  diverge.
- [x] 2.4 Refine: accept the previous result as context and return a complete replacement,
  including when the new constraint contradicts the old one.

## 3. Endpoint (`internal/handler`)

- [x] 3.1 Add the `feature:search-intent` tag constant to `user_llm.go`.
- [x] 3.2 Add the per-caller limiter in the shape of `matchAnalysisLimiter`.
- [x] 3.3 Add the transport-only handler for `POST /api/v1/search/interpret` — require a
  session, cap the text length, bind the model client with `userLLM`, and report 503 when
  the model client is unconfigured. Register it beside the search routes.
- [x] 3.4 ~~Report the no-saved-profile case as a 404 naming the profile page.~~ **Dropped
  with 2.3:** the endpoint reads no profile at all, so the case cannot arise.
- [x] 3.5 ~~Document the endpoint in `openapi.yaml`.~~ **Dropped, with reason:**
  `web/static/openapi.yaml` documents the public, unauthenticated integration surface —
  jobs, companies, geo — and carries no cookie-authenticated route at all. This endpoint
  is cookie-only by design (see design.md), so an integrator reading the contract could
  not call it. Listing it would describe a capability the contract does not offer. If it
  is ever widened to accept an API key, it belongs there in the same change.

## 4. Sidebar entry point (`web/`)

- [x] 4.1 Add a `beforeButton` snippet to `FilterSummaryShell.svelte` and render
  `AiFilterButton.svelte` from `FilterSummary.svelte` only. The company summary passes
  nothing.
- [x] 4.2 Open the existing auth dialog on a signed-out activation, sending no request.
- [x] 4.3 Add the API client call for the interpret endpoint.

## 5. Dialog (`web/`)

- [x] 5.1 `AiFilterDialog.svelte` input state: a description box with example queries.
  (No profile tab — see 2.3; "Apply my profile" already exists and is a better answer.)
- [x] 5.2 Preview state: the summary sentence, the resolved values as chips grouped the way
  the sidebar groups them, and the not-recognised line when anything was dropped.
- [x] 5.3 The nothing-resolved state — its own message, with no apply action offered.
- [x] 5.4 Refine field: post the previewed result back as context and replace the preview
  with the new result.
- [x] 5.5 Apply: clear the filter store, then write the interpreted values through its
  published operations and nothing else. Dismissing discards the preview.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, and
  `go vet -tags=integration ./...` all pass.
- [x] 6.2 Web unit tests pass and the applied result writes exactly the interpreted values.
- [x] 6.3 Walked end to end against a locally-running server on the real gateway: described
  a search, refined it with a contradicting constraint, applied it, saw the chips and the
  URL, and confirmed the signed-out click opens the auth dialog. Four defects only that
  walk could find are fixed in the same change — see flexdecode.go and the prompt.
