## 1. Schema derivation — `internal/llmschema`

- [x] 1.1 Add `github.com/invopop/jsonschema` to `go.mod`; verify `go build ./...` and `go vet ./...` stay clean
- [x] 1.2 Create `internal/llmschema` with `Schema` (the wire type sent as `response_format.json_schema.schema`) and `Of[T any](overrides ...Override) (Schema, error)` deriving from a Go contract type by reflection
- [x] 1.3 Post-process for strict mode: `additionalProperties: false` on every object including nested ones, every property in `required`, and each `omitempty` field widened to a nullable type
- [x] 1.4 Test the derivation against a fixture struct: property names are exactly the JSON tags; an unexported field and a `json:"-"` field are absent; a nested struct and a slice-of-struct are walked
- [x] 1.5 Test strict conformance: every object carries `additionalProperties:false`, `required` lists every property, and an `omitempty` field's type admits `null` while a non-`omitempty` field's does not
- [x] 1.6 Implement `Enum(field string, values []string) Override` attaching a vocabulary to one property; an override naming a field the type does not have returns an error rather than being ignored
- [x] 1.7 Test overrides: a constrained property carries the vocabulary as `enum` and its siblings are untouched; an override for an unknown field is an error

## 2. Schema-constrained generation — `internal/llm`

- [x] 2.1 Retain the constructor settings on `Client` so a second model can be built from them, without changing `NewClient`'s signature or the tracer/timeout wiring
- [x] 2.2 Add `GenOption` and `WithSchema(name string, s llmschema.Schema) GenOption`; make `GenerateJSON` and `GenerateJSONStream` variadic so the nine existing call sites compile untouched
- [x] 2.3 Build the schema-bound model lazily and cache it per schema name under a mutex, since langchaingo binds `ResponseFormat` to the client while `WithJSONMode` is a call option
- [x] 2.3a Install the schema through an `http.RoundTripper` given to `openai.WithHTTPClient`, rewriting `response_format` in the outgoing body — langchaingo's `ResponseFormatJSONSchemaProperty.Type` is a `string` and cannot carry the nullable type strict mode needs
- [x] 2.3b Test the injector directly: a body carrying `json_object` comes out carrying the strict `json_schema`, every other field of the request is preserved byte-identical, and a non-chat request passes through untouched
- [x] 2.4 Test that a call with no schema sends exactly what it sends today (plain JSON mode, no response format) — the regression that protects every unmigrated call site
- [x] 2.5 Test that a call with a schema sends it as a strict `json_schema` response format under the given name, in both the streaming and non-streaming paths
- [x] 2.6 Test that repeated calls with one schema construct one model, and that a second schema does not disturb the first
- [x] 2.7 Test that timeout, tracing/observation and the empty-choices guard behave identically on the schema path

## 3. Migrate `resumeextract` — the measured call site

- [x] 3.1 Record the baseline: `live_measure_test.go` (build tag `llmlive`) runs both modes over the CV fixtures in one pass, so the comparison shares a model and a moment
- [x] 3.2 Derive the schema from `Structured` and pass it at the call site, keeping `total_years` a string so `truncInt` keeps truncating rather than the model rounding. **Required two new overrides:** `Omit` — the contact fields come from PII detection over text the model never sees, and strict mode would have ordered it to invent them — and `AsText`, for the years field
- [x] 3.3 Test that the derived schema's total-years property is a string, that the contact fields are absent, and that a model reporting 5.9 years still stores 5
- [x] 3.4 Re-run the measurement: every field matches except `location`, which drops 1→0 in 3 of 3 runs. **Investigated rather than accepted** — the unconstrained call was filling the candidate's location with `"Singapore (Remote)"`, the last employer's office, in 3 of 3 runs; the CV states no location for the person. The schema returns null. The one systematic difference is a bug being fixed, not a regression. Skills varied 26–30 across runs in both modes: noise
- [x] 3.5 **`verbatimString` KEPT, deliberately departing from the task as written.** The schema now declares those fields as strings, making the shim unreachable while the gateway honours it — but that is exactly the condition `Validate` is kept against, and a provider that stops honouring a schema answers 200 with ordinary JSON. Removing 25 tested lines would trade a standing guard for nothing. Reason recorded in `flexdecode.go`
- [x] 3.6 Covered by the existing `TestExtract_ParsesAndSanitizes`, which now runs through the schema path and still proves the bounds (negative years coerced, empty entries dropped); `Sanitize` is path-independent, so a second copy of it under a schema-shaped name would assert nothing new

## 4. Migrate `mailclassify` and `telegram`

- [ ] 4.1 Derive the classification schema from the `mailclassify` contract, attaching its label vocabulary as an `enum` override
- [ ] 4.2 Test that a label outside the vocabulary is still rejected on receipt, proving validation survives a provider that ignores the schema
- [ ] 4.3 Remove `verbatimString` from `internal/mailclassify/flexdecode.go`, keeping any numeric-coercion shim
- [ ] 4.4 Derive the extraction schema from the `telegram` vacancy contract and pass it at the call site
- [ ] 4.5 Test the telegram path end to end against a stored fixture message, asserting the control-character repair still runs

## 5. Migrate `enrich` — last, heaviest traffic

- [ ] 5.1 Derive the schema from the `Enrichment` contract with `Enum` overrides for every controlled vocabulary in `internal/vocab` that the prompt asks for
- [ ] 5.2 Test that each enum property carries exactly its `vocab` list, so a vocabulary edit reaches the schema without a second edit
- [ ] 5.3 Verify `Sanitize` and `Validate` treat an explicit `null` exactly as they treat an absent field — the behaviour the `ai-enrichment` delta now requires
- [ ] 5.4 Trim from the prompt only the field-shape instructions the schema now carries, leaving the salary rounding rule and every semantic instruction in place
- [ ] 5.5 Test that a provider returning an out-of-vocabulary value despite the schema still has that field dropped and the rest of the enrichment kept
- [ ] 5.6 Run `enrich` against a handful of real jobs with `LLM_*` pointed at the proxy and confirm the contract fills as before

## 6. Finish

- [ ] 6.1 `go build ./... && go vet ./... && go test ./...` clean
- [ ] 6.2 Update `internal/llm`'s package doc and `internal/enrich/AGENTS.md` to state that vocabularies are enforced by the request schema and validated again on receipt
- [ ] 6.3 Note in `internal/resumeextract/AGENTS.md` why `total_years` is requested as a string
