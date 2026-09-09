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

**A second, narrower backend exists for two providers chromedp cannot fill/submit for at
all: Ashby and Workable, via the browser-use.com cloud agent API** (`browseruse_fill.go`,
`openspec/changes/add-browseruse-atsapply-fallback`). This does not reopen the decision
above — it is a plain HTTP client (`internal/platform/browseruse`), no second language or
process, called only via `Client.WithBrowserUse`. Scope is deliberately narrow:
- **Only Ashby and Workable** (`browserUseProviders`) — Greenhouse and Lever already have a
  fill path and never reach this branch; a white-label (unrecognized-layout) Greenhouse posting has
  no `MergedField` schema to build a `Plan` from in the first place, since chromedp's own
  DOM scan is what failed there; a captcha-protected posting (Lever, or
  `reasonCaptchaProtected`) is never routed here — that would be an attempt to bypass a
  platform's bot protection, not this backend's purpose. **Recruitee reaches a `Plan` now
  (its schema-fetch gap is closed — see `fetchSchema`'s stored-form fallback, below) but was never added
  here**: it is in neither `fillProviders` (no live DOM-scan built for it) nor
  `browserUseProviders`, so a fully-resolved Recruitee `Plan` still parks with
  `reasonSubmissionNotImplemented`, the same outcome Ashby/Workable get without the spend
  guard's approval.
- **Only an already fully-resolved `Plan`** (`Plan.FullyResolved()`) — this backend is
  handed exact field values to type and never a decision about what to answer. Every
  invariant `resolve.go`/`draft.go`/`sensitive.go`/`geography.go` already enforce (never
  guess, sensitive-keyword gate, geography park) stays entirely upstream of this backend,
  unchanged and unbypassed — it only ever executes what they already decided. **A résumé
  field no longer disqualifies a plan** (`resolve.go`'s own invariant guarantees any
  `Kind=="file"` field reaching a fully-resolved `Plan` IS the approved résumé, never an
  arbitrary upload — see `resolveOne`): `attachResumeIfPresent` uploads the same rendered
  PDF `Client.attachApprovedResume` already produces for the Greenhouse path into a fresh,
  per-attempt browser-use workspace (`CreateWorkspace`/`RequestFileUpload`/`UploadFile`,
  deleted again once the run ends — a résumé is candidate PII and outlives no attempt it
  was rendered for), and `buildTask` points the agent at it by field id/label rather than
  printing the local temp path a cloud VM could never read. The agent still never decides
  WHICH file to use or what a field's value is — it only attaches the one file this backend
  already resolved and already rendered.
- **Confirmation is a strict marker, never inferred from narrative text.** `buildTask`
  requires the agent's report to end in exactly one of `CONFIRMED: <text>` / `UNCONFIRMED`
  / `PARKED: <reason>`; `parseOutcome` treats anything else — including a marker not on the
  report's own last line — as unconfirmed, the same "ambiguous means not confirmed" rule
  chromedp's own `fillAndSubmit` already follows for `StatusUnconfirmed`.
- **Ships OFF by default** (`AUTO_APPLY_BROWSERUSE_ENFORCE`, a plain per-call env read, not
  memoized — memoizing would freeze whichever value the first call saw for the rest of the
  process, breaking `t.Setenv`-based tests the same way `add-auto-apply-eligibility-gate`'s
  earlier `sync.OnceValue` mistake did). Unset/shadow logs what would have been attempted
  and still parks — no execution ever runs in shadow mode, so there is no cost to observe
  beyond the log line's own count. A separate `runSpendGuard`
  (`AUTO_APPLY_BROWSERUSE_RUN_CAP_USD`) bounds aggregate spend, on top of the v4 API's own
  hard per-run `maxCostUsd` cap — but despite the name, this is a **per cmd/auto-apply
  process invocation** cap, not a calendar-day one (a fresh guard is built on every
  invocation; nothing persists spend across runs), and it is a soft, best-effort check
  (concurrent executions can both pass it before either records its cost), not a hard
  ledger. Found by code review: treat the daily-sounding framing with that in mind until
  it grows persisted state.

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
- **A platform is driven only if `layouts` (`layout.go`) describes its page.** That
  description is three values — the element the form renders under, the button a person
  clicks, and which attribute identifies a control — and each was read off a real posting.
  Greenhouse and Lever have one; everything else reconciles against `applyform.Form` alone
  (`mergedFromAPIOnly`) and parks as not-implemented however completely it resolved.

  **The three values are measured, never inferred, and that is not a style preference.**
  Locating the form as "the element with the most inputs", or the button by its text, is
  what this package already did wrong twice in one day: `hasRecaptchaMarker` fired on the
  mere WORD "recaptcha" and so parked every Greenhouse posting there is, and a
  `requiresCaptcha{"lever": true}` list decided what a page said before anyone had loaded
  the page — two of that candidate's own Lever postings carry no captcha at all. A submit
  click cannot be withdrawn.

  **Addressing is ONE setting, not two.** It decides both which attribute identifies a
  scanned control and which selects it (`domscan.identify`, `fill.fieldSelector`,
  `addressing.queryKind`). Splitting them fails silently: every field resolves, the plan
  calls itself complete, and nothing is found on the page — at the last step, after the
  model spend and after the candidate approved. A review caught exactly this in the
  file-upload branch, which had kept a literal `chromedp.ByID`; under Lever's `byName` that
  becomes `#[name="resume"]`, invalid CSS, on the one field every posting requires.
  `TestFillOne_EveryActionOnTheSharedSelectorCarriesTheLayoutsQueryKind` reads the source to
  keep it from coming back.

  Widening the live DOM-scan to another provider is a real gap to
  close, not a design decision to defend.
- **`fetchSchema` falls back to a stored form ONLY when no live fetcher is registered for
  the provider — not storage-first.** `Client.forms` (`WithStoredFormReader`, same
  `StoredFormReader` port `PreviewClient` uses) closes Recruitee's own schema-fetch gap:
  `internal/ingest/applyform.Fetchers` has no `Fetcher` for it (its form arrives free with
  the ingest crawl and is written directly to `apply_forms`), so before this it parked with
  `errNoSchemaFetcher` before `resolve` ever ran. This does NOT mirror
  `PreviewClient.schemaFor`'s storage-first order — `apply_forms` also holds a row for
  every provider `cmd/capture-apply-form` drains (Greenhouse, Ashby, Workable, Lever), so
  preferring storage unconditionally would risk a REAL submission reading a possibly-stale,
  display-captured row instead of a fresh fetch for providers that already work fine. A
  live fetcher, when one is registered, is always tried first; `c.forms` is reached only
  when `fetchSchema` would otherwise return `errNoSchemaFetcher`. **This does not make
  Recruitee submit** — see this file's own note on `browserUseProviders`, above: reaching a
  `Plan` is not the same as having anything to hand it to. See
  `openspec/changes/atsapply-recruitee-stored-schema`.
- **A posting whose form cannot be scanned parks with a named reason instead of
  erroring.** `ScanForm`'s selector comes from the layout, and Greenhouse's
  (`#application-form`) only ever matched the vanilla `job-boards.greenhouse.io` template.
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
- **A cover-letter free-text field prefers the candidate's own already-drafted letter over
  the generic `Drafter`, when one exists.** `isCoverLetterTextField` (`resolve.go`)
  recognizes the field (Greenhouse's `cover_letter_text` id, the free-text sibling of the
  file-kind `cover_letter` upload `isResumeField`'s own doc comment names) and
  `ResolveWithDrafting` (`draft.go`) checks `LetterReader` — satisfied directly by
  `*coverletter.Store`, the same structural fit `AtomReader` already has over
  `*experience.Store` — before ever calling `Drafter.Draft` for that field. A nil
  `*coverletter.Stored` (no letter drafted yet for this job) or a read error both degrade to
  the ordinary generic-drafter path, logged the same way `buildGroundingContext`'s own read
  failure degrades in `client.go`'s `resolve`. This does not make Recruitee-shaped gaps in
  drafting go away — it only ever runs where `resolve` reaches `ResolveWithDrafting` at all
  (today: Greenhouse only, per `fillProviders`), same as `LLMDrafter` itself. See
  `openspec/changes/autoapply-reuse-cover-letter`.
- **A geography/residency question (`geography.go`) parks before drafting too, for a
  different reason than the sensitive gate.** `sensitiveTerms` (`sensitive.go`) parks a
  question on POLICY grounds — compensation, EEO/demographic, work authorization/visa —
  regardless of how confident a draft would be. `geographyTerms` exists because the
  pipeline cannot VERIFY an answer: `candidateprofile` carries one free-text `location`
  string, never a discrete country/state-of-residence fact matching a platform's own
  enumerated dropdown options, so a drafted answer here would be invented, not reported.
  Found live: a Garner Health Greenhouse posting's "Current State of Residence" (a
  US-states-only dropdown) reached the drafter and was saved from a wrong submission only
  because the candidate's non-US address happened to match none of the offered options —
  `isGeographyLabel` makes that a designed park instead of an accident of one form's own
  option list (`openspec/changes/add-auto-apply-eligibility-gate`). Checked in
  `draftable` (`draft.go`) alongside `isSensitiveLabel`, before the drafter is ever
  invoked; a geography-labeled field already resolvable from a known answer (e.g.
  `location`) is unaffected and fills exactly as before.
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
`applyform.Fetchers` → (only when `layoutFor` describes the platform) launch a browser,
render the page via `renderedHTML` (the layout's own selector, or — on that wait's own
timeout — classify why via `classifyUnscannableForm`, which `Submit` maps to an early
`StatusParked` return via `unscannableFormResult`),
`ScanForm` → `Reconcile` → `Client.resolve`
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
