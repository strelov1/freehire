# Lever Fill and Submit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let auto-apply fill and submit a Lever application, the second provider after Greenhouse.

**Architecture:** A small per-provider registry replaces the four places the provider is currently hard-coded — the form's element id, the submit button's selector, and how a field is identified and selected (`id` on Greenhouse, `name` on Lever). Everything else in the pipeline is already provider-agnostic and does not change.

**Tech Stack:** Go 1.26, chromedp (headless Chrome), `golang.org/x/net/html`.

## Global Constraints

- **English only** in all code, comments, identifiers, docs and commit messages.
- Before committing any `*.go`: `gofmt -w` those paths, then `go vet ./...` and `go test ./...`. Before pushing: `go vet -tags=integration ./...`.
- Pre-commit hooks (gofmt, gitleaks, doc-links, go vet, golangci-lint) run automatically. If one fails, fix the cause — never `--no-verify`.
- The Go build cache is shared and has been observed corrupting mid-build (`no such file or directory` under `~/Library/Caches/go-build`). On that, `export GOCACHE=/tmp/gocache-lv` and retry.
- Use `git commit -F <file>` for multi-line messages, never `-m` with backticks.
- **A submit click is irreversible.** Nothing in this plan may make one more likely on a page nobody measured. When a value is unknown, park.
- **Fixtures must be reduced from real captured DOM**, never hand-written to match the code. A hand-written fixture passed while the live page failed, twice, in this package this week.
- `internal/api/atsapply` is layer 8 (`api`); it may import `dict` and `application` but nothing may import it back.

---

## File Structure

**Create:**
- `internal/api/atsapply/layout.go` — the provider registry: the `formLayout` type, the `addressing` values, the table, and its lookup. One responsibility, no I/O.
- `internal/api/atsapply/layout_test.go`
- `internal/api/atsapply/leverform_test.go` — the reduced real Lever DOM fixture and the scan/selector tests over it.

**Modify:**
- `internal/api/atsapply/domscan.go` — `ScanGreenhouseForm` becomes layout-driven; `scanControls` honours the addressing.
- `internal/api/atsapply/fill.go` — `fieldSelector` and the submit click read the layout.
- `internal/api/atsapply/client.go` — `fillProviders` gains `lever`; the `if provider == "greenhouse"` branch becomes a layout lookup.
- `internal/api/atsapply/preview_client.go` — its own Greenhouse branch, same change.

---

### Task 1: The provider registry

**Files:**
- Create: `internal/api/atsapply/layout.go`
- Create: `internal/api/atsapply/layout_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type addressing int` with `byID` and `byName`
  - `type formLayout struct { formSelector, submitSelector string; addressBy addressing }`
  - `func layoutFor(provider string) (formLayout, bool)`

- [ ] **Step 1: Write the failing test**

Create `internal/api/atsapply/layout_test.go`:

```go
package atsapply

import "testing"

func TestLayoutFor_KnowsTheTwoProvidersWithAFillPath(t *testing.T) {
	gh, ok := layoutFor("greenhouse")
	if !ok {
		t.Fatal("greenhouse has no layout")
	}
	if gh.formSelector != "application-form" || gh.submitSelector != "#submit_app" || gh.addressBy != byID {
		t.Errorf("greenhouse layout = %+v, want the ids measured on its own pages", gh)
	}

	lv, ok := layoutFor("lever")
	if !ok {
		t.Fatal("lever has no layout")
	}
	// Measured on jobs.lever.co/coderio/.../apply, 2026-09-09: the form carries the same
	// id Greenhouse's does, the button a person clicks is #btn-submit, and the inputs
	// carry no id at all — only name.
	if lv.formSelector != "application-form" || lv.submitSelector != "#btn-submit" || lv.addressBy != byName {
		t.Errorf("lever layout = %+v, want what was measured on a live posting", lv)
	}
}

func TestLayoutFor_RefusesAProviderWithNoFillPath(t *testing.T) {
	for _, provider := range []string{"ashby", "workable", "recruitee", ""} {
		if _, ok := layoutFor(provider); ok {
			t.Errorf("layoutFor(%q) returned a layout; no fill path exists for it", provider)
		}
	}
}

// The registry and fillProviders answer the same question and must not drift: a provider
// Submit will try to fill, with no layout to fill it by, would reach a submit click with
// selectors that match nothing.
func TestLayoutFor_CoversEveryFillProvider(t *testing.T) {
	for provider := range fillProviders {
		if _, ok := layoutFor(provider); !ok {
			t.Errorf("%q is in fillProviders but has no layout", provider)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/atsapply/ -run TestLayoutFor`
