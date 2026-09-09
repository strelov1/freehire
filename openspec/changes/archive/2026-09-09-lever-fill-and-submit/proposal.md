## Why

A Lever application now resolves completely and still cannot be sent. Retiring the blanket
`requiresCaptcha{"lever": true}` refusal (#2721) let the preview pass reach Lever's schema
for the first time, and queue entry 6 came back with **every field answered and nothing
pending** — full name, email, phone, current location. `fillProviders` names Greenhouse
alone, so `Submit` parks it as not-implemented. The data is there; the code to type it is
not.

Lever is also the cheapest second provider available: most of the pipeline is already
provider-agnostic, and what differs between the two platforms is three values somebody can
read off a real posting.

## What Changes

- A per-provider **form layout registry** replaces the four places the provider is currently
  hard-coded: the form's element id (`domscan.go`), the field selector (`fill.go`), the
  submit button (`fill.go`), and whether to scan the DOM at all (`client.go`,
  `preview_client.go`).
- `ScanGreenhouseForm` becomes `ScanForm(pageHTML, layout)` — it scans whatever form the
  layout names. **BREAKING** for callers inside this package only; the function is not used
  outside it.
- A field's **addressing** becomes explicit: Greenhouse identifies and selects a control by
  `id`, Lever by `name`. Lever's inputs carry no `id` at all. This is one setting rather
  than two, because a mismatch between identification and selection is silent — every field
  would resolve and then not be found on the page.
- A `byName` control with no `name` attribute is **dropped from the scan** rather than given
  the synthetic key an id-less control currently gets: it can never be selected, so a field
  for it would resolve and fail at the fill.
- `lever` joins `fillProviders`. Its resolved plans reach a browser, a fill and a submit
  click instead of parking as not-implemented.
- The preview pass scans Lever's live DOM too, so the candidate's preview and the actual
  submission cannot disagree about what the form contains.

Not in this change: Ashby, Workable and Recruitee (no fill path either, and the browser-use
fallback already in the code is a separate metered decision); solving a rendered captcha
challenge (those still park); anything about which postings to apply to.

## Capabilities

### New Capabilities
- `atsapply-form-layouts`: which application platforms this system can drive a browser
  against, and what it must know about each one's page to do so safely.

### Modified Capabilities
<!-- None. atsapply-unscannable-form-detection's requirements are unchanged: a form that
     cannot be read still parks, and a rendered challenge still parks. What changes is which
     platforms are attempted at all, which that spec does not speak to. -->

## Impact

- `internal/api/atsapply`: `layout.go` (new), `domscan.go`, `fill.go`, `client.go`,
  `preview_client.go`, `browser.go`, `AGENTS.md`.
- No database, API, or wire-format change. No migration.
- **Operationally significant:** this is the change after which auto-apply can submit a real
  application to a real employer on a second platform. A submit click cannot be taken back,
  so an unconfirmed submission stays a distinct, deliberately non-retried outcome.
- Unknown, deliberately: whether an invisible hCaptcha lets a headless browser through.
  Four of six measured Lever postings carry one; the two queued for this candidate carry
  none. This change is what makes that answer observable.
