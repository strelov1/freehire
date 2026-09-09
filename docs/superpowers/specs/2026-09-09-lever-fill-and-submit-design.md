# Filling and submitting a Lever application

**Status:** design, approved 2026-09-09
**Problem owner:** auto-apply — Lever applications now resolve completely and still cannot be sent.

## Where this starts

Earlier today (#2721) the blanket `requiresCaptcha{"lever": true}` refusal was retired, on a
live measurement: two of the candidate's own queued Lever postings render no captcha of any
kind, four others carry an invisible hCaptcha, and the only "recaptcha" on any of them is a
string inside a CSS rule.

With the refusal gone, the preview pass reached Lever's schema for the first time. Queue
entry 6 came back with **every field answered and nothing pending**:

```
Full name         Ilya Strelov
Email             strelov1@gmail.com
Phone             +55 48 99653-6547
Current location  Florianópolis, Brazil
```

`fillProviders` still names Greenhouse alone, so `Submit` parks it as not-implemented. The
data is there; the code to type it is not. That is the whole of this change.

## What is already provider-agnostic

Most of the pipeline is. `fillAndSubmit` (`internal/api/atsapply/fill.go`) fills from the
resolved plan, clicks, and verifies against confirmation markers — none of it Greenhouse-
specific. `Resolve`, `Reconcile`, the answer bank, the résumé attachment and the
unconfirmed/parked outcomes are all shared.

The provider is baked into exactly four places:

| Where | What it hard-codes |
|---|---|
| `domscan.go:54` | `findByID(doc, "application-form")` |
| `fill.go:141` | `fieldSelector` builds `"#" + id` |
| `fill.go:74` | `greenhouseSubmitSelector = "#submit_app"` |
| `client.go:181` | `if claimed.Provider == "greenhouse"` — whether to scan the DOM at all |

## What actually differs, measured

Captured from `jobs.lever.co/coderio/.../apply` on 2026-09-09:

- **The form's id is `application-form` — the same as Greenhouse's.** A coincidence, and one
  worth writing down as a value rather than relying on silently: the third provider will not
  share it.
- **Lever's fields carry no `id` at all.** `<input type="text" data-qa="name-input"
  name="name" required>` — only `location-input` has one. Greenhouse addresses by `id`,
  Lever by `name`. This is a property of the platforms, not a gap in our scan.
- **The submit button is `#btn-submit`, and it is `type="button"`.** Clicking it runs
  Lever's own JS, which performs the invisible hCaptcha when the employer enabled one and
  then clicks the real, hidden `#hcaptchaSubmitBtn`. Nothing here needs to know that — we
  click the button a person clicks.
- Employer questions render as radio groups named `cards[<uuid>][field0]`, with values in
  the posting's own language (`"Menos de 1 año"` on this Spanish-language posting).

## The registry

One table, three values per provider:

```go
type formLayout struct {
    formSelector string // the element id the application form renders under
    submitSelector string // the button a person clicks to submit
    addressBy addressing // how a field is identified and selected: byID or byName
}
```

The four sites above read from it. `ScanGreenhouseForm` loses "Greenhouse" from its name —
it takes the layout and scans whatever form that names. `client.go`'s
`if provider == "greenhouse"` becomes "this provider has a layout".

**Why a table and not heuristics.** Finding the form by "the one with the most inputs" or
the button by "the one that says Apply" is the class of guess this package has been burned
by twice today — the captcha marker that fired on a word, and the provider list that decided
what a page said before anyone looked at it. A submit click is irreversible; three measured
values per provider are not a list of guesses, they are a description of a page somebody
loaded.

**Why not a per-provider code path.** A third provider would be a third copy of scan,
select, click and verify, plus a fourth `if` in each of the four sites. The registry makes
it one row.

## Addressing

`addressBy` decides two things that must agree:

1. **Which attribute identifies a scanned field.** `scanControls` already records both `ID`
   and `Name`. For a `byName` provider the field's identity IS its name — so the scan sets
   `DOMField.ID` from `Name`, and everything downstream (`Reconcile`, `Resolve`, the plan,
   the answer bank's topics) keeps working unchanged on one identifier.
2. **How `fieldSelector` builds the CSS selector** — `#id` or `[name="..."]`.

They are one setting because a mismatch between them is silent: fields would resolve and
then not be found on the page.

A `byName` field with no `name` attribute cannot be addressed at all, so the scan drops it
rather than minting the synthetic key it currently gives an id-less, name-less control. That
key exists so two such controls do not collide in the scan; for a `byName` provider it would
instead produce a field that resolves and then cannot be typed into — the silent failure this
setting exists to prevent. Dropping it means the platform's own schema is the only thing
that can still declare the field required, which is the honest outcome: the entry parks.

Lever's radio groups need nothing new — `fillOne`'s `checkbox_group` branch already selects
`input[name=…][value=…]`, and `cards[<uuid>][field0]` is an ordinary attribute value.

## Submitting

Click `#btn-submit`, then verify with the existing markers. A click that produces no
confirmation is `StatusUnconfirmed` — already a distinct, deliberately non-retried outcome.
That matters more here than on Greenhouse: an invisible hCaptcha may refuse a headless
browser, and the honest report is "we could not confirm it went through", never a retry that
might submit twice.

We do not know yet whether an invisible hCaptcha passes. This change is what makes the
answer observable; the design does not assume either result.

## What Lever still cannot do

An employer's custom questions (`cards[...]`) have no answer until the candidate gives one.
The screening answer bank (shipped today) is what collects them; until then such an entry
parks with the questions listed, which is the correct outcome and not a regression.

## Testing

- **Fixture tests** over the captured Lever form: the scan finds the fields, identifies them
  by name, and selects them the way `fillAndSubmit` would. The fixture is the real DOM,
  reduced — the same rule the rest of this package now follows, after a hand-written
  fixture passed while the live page failed.
- **A registry test** asserting every provider in `fillProviders` has a layout, so adding
  one without the other fails loudly rather than at a submit click.
- **Live verification** on queue entries 6 and 10 — both Lever, both captcha-free, both
  already fully resolved. This is a real application to a real employer, which is why it is
  a deliberate step with the candidate's approval rather than a test.

## Not in this design

- Ashby, Workable, Recruitee. They have no fill path either, and the browser-use fallback
  already in the code is a separate, metered decision.
- Solving a rendered captcha challenge. Unchanged: those park.
- Anything about which postings to apply to.