Expected: FAIL to build — `layoutFor`, `formLayout`, `byID`, `byName` undefined.

- [ ] **Step 3: Write the registry**

Create `internal/api/atsapply/layout.go`:

```go
package atsapply

// addressing is how a platform identifies a field on its own application form — which
// attribute names it, and therefore which attribute selects it.
//
// It is ONE setting rather than two because the two must agree. If the scan identified a
// field by one attribute and the fill selected it by another, every field would resolve
// and then not be found on the page: a silent failure at the last step, after the model
// spend and the candidate's approval.
type addressing int

const (
	// byID identifies a field by its `id`. Greenhouse names every control.
	byID addressing = iota
	// byName identifies a field by its `name`. Lever's inputs carry no id at all —
	// `<input type="text" data-qa="name-input" name="name" required>` — measured on
	// jobs.lever.co/coderio/.../apply, 2026-09-09.
	byName
)

// formLayout is what this package needs to know about one platform's application page.
// Three values, each of which somebody read off a real posting.
//
// A table rather than heuristics, deliberately. Finding the form by "the one with the most
// inputs", or the button by "the one that says Apply", is the class of guess this package
// was burned by twice in one day: a captcha marker that fired on the mere WORD "recaptcha"
// and parked every Greenhouse posting there is, and a per-provider captcha list that
// decided what a page said before anyone loaded the page. A submit click cannot be taken
// back, so what drives one is measured, not inferred.
type formLayout struct {
	// formSelector is the element id the application form renders under — the id itself,
	// with no leading '#', because renderedHTML waits on it with chromedp.ByID and
	// domscan finds it with findByID.
	formSelector string
	// submitSelector is the CSS selector for the button a PERSON clicks. On Lever that is
	// `#btn-submit`, a type="button" whose own JS performs the invisible hCaptcha when the
	// employer enabled one and then clicks the real hidden button. Nothing here needs to
	// know that.
	submitSelector string
	// addressBy is how a field on this platform is named and selected.
	addressBy addressing
}

// layouts is every provider this package can drive a browser against. A provider absent
// here has no fill path, which Submit reports as not-implemented rather than attempting.
var layouts = map[string]formLayout{
	"greenhouse": {formSelector: "application-form", submitSelector: "#submit_app", addressBy: byID},
	"lever":      {formSelector: "application-form", submitSelector: "#btn-submit", addressBy: byName},
}

