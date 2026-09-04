# ATS browser-driver conventions

## Scope
Drives a headless Chrome (`chromedp`) against one job's live application-form page: scan the
rendered DOM, reconcile it against the platform's own declared schema
(`internal/applyform.Form`, reused — not re-fetched), resolve the merged fields against a
candidate's known answers, and fill + submit only when every required question is answered.
Implements `internal/autoapply.SidecarClient` — the one caller is `cmd/auto-apply`.

**This package is chromedp, in-process — not a Python/Patchright sidecar.** The OpenSpec
change this package belongs to originally proposed one; a follow-up spike found chromedp + a
real Chrome install matched or beat Patchright on every automation-detection signal measured,
at the cost of no second language, process, or deploy artifact. See
`openspec/changes/auto-apply-worker/design.md`'s "chromedp, not a Python/Patchright sidecar"
decision for the measurements and its caveats.

## Always true
- **The DOM decides what exists; required is the union of DOM and API.** A field the API
  declares but the DOM never renders is dropped (filling it would write into a control that
  isn't on the page); a field the DOM renders but the API never declares is kept regardless.
  Required is NOT DOM-only, though: a live Greenhouse posting rendered `country` as required
  with no HTML `required` attribute at all — the API's own required flag is what catches
  that. See `reconcile.go`'s `TestReconcile_RequiredIsTheUnionOfDOMAndAPI`.
- **Never guesses an answer.** A select/checkbox's answer must match one of the platform's
  own offered option labels, or the field parks. An optional field with no known answer is
  left alone entirely (neither filled nor reported) — nothing here drafts text for it.
- **A DOM-only live scan is built for Greenhouse only.** Lever always parks on its captcha
  (`requiresCaptcha`, a static per-provider check, not DOM-based detection) before any
  fetcher or browser is touched. Every other provider (Ashby, and anything reached in the
  future) reconciles against `applyform.Form` alone — `mergedFromAPIOnly` — and a fully
  resolved form for one of them still parks rather than being submitted through a fill path
  never built or verified. Widening the live DOM-scan to another provider is a real gap to
  close, not a design decision to defend.
- **A Greenhouse posting whose form cannot be scanned parks with a named reason instead of
  erroring.** `ScanGreenhouseForm`'s known selector (`greenhouseFormReadySelector`,
  `#application-form`) only ever matched the vanilla `job-boards.greenhouse.io` template.
  Live verification found a real, likely-common case it does not: a white-label custom
  domain (a real GoDaddy posting on `careers.godaddy`) renders a completely different DOM id
  scheme and is gated by reCAPTCHA Enterprise on the form itself. `renderedHTML` (`browser.go`)
  waits the full, unchanged `pageLoadTimeout` for the known selector and only then — not
  before — spends an ADDITIONAL `classifyTimeout` capturing the page's current HTML and
  classifying it (`classifyUnscannableForm`, pure and fixture-tested, no live browser
  needed): `"recaptcha"` appearing anywhere in the page's HTML (an unscoped substring search,
  not parsed against a specific iframe/script element — see `hasRecaptchaMarker`'s doc
  comment for why) maps to `reasonCaptchaProtected`, otherwise `reasonUnrecognizedLayout`. Either maps to
  `autoapply.StatusParked` (`unscannableFormResult`, `client.go`) — never a plain error —
  so `internal/autoapply`'s runner never spends its transient-failure retry/dead-letter
  budget on a form that will never change shape or stop being challenge-protected. **A live
  finding while verifying this fix, worth remembering**: an earlier cut shortened the
  selector wait itself to make room for classification inside the same overall budget, and
  that intermittently misclassified an ordinary, fully-scannable vanilla-template posting as
  `unrecognized_form_layout` under real load — classification time must stay strictly
  additive, never subtracted from the selector wait, or every fillable posting quietly loses
  reliability margin. If either park reason ever spikes among providers that were previously
  filling fine, treat it as a possible regression in the vanilla template's own DOM shape (or
  this detection itself), not as expected background noise. See
  `openspec/changes/auto-apply-whitelabel-greenhouse`. Scope stays detection-only: this does
  not add fill/submit coverage for a white-label domain's own bespoke form — an attempt still
  parks even when its layout IS eventually recognized as "just not ours to fill."
- **Only the résumé file field resolves; everything else does not.**
  `openspec/changes/auto-apply-tailored-resume` closed the artifact gap for the résumé/CV
  upload specifically: `resolve.go`'s `isResumeField` recognizes it by field id (`resume`,
  Greenhouse's own convention) or label, and it resolves once the claim carries an approved
  tailored CV (`Claimed.TailoredCVID != uuid.Nil` — set only once a candidate has reviewed
  and approved a tailoring run; see `internal/application/autoapply/AGENTS.md`). A cover
  letter or any other file field is still always unmapped — there is no artifact for those.
  `Client.attachApprovedResume` renders the approved CV through the existing Typst renderer
  (`internal/candidate/cv`) to a temp file on demand and removes it once the attempt ends; a
  render failure parks the attempt naming the field rather than being retried. No object
  storage is involved, so `cmd/auto-apply` still does not require `S3_*` to be configured —
  `candidateprofile`'s own résumé read (`resume.Store.Structured`) is a Postgres read that
  never touches `blobs`, and neither does the CV render.
