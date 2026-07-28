## Why

Every LLM call in the codebase asks for JSON and gets back a document whose *shape*
is nobody's guarantee. `llms.WithJSONMode()` promises only that the bytes parse; it
says nothing about field types or allowed values. The cost of that gap is already
written into the repo three times over — `internal/resumeextract/flexdecode.go`,
`internal/matchanalysis/flexdecode.go` and `internal/mailclassify/flexdecode.go`
each exist because `encoding/json` aborts the whole unmarshal on the first type
mismatch, so a single `"year": 2019` silently discards an entire parsed CV.

A spike against the production proxy settled that this is fixable at the source
rather than patched at each decoder. With `response_format: json_schema` and
`strict: true`, `proxy.privatclaw.com` constrains both types and enums on
`privateclaw/mid` and `privateclaw/light`; the control run in plain `json_object`
mode reproduced all three defect classes the shims were written against
(`start_year: 2019` as a number, `total_years: 5.9` as a float, and a `level` of
`"senior architect"` outside the vocabulary).

## What Changes

- **New `internal/llmschema` package.** `Of[T any](overrides ...Override) Schema`
  derives a JSON Schema from a Go contract type by reflection (`invopop/jsonschema`)
  and post-processes it for strict mode: `additionalProperties: false` on every
  object, every field in `required`, and optional fields made nullable. A schema
  cannot drift from the type it describes, so a field added to a contract reaches
  the model without a second edit.
- **Enum fields carry their vocabulary into the schema.** `internal/vocab`'s lists
  are attached at the call site through explicit overrides
  (`llmschema.Enum("work_mode", vocab.WorkModeValues)`) rather than struct tags —
  a tag would force `internal/enrich` to restate lists that `vocab` exists to keep
  in one copy.
- **`internal/llm` gains schema-constrained generation.** `GenerateJSON` and
  `GenerateJSONStream` take variadic options, so `llm.WithSchema(name, schema)` is
  opt-in per call and the nine existing call sites compile untouched. The `Client`
  retains its constructor settings and lazily caches one `openai.LLM` per schema,
  because langchaingo binds `ResponseFormat` to the client while `WithJSONMode` is
  a call option.
- **Call sites migrate one at a time**, in order of measurable risk:
  `resumeextract` → `mailclassify` → `telegram` → `enrich`. Each migration is
  separately revertible; enrichment goes last because it carries the heaviest
  production traffic and the longest schema.
- **The existing guards stay.** `Sanitize` (a schema bounds neither string length
  nor array size), `StripJSONFence`, and `Validate` all remain — the last one
  because a proxy or model that quietly stops honouring `json_schema` returns a
  perfectly ordinary 200, and nothing else would notice.
- **`truncInt` stays, and `total_years` stays a string in the schema.** The spike
  found that under a schema the model *rounds* `5.9` to `6`, while
  `flexdecode.go:41` deliberately truncates so a stray fraction cannot inflate a
  candidate's years of experience. Constrained decoding fixes the type, not the
  arithmetic, and here it moves in the wrong direction.
- **`verbatimString` is removed only from migrated call sites**, once each is
  observed to hold.

## Capabilities

### New Capabilities
- `llm-structured-outputs`: schema-constrained JSON generation — how a contract type
  becomes a request schema, how controlled vocabularies reach it, what the client
  guarantees per call, and how the system behaves when a provider does not honour
  the schema.

### Modified Capabilities
- `ai-enrichment`: the requirement that the provider "instructs the LLM with the
  controlled vocabularies … so that the enum fields are constrained" becomes a
  constraint enforced by the request schema rather than by prompt text, and
  "fields not determinable SHALL be omitted" becomes an explicit `null` — strict
  mode admits no absent key.

## Impact

**New:** `internal/llmschema`, a dependency on `github.com/invopop/jsonschema`.

**Modified:** `internal/llm` (`Client` retains settings, caches a model per schema,
variadic options on both generate functions); `internal/enrich`,
`internal/resumeextract`, `internal/mailclassify`, `internal/telegram` (each opts
into a schema); `internal/resumeextract/flexdecode.go` and
`internal/mailclassify/flexdecode.go` (shed `verbatimString`, keep `truncInt`).

**Untouched:** `internal/matchanalysis`, `internal/atscheck`,
`internal/autofillagent` — they keep the current path until the four migrations
above prove out.

**No new env, no migration, no API change.** The user-visible effect is a parse
that stops failing on a type the model chose freely.
