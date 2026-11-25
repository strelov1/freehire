## Context

`autofillagent.Planner` (`internal/ai/autofillagent/agent.go:98`) is one interface with two
methods, and today exactly one implementation — `LLMPlanner` — satisfies both:

- `Plan(fields, profile) -> []Fill`: one batched call that maps the whole form at once,
  mixing free-text extraction (e.g. "take the city out of the address") with discrete
  choices (selects, checkboxes) in a single prompt/JSON response.
- `Choose(question, options, profile) -> string`: one call per custom-widget combobox,
  asked only after the page has rendered the widget's real option list (`agent.go:320`),
  because the options do not exist before then. Purely discrete: pick one of `options`
  verbatim, or decline (return `""`) when the profile supports none of them.

Only `Choose` is a clean match for Jev's `Choice` question type (a closed label set with
per-option probabilities). `Plan` is not, because it answers free-text and discrete fields
in the same call — splitting it is a separate, larger change, explicitly out of scope.

A spike (2026-10-08) confirmed against the live Typesafe API
(`https://api.typesafe.ai/v1/systemone`, `POST`, Bearer key) that a `Choice` question with
the page's real options plus an explicit "none of these" alternative reproduces `Choose`'s
documented decision rules: 8/8 synthetic cases matched (location, visa sponsorship,
relocation, notice period, salary range, and two decline cases), each at 0.99-1.0
confidence, at ~0.3-0.5s per call.

## Goals / Non-Goals

**Goals:**
- Answer `Choose` with Jev when `TYPESAFE_API_KEY` is configured, preserving the exact
  same contract callers rely on today: a verbatim option from `options`, or `""`.
- Degrade safely: any Jev-side failure (network, auth, malformed response) falls back to
  the existing `LLMPlanner.Choose` for that call, so a third-party outage never turns into
  an unanswered field that today's LLM path would have answered.
- Leave `Plan` byte-for-byte on `LLMPlanner.Plan` — this change does not touch it.
- Zero behavior change when `TYPESAFE_API_KEY` is unset (the default today).

**Non-Goals:**
- Splitting `Plan` into discrete/free-text sub-calls so it too can use Jev.
- Touching `LLMDrafter.Draft` (`internal/api/atsapply/llm_drafter.go`), `cmd/auto-apply`,
  or `cmd/enrich`.
- Shadow-mode comparison logging between Jev and the LLM on production traffic (a
  shadow-first rollout was proposed and explicitly declined in favor of a direct switch
  with a fallback safety net).
- A/B rollout controls, per-user flags, or a kill switch beyond unsetting the env var.

## Decisions

**`JevPlanner` wraps `LLMPlanner` rather than replacing it.**
`Planner` is one interface with two methods; `JevPlanner{LLM: LLMPlanner}` implements
`Plan` by delegating straight to `p.LLM.Plan` and implements `Choose` with its own Jev-first
logic. This keeps the swap to the single construction site
(`internal/api/handler/autofill_agent.go:48`) instead of threading a provider choice
through every call site of `Planner`.

**Fallback lives inside `Choose`, not as a separate decorator.**
Considered a generic `FallbackPlanner(primary, secondary Planner)` composition instead.
Rejected: it would apply the same fallback rule to `Plan` too, which is out of scope here
and was never spiked. Keeping the fallback inline in `JevPlanner.Choose` keeps the
unvalidated `Plan` path untouched.

**Decline is modeled as an explicit extra criterion, not a confidence threshold.**
The spike added a `"none of these"` key alongside the real options in the `Choice`
question's `criteria` map and let Jev pick it directly, rather than picking a probability
cutoff on the top option. This mirrors `choosePrompt`'s own instruction ("declining is the
correct answer whenever nothing in the profile supports any of the options") as a named
alternative instead of a tunable number, and it is what the spike validated — a threshold
was not tested and would be a second, unvalidated decision.