- **A `Multi` field (a checkbox group taking several answers) only ever resolves at most one
  value.** Not a shortcut: `AnswerSource` never supplies more than one candidate value per
  question today, so there is never more than one to match in the first place. See
  `resolveOne`'s doc comment (`resolve.go`). Widening `AnswerSource` to a multi-valued
  source is what would turn this into a real gap.
- **A custom employer question resolves three ways, in order, before it parks: id match,
  label-keyword match, then a grounded LLM draft.** `answerKeyFor` (`resolve.go`) is an
  ID-based lookup for Greenhouse's own standardized field names; `labelAnswerKeyFor` is a
  narrow label-text fallback (today: `visa_sponsorship_needed` only — see its doc comment
  for why "authorized to work in this country" is deliberately NOT covered the same way,
  even though it is the same shape of gap). `ResolveWithDrafting` (`draft.go`) is the third
  and last resort: for a required, non-sensitive, free-text/single-choice field the first
  two steps left unmapped, it asks a `Drafter` (`LLMDrafter`, `internal/llm`-backed) for a
  grounded answer — ported from `freehire-apply/internal/drafting`'s pattern (single-shot
  call, sensitive-keyword gate, never an agentic loop). A drafted answer is still checked
  against the field's own offered options (`matchOption`, shared with the deterministic
  path) before it is used.
- **The sensitive-keyword gate (`sensitive.go`) runs before the model is ever called, and
  wins absolutely.** A question whose label matches — compensation, work authorization/visa
  sponsorship, or an EEO/demographic category — is never drafted, regardless of how
  confident a draft would be; `draftable` checks it before `Drafter.Draft` is ever invoked.
  `sensitiveTerms` is a port of `freehire-apply`'s own `isSensitive` list, with one fix a
  live smoke check found: the ported `"work authoriz"` is fixed-order and never matches
  the real, common phrasing `"authorization to work"` (words reversed) — replaced with the
  standalone `"authoriz"`, which catches either ordering.
- **A draft is grounded only in `Provenance.Publishable()` experience-bank atoms — never
  raw CV text, never a system-inferred fact.** `buildGroundingContext` (`grounding.go`)
  filters `internal/experience.Store.ListAtoms` to `cv_import`/`stated_in_chat`/`manual`
  provenance, the same gate `internal/cvedit`'s CV-write path already enforces, applied here
  at read time. An `agent_inferred` atom can never reach a draft.
- **Drafting LLM spend is attributed to the candidate, tagged `auto-apply-drafting`**
  — bound fresh per attempt (`llmkey.Bind`, in `Client.resolve`), never shared across
  attempts. `cmd/auto-apply` is one of exactly two binaries allowed to resolve a per-user
  LLM credential at all (`internal/llmkey/scope_test.go`'s allowlist, alongside
  `cmd/server`) — see `openspec/changes/auto-apply-llm-drafting/design.md`'s "cmd/auto-apply
  becomes a second per-user LLM caller" decision for why.
- **The fill/submit path (`fill.go`, `browser.go`) is the least-verified part of this
  package.** No unit tests exercise it — a real browser session cannot be faked usefully, and
  no test asserts a real submission against a live board (that would spam a real employer).
  Correctness rests on a single spike's measurements and the reference implementation's own
  documented rules, not on this package's own live testing, until real submit volume proves
  otherwise. One known gap: one field shape was scanned with an empty id on a real posting
  that this package does not yet name correctly.
- **An unconfirmed submission is never retried through the ordinary path.** If neither a
  confirmation nor a refusal marker appears after the submit click, `fillAndSubmit` reports
  that honestly rather than guessing either way, and `Client.Submit` returns
  `autoapply.StatusUnconfirmed` — a distinct outcome from an error. `internal/autoapply`'s
  runner dead-letters it immediately (the same forced path a lost post-submit DB record
  takes), because the click may well have gone through: retrying normally would risk a
  second real submission. A code review caught an earlier version of this that mapped the
  same situation to a plain retryable error — see `internal/autoapply/runner_test.go`'s
  `TestRunDeadLettersImmediatelyOnAnUnconfirmedSubmission`.

## How it works
`Client.Submit`: captcha short-circuit → fetch the platform's schema via
`applyform.Fetchers` → (Greenhouse only) launch a browser, render the page via `renderedHTML`
(known selector, or — on that wait's own timeout — classify why via `classifyUnscannableForm`,
which `Submit` maps to an early `StatusParked` return via `unscannableFormResult`),
`ScanGreenhouseForm` → `Reconcile` → `Client.resolve`
(deterministic `Resolve`, then — if an experience-bank reader is configured —
`ResolveWithDrafting` over what is still unmapped, via a freshly `llmkey.Bind`-ed
`LLMDrafter`) → if `Plan.FullyResolved()`, `fillAndSubmit`; else return `StatusParked` with
`Plan.Unmapped`. `fillAndSubmit` fills every resolved field (a select's
`SetValue` is followed by a dispatched `input`/`change` event, since `SetValue` alone writes
the DOM property without firing what a React-controlled select listens for), clicks
Greenhouse's submit button, and waits for a text-based confirmation or refusal marker —
matching neither reports `StatusUnconfirmed`, never silently treated as success.

`stealthAllocatorOptions` (`browser.go`) is the whole anti-detection surface: headless plus
`disable-blink-features=AutomationControlled`, the one flag the spike measured flipping
`navigator.webdriver` to `false`. Datacenter IP reputation is a separate, unaddressed risk —
see design.md's Risks.
