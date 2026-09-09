# Handoff — assistant-model-bakeoff

Written 2026-09-08. Twelve of twenty tasks are done and merged (#2639); the eight that
remain are groups 5 and 6, which are the bake-off itself.

## What the change is for

Choosing the assistant's model has been an argument from price lists, and a price list does
not predict what a turn costs here. Two reasons, both measured:

- The transcript is replayed into context every round, so cost grows with ROUNDS as well as
  tokens. A model needing fourteen rounds where another needs five costs far more than its
  token price says.
- Whether the provider serves the replayed prefix from cache decides the rest. #2633
  measured 20k tokens a turn being re-read at full price because the window slid.

Neither number is on a price page. The first thing this change touched proved the point: the
gateway catalogue quotes `deepseek/deepseek-v4-flash-0731` at **$0.14/$0.28** per million
where every secondhand summary said $0.05/$0.16.

## What is already merged and live

**One production-reachable change**, in #2639: `llm.Usage` carries `CachedInput`, read from
the `PromptCachedTokens` langchaingo already surfaces. Langfuse receives it as `usageDetails`
with **exclusive buckets** — the input bucket is sent net of the cache, because
`prompt_tokens` counts the cached prefix inside itself and sending both unadjusted bills it
twice. Dashboards read a smaller `input` from here on; that is the point.

Everything else merged is test-only: the run tally, the cache verdict, cost, ranking, the
price table, the case fixtures and the seed.

## Where the code is

| Piece | File |
|---|---|
| Tally, cache verdict, cost, ranking, price + case + profile loaders | `internal/api/handler/assistant_bakeoff.go` |
| Their unit tests | `internal/api/handler/assistant_bakeoff_test.go` |
| Seed helper (`//go:build integration`) | `internal/api/handler/assistant_bakeoff_seed_integration_test.go` |
| Committed price capture | `internal/api/handler/testdata/openrouter-prices.json` |
| Committed cases (3 real postings) | `internal/api/handler/testdata/bakeoff-cases.json` |

## The one thing you must rebuild locally

`internal/api/handler/testdata/bakeoff-profile.json` is **gitignored and must stay that
way** — this repository is public and the file is a real CV: a name, a phone number, an
address and an employment history. `.gitignore` holds a prefix rule (`bakeoff-profile*`,
`bakeoff-resume*`), deliberately a prefix rather than two filenames, because naming each one
let the second land untracked-but-visible until somebody noticed.

Its absence is its own error (`errMissingBakeoffProfile`), so unit tests **skip** on it while
a bake-off run must stop. CI has no business failing over a file it cannot have.

Shape:

```json
{"captured":"YYYY-MM-DD", "cv":{…internal/candidate/cv.Document…}, "experience":{…/me/experience…}}
```

To rebuild it:

- **experience** — `curl -H "Authorization: Bearer <key>" https://freehire.me/api/v1/me/experience`.
  An ordinary API key reaches this.
- **cv** — an API key does NOT reach it. `/me/resume` is `mw.cookie` (cookie-only, see
  `resume.go`), and of the 84 CVs the account holds every one is a tailored copy — the base
  document the tailoring flow mints them from stays behind a session. Export it from the
  browser, or build the `cv.Document` from the PDF as was done on 2026-09-08.

A fit analysis per case is also worth capturing into the fixture, so every candidate model
sees byte-identical requirements: the analysis is INPUT to the tailoring run, not part of
what is measured. Compute one with `GET /jobs/<slug>/match-analysis/stream` (the streaming
route — `POST` returns 504, the chain outruns the proxy) and read it back from the free
`GET /jobs/<slug>/match-analysis`, at `.data.analysis`.

## Tasks 5.1–6.2, and what is known about each

- **5.1** `newAutopilotHarness` (`assistant_autopilot_integration_test.go:43`) already stands
  a Postgres and takes the turn model as a parameter. It hardcodes `MaxSteps: 3`, which does
  NOT bind autopilot — that turn sets its own `TurnConfig{MaxSteps: 30}`.
- **5.2** Tag it `integration && llmlive`. **Swap only `turnM`, never `fitM`** — varying both
  measures two models and attributes the result to one.
- **5.3/5.6** `cvmatch.Compute` and `atscheck.Compare` are pure and free. The report must
  carry each run's tailored CV text: no grader model is called, and whether a CV is good is
  read by whoever reads the report.
- **5.5** Report to `.cache/` (not in git), naming the price table's capture date and the
  profile the cases ran against.

### The trap that will bite 5.2

**Token counts are not in the event stream.** `emitUsage` is called only on the runner's two
terminal paths (`runner.go`), so a thirty-round autopilot reports the LAST call's tokens and
says nothing about the other twenty-nine. Nothing consumes that event today — the web client
lists it under "ignored" (`chat.ts:76`) — so this is not a bug to work around, but it is not
a source either. **Count tokens by wrapping the `assistant.Model` you already substitute.**
That is what `bakeoffTally.observeCall` exists for; `observeEvent` takes the rest.

## Everything else that happened on 2026-09-08

The bake-off stalled because production was broken, and fixing it took the day. All of this
is merged and live, and matters if you touch the fit analysis:

| PR | What |
|---|---|
| #2640 | Stage 1 asks for no deliberation. It was timing out at 90s twice per analysis and failing 55% of real users' requests. |
| #2647 | The adversarial audit gets one attempt, not two — three of three retries had burned an identical 90s to fail identically. |
| #2649 | The audit gets twice the budget for that one attempt, which is exactly what its retry used to cost. It then landed for the first time, needing 160s. |
| #2655 | Stage 2 scores under the evidence rule only Stage 3 knew. Measured: Stage 2 rated skills coverage 95/100/93 and the audit pulled each to 62/70/74 — same direction, same magnitude, every time. |
| #2660 | `cmd/llm-probe` asks the gateway three questions every five minutes and publishes what came back. |
| #2665 | `llm-probe` added to `release.sh`'s worker list. |

Also: #2636/#2637 were merged but undeployed and are now out; three dead Z.ai keys
(`zai-91`, `zai-100`, `zai-126` — 429 Fair Usage Policy) were removed from the live gateway;
two Grafana alert rules are provisioned (`pipeline-llm-alias-refusing`,
`pipeline-llm-probe-stopped`).

**Verifying #2655 needs one more measurement.** After it deployed, two cases were re-run: on
one the audit changed nothing at all (0 of 6 dimensions, overall 75→75, against 5 of 6 and
77→65 before); on the other its correction shrank from −33 to −14. Two cases is two cases.
The method is in `.cache/audit-diff.sh` — the SSE stream carries the pre-audit `dimensions`
event and the post-audit `final` one, so diffing them needs no new instrumentation, and the
raw streams are saved so a mistake in the query costs nothing to correct.

### One measurement lesson worth repeating

The first version of that diff built `from_entries` from `{key, score}` objects.
`from_entries` reads `.value`, which those do not have, so every score compared as `null` and
"0 of 6 moved" came out regardless of the data — which read as "the audit changes nothing,
delete stage 3". The opposite was true.

What caught it was that the number **argued with the code**: `buildAnalysis` computes the
overall purely from the six dimension scores, so a 10-point drop with zero moved dimensions
is impossible. A figure that contradicts the code is almost always a broken measurement.

## Still open

- Stage 2 completed in 1m23s against a 90s deadline in one run and 45s in another. Seven
  seconds of headroom is not a margin.
- The gateway is bimodal — a chain lands in 28s on Gemini and 90s per stage on Z.ai. Every
  deadline here is a lottery until that is addressed.
- `freehire_llm_probe_*` has alert rules but no dashboard panel.
- Four (now five) `rules.yaml.bak-*` files have accumulated on the Grafana host; it warns
  about each on every restart, in the same log where success is confirmed.
