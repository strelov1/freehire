## Context

`internal/api/atsapply` drives a headless browser against an employer's application page:
scan what it renders, reconcile against the platform's declared schema, resolve against the
candidate's known answers, and fill and submit only when every required question is
answered.

It has driven exactly one platform. Greenhouse is named in four places — the form's element
id (`domscan.go:54`), the field selector (`fill.go:141`), the submit button
(`fill.go:74`), and whether to scan a DOM at all (`client.go:181`, and its twin in
`preview_client.go`). Everything else — `fillAndSubmit`, `Resolve`, `Reconcile`, the answer
bank, the résumé attachment, the parked and unconfirmed outcomes — is already
provider-agnostic.

Earlier today #2721 retired a blanket `requiresCaptcha{"lever": true}` refusal, measured
wrong: two of this candidate's own queued Lever postings render no captcha of any kind, four
others carry an invisible hCaptcha, and the only "recaptcha" on any of them is a string
inside a CSS rule. With the refusal gone, the preview pass reached Lever's schema and came
back with every field answered and nothing pending.

Two failures in this package this week shape the decisions below. A captcha marker fired on
the mere word `recaptcha` and parked every Greenhouse posting there is. A per-provider
captcha list decided what a page said before anyone had loaded the page. Both were
inferences standing in for a measurement.

## Goals / Non-Goals

**Goals:**
- Fill and submit a Lever application, using the data the preview already resolves.
- Make adding a third platform a row in a table rather than a fourth copy of scan, select,
  click and verify.
- Keep every existing Greenhouse behaviour byte-identical.

**Non-Goals:**
- Ashby, Workable, Recruitee. They have no fill path either; the browser-use fallback
  already in the code is a separate, metered decision.
- Solving a rendered captcha challenge. Those still park.
- Deciding which postings to apply to.
- Proving that an invisible hCaptcha admits a headless browser. Nothing here can; this is
  what makes the answer observable.

## Decisions

### A registry of measured page descriptions, not heuristics

Three values per platform: the form's element id, the submit control's selector, and how a
field is addressed.

*Alternative considered — heuristics.* Find the form as "the element with the most inputs",
the button as "the one whose text says Apply". Rejected: a submit click cannot be withdrawn,
and this is the shape of the two failures above. An inference that is wrong on one board is
wrong silently, on somebody's real application.

*Alternative considered — a code path per platform.* `ScanLeverForm`, `leverSubmitSelector`,
a second branch in `Submit` and in `fillAndSubmit`. Rejected: a third platform becomes a
third copy and a fourth `if` in each of four places.

Both platforms happen to call the form `application-form`. That is written out per platform
rather than shared: the third will not share it, and a shared constant would have to be
un-shared under time pressure.

### Addressing is one setting, not two

Greenhouse names every control; Lever's inputs carry no `id` at all — only `name` and a
`data-qa` hook. So the platform decides both which attribute identifies a scanned field and
which locates it on the page.

These are a single value because a mismatch between them is silent. Fields would resolve,
the plan would report itself complete, and nothing would be found — at the last step, after
the model spend and after the candidate approved the application. One value cannot
disagree with itself.

A `byName` control carrying no `name` is dropped from the scan rather than given the
synthetic key an id-less control currently gets. That key exists so two anonymous controls
do not collide in the scan; under `byName` it would instead mint a field that resolves and
then cannot be typed into. Dropping it leaves the platform's own schema as the only thing
that can still declare the field required — and the entry parks, which is honest.

### Click the button a person clicks

Lever's `#btn-submit` is a `type="button"`. Its own JS performs the invisible hCaptcha when
the employer enabled one, then clicks the real hidden `#hcaptchaSubmitBtn`. This package
clicks the visible one and does not model the rest: driving the hidden button directly would
bypass the page's own submission logic on a form whose behaviour we have measured once.

Lever's employer questions render as radio groups named `cards[<uuid>][field0]`. These need
no new code — `fillOne`'s existing `checkbox_group` branch already selects
`input[name=…][value=…]`, and a bracketed name is an ordinary attribute value. The selector
must quote it, since brackets are syntax when unquoted.

### The registry and the fill list must not drift

Two sets answer the same question in two files: which platforms will be filled, and which
have a page description. A platform in the first and not the second reaches a submit click
with selectors matching nothing. A test asserts the containment; it passes trivially until
Lever is added, which is when it starts holding weight.

## Risks / Trade-offs

- **An invisible hCaptcha may refuse a headless browser.** Unknown, and unknowable without
  attempting one. The mitigation is the existing unconfirmed outcome: a click that is not
  acknowledged is recorded as unconfirmed and never retried, because a duplicate application
  costs the candidate more than a missing one.
- **The Lever page was measured once, on one posting.** A posting with a different field set
  or a white-label domain may not match. That surfaces as an unscannable form, which already
  parks — the same fallback Greenhouse's white-label domains use.
- **The registry is still a list**, and this package's history is a warning about lists. The
  difference is what is in it: three values somebody read off a loaded page, checkable by
  loading it again, rather than a claim about what pages contain.
- **Greenhouse regressions would be silent** if the refactor changed identification for it.
  Its existing scan tests must pass unmodified; a test that needed editing is the signal
  that behaviour moved.
