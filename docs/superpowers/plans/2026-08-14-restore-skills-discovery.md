# Restore Skills Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-request `skills` in the enrichment LLM prompt/schema so `Enrichment.Skills` starts populating again, as a raw discovery signal for future `internal/skilltag` dictionary expansion.

**Architecture:** Two-line-ish edit to `internal/enrich/langchain.go` (prompt) and `internal/enrich/schema.go` (request schema), plus a comment fix in `internal/enrich/enrichment.go`. No new files, no struct/DB change, no `enrich.Version` bump.

**Tech Stack:** Go, `internal/enrich` package (existing LLM structured-output pipeline).

## Global Constraints

- Forward-only: do NOT bump `enrich.Version`; do NOT write any backfill/re-enrichment code. (proposal.md, design.md)
- Reuse the existing `Enrichment.Skills []string` field (`json:"skills"`) — do NOT add a new field, table, or JSON key. (design.md)
- Only `skills` is restored. Do NOT re-add `work_mode`, `seniority`, `category`, `employment_type`, `education_level`, `english_level`, `posting_language`, or `experience_years_min` to the prompt or schema. (proposal.md)
- No aggregation/dedup pipeline in this change — only the raw-capture restart. (design.md)
- OpenSpec change `restore-skills-discovery` (already committed at `f2fb432d`) is the spec of record; `openspec change validate restore-skills-discovery --strict` must keep passing after code changes land.

---

### Task 1: Prompt — re-request `skills` in `buildSystemPrompt`

**Files:**
- Modify: `internal/enrich/langchain.go:128-134` (the "Other keys" line inside `buildSystemPrompt`)
- Test: `internal/enrich/langchain_test.go`

**Interfaces:**
- Consumes: nothing new — `buildSystemPrompt(askGeo bool) string` keeps its existing signature.
- Produces: the string returned by `buildSystemPrompt` now contains the substring `"skills"` (and the fuller phrase `"skills (array of lowercase tokens, e.g. go, postgresql)"`) regardless of `askGeo`. `Task 2` and `Task 3` depend on this exact phrase existing in the prompt.

- [ ] **Step 1: Edit `TestSystemPromptOmitsDictBackedFacets` to drop `skills`**

Current test body (`internal/enrich/langchain_test.go:60-71`):

```go
func TestSystemPromptOmitsDictBackedFacets(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, f := range []string{
		"work_mode", "seniority", "category", "skills",
		"employment_type", "education_level", "english_level",
		"posting_language", "experience_years_min",
	} {
		if strings.Contains(p, f) {
			t.Errorf("prompt must not request dict-backed facet %q (served from dictionaries), got:\n%s", f, p)
		}
	}
}
```

Replace the field list (drop `"skills"` — it is no longer a dict-backed-and-unrequested facet):

```go
func TestSystemPromptOmitsDictBackedFacets(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, f := range []string{
		"work_mode", "seniority", "category",
		"employment_type", "education_level", "english_level",
		"posting_language", "experience_years_min",
	} {
		if strings.Contains(p, f) {
			t.Errorf("prompt must not request dict-backed facet %q (served from dictionaries), got:\n%s", f, p)
		}
	}
}
```

- [ ] **Step 2: Add `skills` to `TestSystemPromptKeepsServedAndHybridFields`**

Current test body (`internal/enrich/langchain_test.go:78-96`):

```go
func TestSystemPromptKeepsServedAndHybridFields(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, f := range []string{
		"summary",
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"visa_sponsorship", "timezone_note",
		"company_type", "company_size", "domains",
		"relocation", "countries", "regions",
	} {
		if !strings.Contains(p, f) {
			t.Errorf("prompt must still request served/hybrid field %q, got:\n%s", f, p)
		}
	}
	if !strings.Contains(p, "exactly one of the allowed values") {
		t.Errorf("served enum fields must keep the strict instruction")
	}
	if !strings.Contains(p, "concise lowercase label of your own") {
		t.Errorf("countries/regions must keep the novel own-label allowance")
	}
}
```

Add `"skills"` to the field list:

```go
func TestSystemPromptKeepsServedAndHybridFields(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, f := range []string{
		"summary",
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"visa_sponsorship", "timezone_note",
		"company_type", "company_size", "domains",
		"relocation", "countries", "regions", "skills",
	} {
		if !strings.Contains(p, f) {
			t.Errorf("prompt must still request served/hybrid field %q, got:\n%s", f, p)
		}
	}
	if !strings.Contains(p, "exactly one of the allowed values") {
		t.Errorf("served enum fields must keep the strict instruction")
	}
	if !strings.Contains(p, "concise lowercase label of your own") {
		t.Errorf("countries/regions must keep the novel own-label allowance")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/enrich/... -run TestSystemPromptOmitsDictBackedFacets -v` — Expected: PASS (skills was never in the prompt yet, so its absence still holds; this one should already pass, confirming step 1 didn't break anything).

Run: `go test ./internal/enrich/... -run TestSystemPromptKeepsServedAndHybridFields -v` — Expected: FAIL with `prompt must still request served/hybrid field "skills"`.

- [ ] **Step 4: Edit `buildSystemPrompt` to re-request `skills`**

In `internal/enrich/langchain.go`, find this block (currently lines 128-134):

```go
	b.WriteString("\nOther keys (null when unstated): ")
	b.WriteString("visa_sponsorship (boolean), ")
	if askGeo {
		b.WriteString("countries (array of ISO 3166-1 alpha-2), ")
	}
	b.WriteString("cities (array of strings), timezone_note (string), ")
	b.WriteString("salary_min (int), salary_max (int), salary_currency (ISO 4217).\n")
```

Replace the last line to add `skills` after the salary fields (matching the pre-`enrich-prompt-trim` wording exactly):

```go
	b.WriteString("\nOther keys (null when unstated): ")
	b.WriteString("visa_sponsorship (boolean), ")
	if askGeo {
		b.WriteString("countries (array of ISO 3166-1 alpha-2), ")
	}
	b.WriteString("cities (array of strings), timezone_note (string), ")
	b.WriteString("salary_min (int), salary_max (int), salary_currency (ISO 4217), ")
	b.WriteString("skills (array of lowercase tokens, e.g. go, postgresql).\n")
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/enrich/... -run 'TestSystemPromptOmitsDictBackedFacets|TestSystemPromptKeepsServedAndHybridFields' -v`

Expected: both PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/i_strelov/Projects/hire
git add internal/enrich/langchain.go internal/enrich/langchain_test.go
git commit -m "enrich: re-request skills in the LLM prompt as a discovery facet"
```

---

### Task 2: Schema — stop omitting `skills` from the request schema

**Files:**
- Modify: `internal/enrich/schema.go:23-27` (`unaskedFields`)
- Test: `internal/enrich/schema_test.go`

**Interfaces:**
- Consumes: `requestSchema(askGeo bool) (llmschema.Schema, string, error)` (unchanged signature) and `schemaProps(t *testing.T, askGeo bool) map[string]any` test helper (unchanged, `internal/enrich/schema_test.go:10-23`).
- Produces: `schemaProps(t, true)["skills"]` now exists. `unaskedFields` no longer contains `"skills"` — `Task 3`'s comment fix references this list.

- [ ] **Step 1: Add `skills` to `TestRequestSchema_CarriesTheFieldsThePromptAsksFor`**

Current test body (`internal/enrich/schema_test.go:67-80`):

```go
func TestRequestSchema_CarriesTheFieldsThePromptAsksFor(t *testing.T) {
	props := schemaProps(t, true)

	for _, field := range []string{
		"summary", "relocation", "visa_sponsorship", "cities", "timezone_note",
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"domains", "company_type", "company_size", "regions", "countries",
	} {
		if _, ok := props[field]; !ok {
			t.Errorf("schema is missing %q, so the model would stop returning it", field)
		}
	}
}
```

Add `"skills"` to the field list:

```go
func TestRequestSchema_CarriesTheFieldsThePromptAsksFor(t *testing.T) {
	props := schemaProps(t, true)

	for _, field := range []string{
		"summary", "relocation", "visa_sponsorship", "cities", "timezone_note",
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"domains", "company_type", "company_size", "regions", "countries", "skills",
	} {
		if _, ok := props[field]; !ok {
			t.Errorf("schema is missing %q, so the model would stop returning it", field)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/enrich/... -run TestRequestSchema_CarriesTheFieldsThePromptAsksFor -v`

Expected: FAIL with `schema is missing "skills"`.

- [ ] **Step 3: Remove `skills` from `unaskedFields`**

In `internal/enrich/schema.go`, current:

```go
var unaskedFields = []string{
	"work_mode", "employment_type",
	"seniority", "english_level", "education_level", "skills",
	"category", "experience_years_min", "posting_language",
}
```

Replace with:

```go
var unaskedFields = []string{
	"work_mode", "employment_type",
	"seniority", "english_level", "education_level",
	"category", "experience_years_min", "posting_language",
}
```

- [ ] **Step 4: Run the full schema test file to verify everything still passes**

Run: `go test ./internal/enrich/... -run TestRequestSchema -v`

Expected: PASS, including `TestRequestSchema_DoesNotAskForDictionaryCoveredFacets` (it iterates `unaskedFields` directly, so removing `"skills"` from that slice automatically stops it being checked there — no edit needed to that test) and the newly-updated `TestRequestSchema_CarriesTheFieldsThePromptAsksFor`.

- [ ] **Step 5: Confirm `unaskedFields`'s doc comment needs no wording change**

Read `internal/enrich/schema.go:18-22` — the comment above `unaskedFields`:

```go
// unaskedFields are contract fields the prompt deliberately does not request, so the
// schema must not either. Under strict mode every property is required — leaving one
// in would order the model to produce a value that `jobview` then discards, which is
// the token burn `enrich-prompt-trim` removed. They are served from the deterministic
// dictionaries (`internal/jobderive`), not the LLM.
```

This comment describes the slice generically and never names `skills` (unlike
`enrichment.go`'s `Validate` comment, fixed in Task 3) — it stays accurate once
`skills` is removed from the slice in Step 3, describing the eight remaining fields.
No edit needed here; this step exists so the change is a deliberate "checked, no
action" rather than a silently skipped one.

- [ ] **Step 6: Commit**

```bash
cd /Users/i_strelov/Projects/hire
git add internal/enrich/schema.go internal/enrich/schema_test.go
git commit -m "enrich: stop omitting skills from the request schema"
```

---

### Task 3: Comment fix — `Validate`'s doc comment no longer describes `skills` as unrequested

**Files:**
- Modify: `internal/enrich/enrichment.go:113-121` (the doc comment on `Validate`)

**Interfaces:**
- Consumes: nothing (comment-only change).
- Produces: nothing new — no behavior, signature, or test changes. `Validate`'s actual logic is untouched (it already never validates `skills`, which stays correct).

This task has no RED/GREEN cycle — it is a comment-only fix with nothing to assert in a test. Verify by reading, not running.

- [ ] **Step 1: Confirm the current wording is now stale**

Read `internal/enrich/enrichment.go:113-121`:

```go
// Validate checks every SERVED enum field against its controlled vocabulary and
// returns an error identifying the first offending field. Empty (absent) fields
// pass — every field is optional. Non-enum fields (ISO codes, free text, numbers,
// skills) are unconstrained here. The dictionary-covered facets (work_mode,
// seniority, category, regions, employment_type, education_level, english_level,
// plus the non-enum countries/skills) are deliberately NOT validated: they are served from
// the deterministic dictionaries (dict-only), so the LLM's values for them are
// unserved discovery material and an out-of-vocabulary value is captured raw
// rather than rejected.
func (e Enrichment) Validate() error {
```

After Task 1/Task 2, `skills` IS requested from the LLM again (it is no longer "dict-only" in the sense this comment implies for the other six facets it's grouped with). Left as-is, a reader would wrongly conclude `skills` is never sent to the model.

- [ ] **Step 2: Replace the comment**

Replace lines 113-121 with:

```go
// Validate checks every SERVED enum field against its controlled vocabulary and
// returns an error identifying the first offending field. Empty (absent) fields
// pass — every field is optional. Non-enum fields (ISO codes, free text, numbers,
// skills) are unconstrained here. The dictionary-covered facets (work_mode,
// seniority, category, regions, employment_type, education_level, english_level,
// plus the non-enum countries) are deliberately NOT validated: they are served from
// the deterministic dictionaries (dict-only), so the LLM's values for them are
// unserved discovery material and an out-of-vocabulary value is captured raw
// rather than rejected. skills is requested from the LLM too (restore-skills-discovery)
// but is likewise never validated — it has no closed vocabulary to check against, so
// whatever the model returns is captured raw, same as countries/regions.
func (e Enrichment) Validate() error {
```

- [ ] **Step 3: Sweep for any other stale `skills` reference**

Run: `grep -n "skills" internal/enrich/*.go | grep -v _test.go`

Expected matches, all already correct after Steps 1-2 and Task 2, nothing else:

- `internal/enrich/enrichment_unmarshal.go:107` — the `Skills sliceOrWrap` unmarshal
  field (unrelated to this change, always correct).
- `internal/enrich/enrichment.go:79` — the `Skills []string` struct field itself.
- Two lines inside the new `Validate` comment from Step 2 (the "numbers, skills)
  are unconstrained" line and the new "skills is requested from the LLM too" line).
- **`internal/enrich/schema.go` must have ZERO matches** — `unaskedFields` no
  longer contains `"skills"` (Task 2 Step 3) and its doc comment never named
  `skills` (Task 2 Step 5), so nothing in that file should mention it.

If any other file/line mentions `skills` in a way that still claims it is
dict-only/unrequested, fix it the same way as Step 2 before moving on — this step
exists to catch anything this plan's authoring pass missed.

- [ ] **Step 4: Verify the package still builds**

Run: `go build ./internal/enrich/... && go vet ./internal/enrich/...`

Expected: no errors (comment-only change).

- [ ] **Step 5: Commit**

```bash
cd /Users/i_strelov/Projects/hire
git add internal/enrich/enrichment.go
git commit -m "enrich: fix Validate's doc comment now that skills is requested again"
```

---

### Task 4: Full verification and OpenSpec sync check

**Files:** none modified — verification only.

**Interfaces:** none.

- [ ] **Step 1: Run the full `internal/enrich` package test suite**

Run: `go test ./internal/enrich/... -v`

Expected: all tests PASS, including every test named in Tasks 1-2 plus the untouched tests (`TestValidateAcceptsOutOfVocabDiscoveryFacets`, `TestSanitizeDropsOutOfVocabValues`, etc. in `enrichment_test.go`) — none of those should need edits, since `Sanitize`/`Validate` behavior is unchanged.

- [ ] **Step 2: Run the whole-repo build and vet**

Run: `go build ./... && go vet ./...`

Expected: no errors.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`

Expected: all packages PASS. (If any unrelated package fails for reasons unconnected to this change, note it but do not attempt to fix it as part of this task — out of scope per the design's Non-Goals.)

- [ ] **Step 4: Re-validate the OpenSpec change**

Run: `openspec change validate restore-skills-discovery --strict` (from `/Users/i_strelov/Projects/hire`)

Expected: `Change "restore-skills-discovery" is valid` (already confirmed once when the proposal was written; re-run now that code matches the spec).

- [ ] **Step 5: Confirm no version bump or backfill code was introduced**

Run: `git diff f2fb432d..HEAD --stat` (from `/Users/i_strelov/Projects/hire`) and confirm the file list is exactly: `internal/enrich/langchain.go`, `internal/enrich/langchain_test.go`, `internal/enrich/schema.go`, `internal/enrich/schema_test.go`, `internal/enrich/enrichment.go`.

Also run: `git diff f2fb432d..HEAD -- internal/enrich/enrichment.go internal/enrich | grep -n "Version"`

Expected: no output (or only unrelated matches unconnected to `enrich.Version`) —
confirms the Global Constraint ("do NOT bump `enrich.Version`; do NOT write any
backfill/re-enrichment code") held throughout implementation.

- [ ] **Step 6: Final commit / wrap-up**

Confirm `git log --oneline -5` shows the three commits from Tasks 1-3 plus the earlier proposal commit (`f2fb432d`), all still on the current branch (`harvest-echojobs-orphans-retry-ratelimited` at plan-writing time — confirm this is still the intended branch before pushing, per the note already raised with the user about the branch name).

No further commit needed at this step — it is a verification-only task.