**No new provider abstraction in `internal/platform/llm`.**
Jev is called directly inside the autofillagent package (a small unexported HTTP client),
not registered as a new `llm.Client` backend. Jev does not speak the Chat
Completions-shaped interface `llm.Client` wraps (it has no notion of a conversation or
free-text generation), so forcing it through that seam would need a shim that lies about
what it supports. This mirrors the finding from the closed `cmd/enrich` Jev PR (#3068) and
the `langchain-typesafe` precedent: LangChain itself models Jev as a
`RunnableSerializable[ClassifierRequest, ClassifierResponse]`, not a `BaseChatModel`.

**No external Jev client library — a minimal hand-rolled HTTP client instead.**
Considered `github.com/wawan93/gojev`, the only existing Go wrapper (confirmed working
against the live API during the spike). Rejected: it is a two-commit personal package from
the same contributor whose `cmd/enrich` Jev PR (#3068) was closed for a bug, carries
generic support for Jev's `Noul`/`Score` question types this change never uses, and the
integration surface is one HTTP endpoint (`POST /v1/systemone`, a JSON body, a Bearer
header) simple enough that vendoring a client for it traded a smaller diff for an
unreviewed external dependency with no offsetting benefit. No official Go client exists
either: Typesafe's own `langchain-typesafe` partner package is Python-only and alpha
(`0.0.1a3`), and `langchaingo` has no Typesafe provider. `internal/ai/autofillagent/jev.go`
implements the one request/response shape directly, with no retries (a single attempt;
`JevPlanner` already falls back to the LLM on any failure, so a client-level retry would
only delay that fallback) and a bounded 15s timeout.

**Config: one new optional env, independent of any other `TYPESAFE_*` setting.**
`TYPESAFE_API_KEY` is read fresh for this feature. It is not shared state with `cmd/enrich`
(which never merged the setting) or any other consumer — if a future change adds Jev
elsewhere, it is free to read the same env var, but this change does not introduce a
shared config struct for that until a second consumer exists.

## Risks / Trade-offs

- **[Risk]** Jev's real-traffic accuracy on live, messy combobox option lists (long labels,
  near-duplicate options, non-English text) could differ from the 8 synthetic cases spiked.
  → **Mitigation**: the fallback-on-error path does not catch "Jev answered confidently but
  wrong" — only hard failures. This is an accepted trade-off of choosing direct replacement
  over shadow-mode (explicitly requested); if live mismatches surface, the next change adds
  the shadow/comparison instrumentation that was deferred here.
- **[Risk]** Jev adds a network hop on the autofill critical path. → **Mitigation**:
  bounded by the client's own 15s HTTP timeout, one attempt, no retries; a slow/hanging Jev
  call is bounded, not unbounded, and the existing `agent.go` run-level timeout still
  applies as the outer bound.
- **[Risk]** `TYPESAFE_API_KEY` leaking into logs/errors. → **Mitigation**: never logged;
  errors returned from the Jev call path are wrapped without including request/response
  bodies containing the key (the key is a header, never serialized into the request body
  that gets logged).

## Migration Plan

- No data migration. No stored schema changes.
- Deploy: ship `JevPlanner` behind `TYPESAFE_API_KEY` being unset by default — the change
  is inert until the env var is set in a deployment's config.
- Rollback: unset `TYPESAFE_API_KEY` (or revert the one-line construction-site change) to
  return to pure `LLMPlanner` behavior; no code rollback required to disable.

## Open Questions

- None blocking implementation. Whether to add shadow-mode comparison logging later is
  deferred to a follow-up change if live mismatches are observed.
- `JevPlanner.Choose` discards the Jev-side error before falling back, with no log line —
  matching the rest of this package, which logs nothing anywhere. If the key expires or
  Jev starts erroring on every call, the feature silently and permanently degrades to pure
  LLM behavior with no signal to notice it happened. Flagged during code review as worth a
  follow-up once there is real production traffic to watch; not addressed here to keep
  this change's scope to the provider swap itself.
