## Context

See proposal.md - Why for the motivating gap and the live spike findings. Two facts from
the spike shape this design directly:

- On a real Greenhouse posting, browser-use filled exactly the fields it was told, left
  every other field (including a residency question and every EEO/demographic field)
  completely untouched, correctly hit a native required-field validation it wasn't told
  how to satisfy, and reported that honestly rather than fabricating success — all
  without any of `internal/api/atsapply`'s own enforcement code. Cost/latency there: ~$0.026,
  ~70 seconds.
- Passing the target page as a `data:` URI (rather than a normal `https://` URL) bloated
  the agent's per-step context and drove cost/latency up nearly 3x ($0.07, ~12 minutes) —
  an artifact of the test method, not of the platform. Every execution this design builds
  always targets the posting's own live URL, never a constructed one.

`internal/api/atsapply` already assembles a fully-resolved `Plan` for Ashby/Workable today
(`applyform.Fetcher` → `Reconcile`'s `mergedFromAPIOnly` → `Resolve`/
`ResolveWithDrafting`) — the gap this change closes is purely the last mile: something to
execute that `Plan` on the live page, since only Greenhouse has a chromedp fill path
(`fillAndSubmit`, `browser.go`). Recruitee was assumed to be a third such provider when
this design was first drafted; found false during implementation —
`internal/ingest/applyform.Fetchers` never registered a `Fetcher` for it at all (its form
arrives free with the ingest crawl, written directly rather than fetched on demand), so
`Client.fetchSchema` already parks a Recruitee attempt before a `Plan` ever exists to hand
this executor. It is excluded from `browserUseProviders` for that structural reason, not
a policy one.

## Goals / Non-Goals

**Goals:**
- Submit through browser-use only what the existing deterministic pipeline already
  decided — the backend is a typist, never a decision-maker.
- Bound cost per execution and in aggregate before it is ever exposed to real traffic.
- Treat an ambiguous outcome as unconfirmed, never as success.

**Non-Goals:**
- Widening coverage to white-label Greenhouse (needs a scan-then-fill two-phase design —
  see proposal.md's excluded scope) or to captcha-protected postings (out of scope by
  policy, not by capability).
- Handling résumé/CV file upload through this backend.
- A local/self-hosted browser-use runtime — see proposal.md; this stays a pure cloud-API
  HTTP client.
- Widening `internal/candidate/hardconstraint`-style eligibility checks — this change
  reuses whatever the pipeline already resolved and gates nothing new about candidate
  fit; `add-auto-apply-eligibility-gate` already covers that upstream, unaffected here.

## Decisions

**A thin transport package, `internal/platform/browseruse`, separate from
`internal/api/atsapply`.** Mirrors `internal/platform/llm`'s "transport, not domain"
split: this package knows the v4 API's shapes (create/poll/fetch a run, the `maxCostUsd`
field) and nothing about `Plan`, ATS providers, or field resolution. Alternative
considered: fold the HTTP calls directly into `atsapply`. Rejected — the same reasoning
`platform/llm`'s placement already established: a provider-specific transport that leaks
into a domain package resists reuse and muddies `atsapply`'s existing DOM-fill-specific
tests.

**The executor builds one instruction string per execution, enumerating every resolved
field's id/label and exact value, with an explicit "do not act on anything else, do not
guess, do not submit unless every listed field succeeds" clause** — the same shape
verified live in the spike. Alternative considered: pass the candidate's raw known-answer
map and let browser-use match it against the live page itself (closer to how a generic
browser-use task usually works). Rejected outright — this is the one decision this whole
line of work already made and re-confirmed twice: the backend must never be the one
deciding an answer's content, or every invariant `internal/api/atsapply` enforces today
(sensitive-keyword gate, geography park, "never guess") would need reinventing on the
agent side, unverified.

**Three explicit terminal markers, not free-text inference, decide the outcome.** The
task instructs the agent to end its report as one of: a confirmation naming what it
observed, an explicit "unconfirmed," or an explicit "parked" naming a reason. Anything
else — including a plausible-sounding narrative with no explicit marker — is treated as
unconfirmed. Alternative considered: use browser-use v4's optional `judge` field (a
second LLM pass judging the run's trajectory) instead of a marker convention. Deferred,
not rejected — `judge` bills a second model call per execution (compounding the
per-attempt cost this design is already trying to bound) and its `judgement` payload's
exact shape was not fully characterized in the spike; worth reconsidering once real
volume shows the marker convention's false-unconfirmed rate.

**Cost bounded two ways: `maxCostUsd` per run, a daily aggregate threshold in
`cmd/auto-apply`.** The per-run cap is the v4 API's own field — cheap insurance against
exactly the runaway-context failure mode the spike's `data:` URI test produced. The daily
threshold is ours: read once at startup (mirroring `PLAN_ENFORCE`'s config-at-wiring-time
pattern, not a per-call env read — unlike `add-auto-apply-eligibility-gate`'s single
boolean flag, this is a numeric running total that needs to accumulate across calls
within a run, so memoizing the CONFIGURED threshold at startup is correct; only the
accumulated spend itself is mutable state, not the threshold), shipping in shadow mode
(log projected refusals) before being flipped to enforce — the same rollout discipline
`add-auto-apply-eligibility-gate` already used for the same reason: a false-positive
refusal here costs a legitimate submission, so it earns an observation window first.

## Risks / Trade-offs

- **No anti-detection signal from the spike** — the tested targets carried no bot/
  challenge protection, so whether browser-use actually holds up against Ashby/Workable's
  real defenses (if any) is unknown until this runs against live traffic in shadow mode.
- **Third-party dependency with no SLA established** — browser-use.com outages or latency
  spikes would degrade to the existing park behavior (the fallback for the fallback), but
  this is a new operational dependency `cmd/auto-apply` did not have before.
- **PII in transit** — individual resolved field values (name, email, phone, and similar)
  reach browser-use's cloud infrastructure. This is bounded (no CV file, no unrelated
  candidate data — only what the employer's own form asks for, and only values already
  destined for that employer), but it is still a third-party transit this codebase's
  existing LLM calls avoid via `internal/candidate/pii`'s masking — masking does not apply
  here because the backend must type real, correct values into a real form.
- **Two independently-evolving "outcome" vocabularies** — chromedp's `StatusApplied`/
  `StatusParked`/`StatusUnconfirmed` and this backend's confirmed/unconfirmed/parked
  markers must map onto the same `autoapply.SidecarClient` contract without drifting
  apart as either implementation changes; both live behind the same interface, so a
  future change to one's outcome shape should update the other's mapping deliberately,
  not accidentally.