// layoutFor returns the provider's layout, or false when this package cannot drive it.
//
// Both platforms happen to call the form `application-form` today. That is a coincidence
// and is written out per-provider rather than shared, because the third platform will not
// share it and a shared constant would have to be un-shared under time pressure.
func layoutFor(provider string) (formLayout, bool) {
	l, ok := layouts[provider]
	return l, ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/atsapply/ -run TestLayoutFor -v`
Expected: PASS — three tests. `TestLayoutFor_CoversEveryFillProvider` passes trivially for now (`fillProviders` still holds Greenhouse alone); Task 4 is what makes it load-bearing.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/api/atsapply/
go vet ./... && go test ./...
git add internal/api/atsapply/layout.go internal/api/atsapply/layout_test.go
git commit -F <a file containing a message explaining the registry and why it is measured values rather than heuristics>
```

---

### Task 2: Scanning by the layout

**Files:**
- Modify: `internal/api/atsapply/domscan.go`
- Create: `internal/api/atsapply/leverform_test.go`
- Modify: `internal/api/atsapply/browser.go` (the `greenhouseFormReadySelector` constant's callers)

**Interfaces:**
- Consumes: `formLayout`, `byName`, `layoutFor` (Task 1).
- Produces: `func ScanForm(pageHTML string, layout formLayout) ([]DOMField, error)` — replaces `ScanGreenhouseForm`. `DOMField.ID` carries the field's identifier under the layout's addressing.

- [ ] **Step 1: Capture the fixture**

The fixture must be reduced from the real DOM, not written to match the code. These are the controls a live Lever posting renders, captured on 2026-09-09 from `jobs.lever.co/coderio/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037/apply` — copy them verbatim into `internal/api/atsapply/leverform_test.go`:

```go
package atsapply

import "testing"

// Reduced from the live DOM of jobs.lever.co/coderio/6ce0e52b-.../apply, captured
// 2026-09-09. Every attribute below is verbatim; only unrelated markup between the
// controls was dropped.
//
// The point of the fixture is what Lever does NOT have: no input carries an `id`. Only
// the location autocomplete does, and its id is not what the platform posts under.
const leverFormHTML = `
<html><body>
<form id="application-form" enctype="multipart/form-data" method="POST">
  <input id="resume-upload-input" name="resume" type="file">
  <input type="text" data-qa="name-input" name="name" required="">
  <input name="email" data-qa="email-input" type="email" required="">
  <input type="text" data-qa="phone-input" name="phone" required="">
  <input class="location-input" data-qa="location-input" id="location-input" type="text" maxlength="100" name="location">
  <input type="text" name="org">
  <input type="text" name="urls[LinkedIn]">
  <input type="radio" name="cards[192519c4-2fbe-4575-9ab0-db6643cd5135][field0]" value="Menos de 1 año" required="required">
  <input type="radio" name="cards[192519c4-2fbe-4575-9ab0-db6643cd5135][field0]" value="Entre 1 y 2 años" required="required">
  <button id="hcaptchaSubmitBtn" type="submit" class="hidden"></button>
  <button id="btn-submit" type="button" data-qa="btn-submit">Submit application</button>
</form>
</body></html>
`

// Under byName addressing a field's identity is its name, because that is the only thing
// Lever's inputs carry — and it is what the platform posts under.
func TestScanForm_IdentifiesLeverFieldsByName(t *testing.T) {
	layout, _ := layoutFor("lever")

	fields, err := ScanForm(leverFormHTML, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}

	byIdent := map[string]DOMField{}
	for _, f := range fields {
		byIdent[f.ID] = f
	}
	for _, want := range []string{"name", "email", "phone", "location", "resume", "urls[LinkedIn]"} {
		if _, ok := byIdent[want]; !ok {
			t.Errorf("no field identified as %q; scanned %+v", want, fields)
		}
	}
	if got := byIdent["resume"].Kind; got != "file" {
		t.Errorf("resume kind = %q, want file", got)
	}
	// The location autocomplete has BOTH an id and a name, and they differ. The name is
	// what Lever posts under, so byName must not quietly prefer the id.
	if _, wrong := byIdent["location-input"]; wrong {
		t.Error(`the location field was identified by its id ("location-input") rather than its name ("location")`)
	}
}

// A Greenhouse posting must keep being identified by id — the change must not move the
// provider that already worked.
func TestScanForm_StillIdentifiesGreenhouseFieldsByID(t *testing.T) {
	layout, _ := layoutFor("greenhouse")

	fields, err := ScanForm(greenhouseFixtureHTML, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("no fields scanned from the vanilla Greenhouse fixture")
	}
	for _, f := range fields {
		if f.ID == "" {
			t.Errorf("a Greenhouse field came back with no identifier: %+v", f)
		}
	}
}

// An input a byName layout cannot address is dropped rather than given a synthetic key.
// Under byID such a control gets one so two of them do not collide in the scan; under
// byName that key would produce a field that resolves and then cannot be typed into —
// the silent last-step failure the addressing setting exists to prevent.
func TestScanForm_DropsANamelessFieldUnderByNameAddressing(t *testing.T) {
	const html = `<html><body><form id="application-form">
	  <input type="text" name="email">
	  <input type="text" required="">
	</form></body></html>`
	layout, _ := layoutFor("lever")

	fields, err := ScanForm(html, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}
	if len(fields) != 1 || fields[0].ID != "email" {
		t.Fatalf("scanned %+v, want only the addressable field", fields)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/atsapply/ -run TestScanForm`
Expected: FAIL to build — `ScanForm` undefined.

- [ ] **Step 3: Make the scan layout-driven**

In `internal/api/atsapply/domscan.go`:

Replace `ScanGreenhouseForm` with `ScanForm`, taking the layout:

```go
// ScanForm parses a rendered application page's form into its field inventory, per the
// platform's own layout. Pure function over an HTML string — no browser, no network —
// mirroring internal/ingest/applyform's own FromLever, which parses markup the same way.
func ScanForm(pageHTML string, layout formLayout) ([]DOMField, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, fmt.Errorf("parse page: %w", err)
	}
	form := findByID(doc, layout.formSelector)
	if form == nil {
		return nil, fmt.Errorf("no #%s on the page", layout.formSelector)
	}
	return scanControls(form, layout.addressBy), nil
}
```

Thread `addressBy` through `scanControls`, `scanInput` and `scanSimple`. In each place a
field's identity is decided, the rule is:

```go
// identify returns the identifier this layout addresses a control by, and whether the
// control can be addressed at all.
//
// Under byName a control with no name is dropped: it can never be selected on the page,
// so a field for it would resolve and then fail at the fill. Under byID the existing
// synthetic-key fallback stands — it exists so two id-less controls do not collide in the
// scan, and a Greenhouse control that lands there is still reported to the candidate as an
// unmapped question rather than silently filled.
func identify(id, name string, by addressing, order *[]string) (string, bool) {
	if by == byName {
		if name == "" {
			return "", false
		}
		return name, true
	}
	return fallbackKey(id, name, order), true
}
```

Set `DOMField.ID` from that identifier for both addressings, keeping `Name` as the raw
attribute. Everything downstream — `Reconcile`, `Resolve`, the plan, the answer bank's
topics — keeps working on one identifier and needs no change.

Update the two callers of `ScanGreenhouseForm` (`client.go`, `preview_client.go`) to pass
the layout. Task 3 and Task 4 rewrite those call sites properly; here, just make the build
pass by looking the layout up at the call.

Rename `greenhouseFormReadySelector` in `browser.go` to nothing — its value is now
`layout.formSelector`, and `renderedHTML`'s callers pass it. Delete the constant.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/atsapply/ -v`
Expected: PASS, the whole package. The pre-existing Greenhouse scan tests must pass
unmodified — if one needed editing, the change moved behaviour it should not have.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/api/atsapply/
go vet ./... && go test ./...
git add internal/api/atsapply/
git commit -F <a file containing a message explaining that the scan now identifies a field the way the platform does>
```

---

### Task 3: Selecting and submitting by the layout

**Files:**
- Modify: `internal/api/atsapply/fill.go`
- Modify: `internal/api/atsapply/fill_test.go` (if one exists; otherwise add the test to `layout_test.go`)

**Interfaces:**
- Consumes: `formLayout`, `byName` (Task 1).
- Produces: `func fieldSelector(id string, by addressing) string`; `fillAndSubmit(ctx, plan, layout)`.

- [ ] **Step 1: Write the failing test**

Add to `internal/api/atsapply/layout_test.go`:

```go
// The selector must address the field the same way the scan identified it — and a name
// containing brackets (Lever's `urls[LinkedIn]` and `cards[<uuid>][field0]`) must survive
// into a valid attribute selector.
func TestFieldSelector_AddressesAFieldTheWayItsPlatformNamesIt(t *testing.T) {
	if got := fieldSelector("first_name", byID); got != "#first_name" {
		t.Errorf("byID selector = %q, want #first_name", got)
	}
	if got := fieldSelector("name", byName); got != `[name="name"]` {
		t.Errorf("byName selector = %q, want [name=\"name\"]", got)
	}
	if got := fieldSelector("urls[LinkedIn]", byName); got != `[name="urls[LinkedIn]"]` {
		t.Errorf("byName selector for a bracketed name = %q, want it quoted intact", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/atsapply/ -run TestFieldSelector`
Expected: FAIL to build — `fieldSelector` takes one argument.

- [ ] **Step 3: Make selection and submission layout-driven**

In `internal/api/atsapply/fill.go`:

```go
// fieldSelector builds the CSS selector that addresses a field, the way its own platform
// names it. A byName selector quotes the value, because Lever's names carry brackets
// (`urls[LinkedIn]`, `cards[<uuid>][field0]`) that are syntax in an unquoted selector.
func fieldSelector(id string, by addressing) string {
	if by == byName {
		return fmt.Sprintf("[name=%q]", id)
	}
	return "#" + id
}
```

`fillOne` currently passes `chromedp.ByID` alongside the selector. Under `byName` that is
wrong — the selector is a query, not an id. Take the query kind from the addressing in the
same place the selector is built, so the two cannot disagree:

```go
// queryKind is how chromedp must interpret the selector this addressing produces.
func (a addressing) queryKind() chromedp.QueryOption {
	if a == byName {
		return chromedp.ByQuery
	}
	return chromedp.ByID
}
```

Thread the layout into `fillOne` and use both. Replace the hard-coded
`greenhouseSubmitSelector` click in `fillAndSubmit` with `layout.submitSelector`, and delete
the constant.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/atsapply/ -v`
Expected: PASS, the whole package.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/api/atsapply/
go vet ./... && go test ./...
git add internal/api/atsapply/
git commit -F <a file containing a message explaining that the selector and the query kind are one decision>
```

---

### Task 4: Turning Lever on

**Files:**
- Modify: `internal/api/atsapply/client.go`
- Modify: `internal/api/atsapply/preview_client.go`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing new — this is the switch.

- [ ] **Step 1: Write the failing test**

Add to `internal/api/atsapply/layout_test.go`:

```go
// Lever can be filled. This is the switch the whole change exists to flip, and it is
// asserted separately from the layout because the two are edited in different files and a
// half-done change is exactly what TestLayoutFor_CoversEveryFillProvider guards against
// from the other side.
func TestFillProviders_IncludesLever(t *testing.T) {
	if !fillProviders["lever"] {
		t.Error("lever is not in fillProviders; its resolved plans still park as not-implemented")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/atsapply/ -run TestFillProviders`
Expected: FAIL — `lever is not in fillProviders`.

- [ ] **Step 3: Flip the switch and generalise the branch**

In `internal/api/atsapply/client.go`, add `"lever": true` to `fillProviders`, with a comment
naming what was measured (the live preview came back fully resolved; the form's controls
were captured on 2026-09-09).

Replace `if claimed.Provider == "greenhouse"` with a layout lookup:

```go
	layout, canDrive := layoutFor(claimed.Provider)
	if canDrive {
		// The providers this package can drive a browser against — see layout.go. A
		// provider with no layout reaches neither a browser nor a submit click.
		...
	}
```

Pass `layout.formSelector` to `renderedHTML` and `layout` to `ScanForm` and `fillAndSubmit`.

Make the same change in `preview_client.go`'s own Greenhouse branch — the preview must scan
Lever's DOM too, or the candidate's preview and the submission would disagree about what
the form contains.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/atsapply/ -v`
Expected: PASS, including `TestLayoutFor_CoversEveryFillProvider`, which is now
load-bearing: it fails if a provider is added to one of the two places and not the other.

- [ ] **Step 5: Verify the whole build**

Run: `go build ./... && go vet ./... && go test ./... && go vet -tags=integration ./...`
Expected: all clean.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/api/atsapply/
git add internal/api/atsapply/
git commit -F <a file containing a message explaining that Lever can now be filled, and what was measured to justify it>
```

---

### Task 5: The package's own account of itself

**Files:**
- Modify: `internal/api/atsapply/AGENTS.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

- [ ] **Step 1: Read what it currently says**

Run: `cat internal/api/atsapply/AGENTS.md`

It describes a package that drives Greenhouse alone. Find every statement that is now false
— the provider list, the "one provider with a live DOM-scan" framing, anything naming
`ScanGreenhouseForm` or `greenhouseSubmitSelector`.

- [ ] **Step 2: Rewrite those statements**

State, in the file's existing voice:

- The registry is what makes a provider drivable, and a provider absent from it parks as
  not-implemented.
- Its three values are measured off a real posting, never inferred — with the two failures
  that taught this named, since they are what an agent editing this file next needs to know:
  the captcha marker that fired on the WORD "recaptcha" and parked every Greenhouse posting,
  and the per-provider captcha list that decided what a page said before anyone loaded it.
- Addressing is one setting because a mismatch between identification and selection is
  silent — the field resolves, then cannot be typed into.
- A submit click is irreversible, so an unconfirmed submission is never retried.

Do not restate what the code already says plainly. This file is for what the code cannot
say: why.

- [ ] **Step 3: Verify the links resolve**

Run: `pnpm check:links`
Expected: every relative link resolves. This is a pre-commit hook as well; a renamed target
is the usual cause of a failure here.

- [ ] **Step 4: Commit**

```bash
git add internal/api/atsapply/AGENTS.md
git commit -F <a file containing a message explaining what the package's account of itself now says>
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| The registry (three values per provider) | 1 |
| Addressing decides identification AND selection | 2, 3 |
| A `byName` field with no name is dropped | 2 |
| Radio groups need nothing new | covered — no change required; `fillOne`'s `checkbox_group` branch already selects `input[name=…][value=…]` |
| Submitting via the layout's button | 3 |
| Unconfirmed is not retried | unchanged — existing behaviour, asserted by existing tests |
| Lever in `fillProviders` | 4 |
| Preview scans Lever's DOM too | 4 |
| Fixture reduced from real DOM | 2 |
| Registry test covering every fill provider | 1 (written), 4 (made load-bearing) |
| Live verification on entries 6 and 10 | deliberately NOT a task — it is a real application to a real employer, taken with the candidate's approval after this ships |

**Type consistency:** `formLayout`, `addressing`, `byID`, `byName`, `layoutFor` are defined in Task 1 and used under those exact names in Tasks 2, 3 and 4. `ScanForm(pageHTML string, layout formLayout)` is defined in Task 2 and called in Task 4. `fieldSelector(id string, by addressing)` is defined in Task 3.

**Placeholders:** none. The commit messages are described rather than written out, deliberately — a commit message asserting what was measured must be written by whoever ran the commands, not copied from a plan.

**One thing the plan does NOT do:** it does not verify that an invisible hCaptcha lets a headless browser through. Nothing here can. Entries 6 and 10 carry no captcha at all, which is why they are the live check; what happens on a Lever posting that does carry one stays unknown until one is attempted, and the honest outcome for it is `StatusUnconfirmed`.
