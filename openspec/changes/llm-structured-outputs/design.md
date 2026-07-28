## Context

`internal/llm` exposes two JSON entry points — `GenerateJSON` and
`GenerateJSONStream` — and seven packages sit behind them: `enrich`,
`resumeextract`, `matchanalysis`, `mailclassify`, `telegram`, `atscheck`,
`autofillagent`. Each sends `llms.WithJSONMode()` and unmarshals the returned
string into its own contract. JSON mode guarantees only that the bytes parse.

The gap is already paid for three times. `internal/resumeextract/flexdecode.go`,
`internal/matchanalysis/flexdecode.go` and `internal/mailclassify/flexdecode.go`
each carry hand-written `UnmarshalJSON` shims whose reason is stated plainly in the
first: `encoding/json` aborts the whole unmarshal on the first type mismatch, so one
`"year": 2019` discards an entire structured CV. `internal/flexjson` is a fourth,
shared attempt at the same problem.

A spike against `proxy.privatclaw.com` (2026-07-28) established the fix is available
at the source. Sending `response_format: {"type": "json_schema", "strict": true}`
with a schema, both `privateclaw/mid` and `privateclaw/light` returned `"2019"` as a
string where the schema said string, an integer where it said integer, and a value
inside the `enum` even when the prompt pushed hard toward one outside it. The
control run, identical but in `json_object` mode, returned `start_year: 2019`,
`total_years: 5.9` and `level: "senior architect"` — the three defect classes the
shims exist for, reproduced on demand.

One langchaingo detail shapes the whole design: `llms.WithJSONMode()` is a **call**
option, but `ResponseFormat` is read off the **client**
(`llms/openai/openaillm.go:323`, which overwrites whatever JSON mode set). A single
`llms.Model` therefore carries at most one schema, while this change needs about
ten.

## Goals / Non-Goals

**Goals:**

- A contract type is the only place its request schema is written down.
- Controlled vocabularies from `internal/vocab` reach the model as `enum`
  constraints without being restated anywhere.
- Schema use is opt-in per call, so the nine existing call sites keep compiling and
  migrate one at a time.
- The response is still treated as untrusted: sanitise, validate, strip fences.

**Non-Goals:**

- Removing `internal/flexjson` or the `truncInt` shims. Only `verbatimString`
  becomes redundant, and only where a migration has been observed to hold.
- Migrating `matchanalysis`, `atscheck` or `autofillagent` in this change.
- Any change to prompts beyond dropping the field-shape instructions the schema now
  carries.
- Publishing the schemas as an external artifact.

## Decisions

### Schema derived by reflection, not written by hand

`internal/llmschema` wraps `github.com/invopop/jsonschema` and post-processes its
output for strict mode. A hand-written schema per call site would be a second
description of every contract, free to drift from the first; the drift would be
silent, because a schema missing a field simply stops the model returning it.

Strict mode has two demands that reflection alone does not satisfy, so the
post-process pass is not optional: `additionalProperties: false` must be set on
every object including nested ones, and **every** property must appear in
`required`. Optionality is therefore expressed as a nullable type — a field the Go
contract marks `omitempty` becomes `"type": ["string", "null"]`. This is also why
the `ai-enrichment` delta rewrites "omitted, not guessed" into "null, not guessed":
under a strict schema there is no absent key to omit. Nothing downstream changes,
because `encoding/json` decodes `null` into the same zero value an absent key
produces.

*Alternative considered:* deriving the schema from the existing `cmd/gen-contracts`
reflection walk instead of a new dependency. Rejected — that walk emits TypeScript
and knows nothing of JSON Schema's nullability or enum forms, so it would grow a
second output mode for a job a maintained library already does.

### Vocabularies attached at the call site, not by struct tag

`llmschema.Of[Enrichment](llmschema.Enum("work_mode", vocab.WorkModeValues), …)`.
A `jsonschema:"enum=remote,enum=hybrid"` tag would put the vocabulary in the
contract struct — a literal second copy of a list `internal/vocab` exists to keep in
one place, and one no compiler would catch drifting. An override naming a field the
type lacks is an error, not a no-op, so a renamed field cannot silently shed its
constraint.

### One model per schema, cached on the client

`Client` keeps the settings it was built from and gains a small map from schema name
to `llms.Model`, built lazily under a mutex. The public surface is a variadic option
on the existing functions rather than new methods:

```go
func (c *Client) GenerateJSON(ctx context.Context, system, user string, opts ...GenOption) (string, error)
```

This keeps all nine call sites compiling untouched, which is what makes a one-at-a-
time migration possible. `WithTimeout`'s shallow-copy trick cannot be reused here —
it clones a `Client` around an already-constructed model, and the response format is
baked in at construction.

*Alternative considered:* one `llm.Client` per schema, constructed in `cmd/*`.
Rejected — it pushes schema knowledge into every worker's wiring and multiplies the
tracer and timeout configuration by the number of schemas.

### `total_years` stays a string; `truncInt` stays

The spike's most useful finding is a negative one. Under a schema declaring
`total_years` an integer, the model returned `6` for "5.9 years" — it rounded.
`flexdecode.go:41` truncates on purpose: "a stray 5.9 must not inflate the
candidate's years of experience." Constrained decoding fixes the type and gets the
arithmetic wrong, in the direction that overstates a candidate. So the schema asks
for a string there and the existing decoder keeps doing the arithmetic.

### Validation stays on the constrained path

A proxy or model that stops honouring `json_schema` does not fail — it returns a
normal 200 with a normal JSON body. Every guard that exists today (`Sanitize`,
`Validate`, `StripJSONFence`) therefore stays exactly where it is. The schema is a
first line, never a proof.

## Risks / Trade-offs

**A strict schema degrades extraction quality** → Constrained decoding spends model
attention on form. Measured before/after on the real CVs in
`internal/resume/testdata` and `internal/handler/testdata`, counting populated
fields; `resumeextract` is migrated first precisely because it is the call site
where quality is measurable against fixtures.

**The proxy stops honouring schemas after a model change** → Silent, since the
response is a valid 200. Mitigated by keeping `Validate` and by the migration order:
`enrich`, with the heaviest traffic and the longest schema, goes last, after three
lighter call sites have run in production.

**A nested type's optionality is mis-derived** → A field wrongly marked non-nullable
forces the model to invent a value rather than decline. Covered by a test asserting
the derived schema's `required` and nullable sets against the contract type, not by
inspection.

**A new dependency** → `invopop/jsonschema` is a single-purpose, widely-used library
with no transitive weight of note. The alternative is hand-maintaining ten schemas,
which is the failure mode this change exists to remove.

**`strict: true` may be rejected by a future provider** → The option is per call, so
a call site reverts by deleting one argument. No stored data or API shape depends on
it.

## Migration Plan

1. `internal/llmschema` and the `internal/llm` option, with no call site using them —
   inert on merge.
2. `resumeextract` — measured against CV fixtures before and after.
3. `mailclassify`, then `telegram` — the two that suffered most from type drift.
4. `enrich` last, once the first three have run in production.

Rollback at any step is deleting the `llm.WithSchema(...)` argument from one call
site; nothing persisted depends on the path taken.

## Open Questions

None blocking. Whether `matchanalysis`, `atscheck` and `autofillagent` follow is
deliberately left to a later change, once the four migrations here have produced
evidence either way.
