## 1. The registry

- [x] 1.1 Write the failing test for `layoutFor`: Greenhouse resolves to `("application-form", "#submit_app", byID)`, Lever to `("application-form", "#btn-submit", byName)` — the values measured on `jobs.lever.co/coderio/.../apply` on 2026-09-09 — and a platform with no fill path (ashby, workable, recruitee, the empty string) resolves to nothing.
- [x] 1.2 Write the containment test: every entry in `fillProviders` has a layout. It passes trivially now and becomes load-bearing in 4.1; its purpose is that the two sets live in different files and a platform added to one and not the other would reach a submit click with selectors matching nothing.
- [x] 1.3 Create `internal/api/atsapply/layout.go` with `addressing` (`byID`, `byName`), `formLayout{formSelector, submitSelector, addressBy}`, the `layouts` table and `layoutFor`. The doc comments must carry WHY a table of measured values rather than heuristics — a submit click cannot be withdrawn, and this package has twice acted on an inference about a page nobody had loaded.

## 2. Scanning by the layout

- [ ] 2.1 Capture the fixture: reduce the live DOM of `jobs.lever.co/coderio/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037/apply` to its form controls, verbatim attributes, into `internal/api/atsapply/leverform_test.go`. It must show what Lever does NOT have: no input carries an `id`.
- [ ] 2.2 Write the failing tests over that fixture: Lever's fields are identified by `name` (including `urls[LinkedIn]` and the file input `resume`); the location control, which has BOTH an id and a differing name, is identified by its name; a Greenhouse fixture is still identified by `id`; and a `byName` control with no name is dropped rather than given a synthetic key.
- [ ] 2.3 Replace `ScanGreenhouseForm` with `ScanForm(pageHTML string, layout formLayout)`, threading the addressing through `scanControls`, `scanInput` and `scanSimple`. Set `DOMField.ID` from whichever attribute the layout addresses by, keeping `Name` as the raw attribute, so everything downstream keeps working on one identifier.
- [ ] 2.4 Delete `greenhouseFormReadySelector`; its value is now `layout.formSelector`, passed by the callers. Update both call sites (`client.go`, `preview_client.go`) enough to build.
- [ ] 2.5 Confirm the pre-existing Greenhouse scan tests pass UNMODIFIED. A test that needed editing means behaviour moved for the platform that already worked — stop and say so rather than editing it.

## 3. Selecting and submitting by the layout

- [ ] 3.1 Write the failing test for `fieldSelector(id, by)`: `byID` gives `#first_name`, `byName` gives `[name="name"]`, and a bracketed name (`urls[LinkedIn]`) survives quoted intact — brackets are selector syntax when unquoted.
- [ ] 3.2 Change `fieldSelector` to take the addressing, and derive chromedp's query kind from the same value (`ByID` or `ByQuery`) in the same place, so the selector and the way it is interpreted cannot disagree.
- [ ] 3.3 Replace the hard-coded `greenhouseSubmitSelector` click in `fillAndSubmit` with `layout.submitSelector`, and delete the constant. Record in the comment that Lever's `#btn-submit` is a `type="button"` whose own JS runs the invisible hCaptcha and then clicks the real hidden control — we click what a person clicks and do not model the rest.

## 4. Turning Lever on

- [ ] 4.1 Write the failing test asserting `fillProviders["lever"]`, then add the entry, with a comment naming what was measured to justify it (the live preview came back fully resolved; the form's controls were captured 2026-09-09).
- [ ] 4.2 Replace `if claimed.Provider == "greenhouse"` in `client.go` with a `layoutFor` lookup, passing `layout.formSelector` to `renderedHTML` and `layout` to `ScanForm` and `fillAndSubmit`.
- [ ] 4.3 Make the same change in `preview_client.go`'s own Greenhouse branch — the preview must scan Lever's DOM too, or the candidate's preview and the actual submission would disagree about what the form contains.
- [ ] 4.4 Verify the containment test from 1.2 now fails when `lever` is removed from either the registry or `fillProviders`, and passes with both. This is the only step that proves it holds weight.

## 5. The package's account of itself

- [ ] 5.1 Rewrite the statements in `internal/api/atsapply/AGENTS.md` that are now false: the platform list, the "one provider with a live DOM-scan" framing, and any mention of `ScanGreenhouseForm` or `greenhouseSubmitSelector`.
- [ ] 5.2 State what the code cannot: that a layout's three values are measured off a real posting and never inferred, naming the two failures that taught it; that addressing is one setting because a mismatch is silent; and that an unconfirmed submission is never retried because a duplicate application costs the candidate more than a missing one.
- [ ] 5.3 Run `pnpm check:links` — every relative link in the file must resolve.

## 6. Verification

- [ ] 6.1 Run `go build ./... && go vet ./... && go test ./... && go vet -tags=integration ./...` — all clean.
- [ ] 6.2 Replay `ScanForm` against a freshly captured live Lever DOM on the production host, confirming the field inventory matches what the fixture asserts. A fixture that agrees only with itself is what this package was burned by twice this week.
