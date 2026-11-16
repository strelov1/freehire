# The in-app assistant: adding a tool and choosing the turn model

Part of [AGENT-REFERENCE.md](AGENT-REFERENCE.md): sections moved out of it for size.

## Adding a tool

1. Write it in `internal/api/handler/assistant_*_tools.go` as an `assistant.Tool`:
   a name, a one-paragraph description the model reads, a JSON schema, and a
   `Run` that decodes with `assistant.DecodeArgs` and calls the same service the
   HTTP handler calls.
2. Register it in `registry` (`internal/api/handler/assistant_tools.go`) under the presets that should offer it.
3. Return structured data, not prose. Include the fields the model needs to act
   (a vacancy's `public_slug`, not just its title; an achievement's id and whether it may
   be written to a CV, not just its text).
3b. Keep the result small. It is persisted in the transcript and replayed into the model's
   context on EVERY later turn — this is why `get_profile` reports the experience bank's
   shape and counts while `experience_search` returns its content per question.
4. Give errors a message the model can act on — name the invalid value and list
   the valid ones. That message is the model's only path to self-correction.

**Whose spend a turn is.** Every turn goes out on the CALLER's own gateway credential,
resolved by `internal/ai/llmkey` and minted on their first AI call. The runner is built once
at boot and cloned per turn (`Runner.With`), because the credential is per-user and the
bounds are not. The call is tagged `feature:assistant` **and** `preset:<preset>`: the
gateway files one spend row per tag, and a rehearsal, an unattended tailoring run and a
question cost wildly different amounts, so the preset is what makes them comparable.

Two rules hold the whole thing up, and both fail silently if broken:

- **Attribution never costs a turn.** An unmintable credential, an unreachable admin API
  and a rejected key all fall back to the service credential and the turn completes. A key
  the gateway has forgotten is additionally retried once and reported, in `internal/platform/llm`'s
  transport — the only layer that sees a status code rather than langchaingo's error prose.
- **A re-credentialed client must not share the schema-model cache.** See the comment on
  `modelCache` in `internal/platform/llm/schema.go`. Sharing it sends one user's schema-bound call
  out on another user's key, successfully.

CV tailoring has no tag of its own on purpose: `/me/cvs/tailor` makes no model call: it
mints a CV and debits credits, and the work is a turn under the `tailor` preset. A second
tag would double-count one spend.

## Choosing the turn model: the bake-off

Which model answers a turn is decided by measurement, not by a price page, and the
measurement is a `//go:build integration && llmlive` test:
`internal/api/handler/assistant_bakeoff_run_llmlive_test.go`.

```bash
BAKEOFF_MODELS=vendor/model-a,vendor/model-b \
LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=… \
  go test -tags=integration,llmlive ./internal/api/handler/ -run TestBakeoff -timeout 3h -v
```

It runs the **tailoring autopilot** — the assistant's most expensive turn shape — once per
(candidate model, case) against a fresh Postgres, and writes a ranked table plus a JSON
report under `.cache/`. Needs Docker, `typst` and `pdftotext`, and the local profile
fixture (see `bakeoffProfileFixture`: a real CV, deliberately not in this repository).

**Why a per-million price does not answer the question.** Two things decide what a turn
costs here, and neither is on a price page:

- **Rounds are a multiplier, not an addend.** The transcript is replayed into context every
  round, so round N carries rounds 1..N-1 with it. A model needing fourteen rounds where
  another needs five costs far more than its token price says.
- **Whether the provider serves the replayed prefix from cache decides the rest.** #2633
  measured 20k tokens a turn being re-read at full price because the history window slid by
  one message. The bake-off reads `llm.Usage.CachedInput` per round and reports the share.

The cache verdict has **three** states and not two: langchaingo writes the cached key
unconditionally from a zero-valued struct, so "the provider reported nothing" and "nothing
was cached" both arrive as `0`. A single-round run is therefore `inconclusive` — there was
no earlier round for a cache to have held — and only a multi-round run all reporting zero
is `no cache observed`.

**What it does not do.** It renders no verdict on whether a tailored CV is any good, and
calls no grader model. It ranks on `cvmatch` and `atscheck` — free, pure, and the same
numbers the candidate already sees in the workspace — and emits every run's tailored CV
text beside its vacancy so a reader judges the rest. See
`openspec/changes/assistant-model-bakeoff/design.md` for why a judge model was rejected.

**Only the turn model varies.** The fit chain stays pinned to `LLM_MODEL`, and the test
asserts it: swapping both would measure two models and attribute the result to one.
