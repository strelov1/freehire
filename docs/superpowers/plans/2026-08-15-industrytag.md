# internal/industrytag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `companies.industries` from a two-vocabulary display field into a curated, dict-only facet users can filter on.

**Architecture:** A new `internal/industrytag` package owns an alias→canonical map, exactly as `internal/skilltag` owns skills. Every writer (`cmd/import-yc`, a new import worker) passes raw tags through `Canonicalize` before they reach the column, and unknown tags emit nothing. `UpsertYCCompany` stops replacing columns it does not own. The facet is wired into Meilisearch and the UI last, once the data is clean.

**Tech Stack:** Go 1.25, sqlc, pgx, Meilisearch, SvelteKit.

Spec: `docs/superpowers/specs/2026-08-15-industrytag-design.md`

## Global Constraints

- English only in code, comments, identifiers and commit messages.
- Canonical values are lowercase slugs: `[a-z0-9-]+`, no spaces. Display text comes from `Label()`.
- Dict-only: an unrecognized tag emits nothing. Never guess, never mechanically Title-Case unknown input.
- Before committing any `*.go`: `gofmt -w` those paths, then `go vet ./...` and `go test ./...`.
- Before pushing: `go vet -tags=integration ./...`.
- Never write to `companies` columns owned by `RefreshCompanyFacets`: `job_count`, `regions`, `countries`, `domains`, `company_types`, `company_sizes`, `remote_regions`.
- `internal/db/` is generated. Edit `internal/db/queries/*.sql`, then run `make sqlc`.

---

### Task 1: The `industrytag` package

**Files:**
- Create: `internal/industrytag/industrytag.go`
- Create: `internal/industrytag/dictionaries.go`
- Create: `internal/industrytag/labels.go`
- Test: `internal/industrytag/industrytag_test.go`
- Test: `internal/industrytag/dictionaries_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `industrytag.Canonicalize(tags []string) []string` (sorted, de-duplicated canonical slugs); `industrytag.Label(canonical string) string`; `industrytag.Canonicals() []string`.

Seed data for `dictionaries.go` — 100 canonical values, 155 aliases — is generated at
`scratch/company-dump/industrytag-seed.go.txt`. Copy it in, then hand-check it: the
generator merged what it could see, and a dictionary is meant to be owned by hand.

- [ ] **Step 1: Write the failing test**

```go
package industrytag

import (
	"reflect"
	"testing"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"hyphenated and spaced forms of one industry collapse",
			[]string{"Financial-Services", "Financial Services"},
			[]string{"financial-services"}},
		{"semantic synonyms collapse",
			[]string{"AI", "Artificial Intelligence"},
			[]string{"ai"}},
		{"unknown tags emit nothing",
			[]string{"CTRM-(Commodity-Trading-and-Risk-Management)"},
			[]string{}},
		{"an already-canonical slug is accepted",
			[]string{"medical-devices"},
			[]string{"medical-devices"}},
		{"output is sorted and de-duplicated",
			[]string{"Retail", "AI", "retail"},
			[]string{"ai", "retail"}},
		{"blank input yields empty, not nil-panic",
			[]string{"", "   "},
			[]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Canonicalize(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Canonicalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/industrytag/ -run TestCanonicalize -v`
Expected: FAIL — build error, `undefined: Canonicalize`.

- [ ] **Step 3: Write the implementation**

`internal/industrytag/industrytag.go`:

```go
// Package industrytag resolves free-text industry labels to a curated canonical
// vocabulary for companies.industries.
//
// It is the finer level beneath vocab.DomainValues: domains names ~20 coarse
// verticals derived from job enrichment, and 42% of tagged companies land in its
// "other" bucket. This dictionary names what those companies actually do.
//
// Dict-only, like internal/skilltag and internal/location: a tag outside the
// dictionary produces nothing. Guessing would put a third spelling into a column
// that already mixes two.
package industrytag

import (
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalize folds a raw tag to the lookup key used by the alias map: lowercase,
// "&" spelled out, every other run of punctuation or space collapsed to one hyphen.
// So "Food & Beverage", "food-and-beverage" and "Food and Beverage" share a key.
func normalize(tag string) string {
	s := strings.ToLower(strings.TrimSpace(tag))
	s = strings.ReplaceAll(s, "&", "and")
	return strings.Trim(nonAlnum.ReplaceAllString(s, "-"), "-")
}

// Canonicalize maps raw industry tags to canonical slugs, dropping anything the
// dictionary does not know. The result is sorted and de-duplicated so a caller can
// write it straight into a text[] column and compare runs for equality.
func Canonicalize(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		key := normalize(tag)
		if key == "" {
			continue
		}
		if c, ok := aliases[key]; ok {
			seen[c] = struct{}{}
			continue
		}
		// An already-canonical value passes through, which makes re-running the
		// normalization worker over its own output a no-op.
		if _, ok := labels[key]; ok {
			seen[key] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}
```

Add `"slices"` to the import block.

`internal/industrytag/dictionaries.go`: package clause, then the generated
`aliases` map from `scratch/company-dump/industrytag-seed.go.txt`.

`internal/industrytag/labels.go`:

```go
package industrytag

import "slices"

// labels is the canonical set and its display text. A canonical slug with no entry
// here does not exist — Canonicalize checks this map to accept pass-through slugs,
// and the invariant test asserts every alias target appears.
var labels = map[string]string{
	"adtech":                "AdTech",
	"aerospace-and-defense": "Aerospace and Defense",
	"ai":                    "AI",
	"banking":               "Banking",
	"biotech":               "Biotech",
	// ... the full map — one entry per canonical — is the second half of
	// scratch/company-dump/industrytag-seed.go.txt, generated alongside the aliases.
	// Acronyms are hand-cased there (AI, SaaS, IT Services, HR Tech, IoT); check
	// the rest for ones the generator title-cased wrongly.
}

// Label returns the display text for a canonical slug, or the slug itself when it
// is unknown, so a stale stored value renders as something rather than blank.
func Label(canonical string) string {
	if l, ok := labels[canonical]; ok {
		return l
	}
	return canonical
}

// Canonicals returns every canonical slug, sorted — for the facet options endpoint.
func Canonicals() []string {
	out := make([]string, 0, len(labels))
	for c := range labels {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/industrytag/ -run TestCanonicalize -v`
Expected: PASS, all six subtests.

- [ ] **Step 5: Write the dictionary invariant tests**

`internal/industrytag/dictionaries_test.go`:

```go
package industrytag

import (
	"regexp"
	"testing"
)

var slugForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestEveryCanonicalIsASlug(t *testing.T) {
	for c := range labels {
		if !slugForm.MatchString(c) {
			t.Errorf("canonical %q is not a slug", c)
		}
	}
}

func TestEveryAliasTargetExists(t *testing.T) {
	for alias, canonical := range aliases {
		if _, ok := labels[canonical]; !ok {
			t.Errorf("alias %q points at unknown canonical %q", alias, canonical)
		}
	}
}

func TestEveryAliasKeyIsNormalized(t *testing.T) {
	// A key the normalizer can never produce is dead weight that silently never
	// matches — the failure mode this dictionary exists to prevent.
	for alias := range aliases {
		if got := normalize(alias); got != alias {
			t.Errorf("alias key %q is not in normal form (normalize gives %q)", alias, got)
		}
	}
}

func TestEveryCanonicalHasALabel(t *testing.T) {
	for c, l := range labels {
		if l == "" {
			t.Errorf("canonical %q has an empty label", c)
		}
	}
}
```

- [ ] **Step 6: Run the invariant tests and fix the dictionary until they pass**

Run: `go test ./internal/industrytag/ -v`
Expected: PASS. Failures here mean the seed needs editing, not the code — fix
`dictionaries.go` / `labels.go`.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/industrytag/
go vet ./... && go test ./...
git add internal/industrytag/
git commit -m "Add internal/industrytag: curated industry vocabulary"
```

---

### Task 2: Stop `UpsertYCCompany` from clobbering other sources

**Files:**
- Modify: `internal/db/queries/companies.sql:239-270` (`UpsertYCCompany`)
- Modify: `cmd/import-yc/main.go:185-206` (`recordToParams`)
- Test: `internal/db/company_yc_integration_test.go`

**Interfaces:**
- Consumes: `industrytag.Canonicalize` from Task 1.
- Produces: no signature change — `UpsertYCCompanyParams` keeps its fields.

- [ ] **Step 1: Write the failing integration test**

Append to `internal/db/company_yc_integration_test.go` (the file already carries
`//go:build integration`):

```go
func TestUpsertYCCompanyPreservesExistingValues(t *testing.T) {
	ctx := context.Background()
	q := New(testPool(t))

	// A row another source already enriched: its own tagline, its own website,
	// and an industry YC does not know about.
	_, err := testPool(t).Exec(ctx, `
		INSERT INTO companies (slug, name, tagline, company_info, industries)
		VALUES ('acme-test', 'Acme', 'Our own tagline',
		        '{"website":"https://ours.example"}'::jsonb, '{"logistics"}')`)
	if err != nil {
		t.Fatal(err)
	}

	if err := q.UpsertYCCompany(ctx, UpsertYCCompanyParams{
		Slug:        "acme-test",
		Name:        "Acme",
		Tagline:     pgtype.Text{String: "YC tagline", Valid: true},
		CompanyInfo: []byte(`{"yc_url":"https://ycombinator.com/acme"}`),
		Industries:  []string{"fintech"},
		YcBatch:     []string{"W20"},
		YcStatus:    []string{}, YcStage: []string{}, YcFlags: []string{},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := q.GetCompany(ctx, "acme-test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Tagline.String != "Our own tagline" {
		t.Errorf("tagline = %q, want the existing one preserved", got.Tagline.String)
	}
	var info map[string]any
	if err := json.Unmarshal(got.CompanyInfo, &info); err != nil {
		t.Fatal(err)
	}
	if info["website"] != "https://ours.example" {
		t.Errorf("website = %v, want the existing one preserved", info["website"])
	}
	if info["yc_url"] == nil {
		t.Error("yc_url missing: the YC keys should still have been merged in")
	}
	if !slices.Equal(got.Industries, []string{"fintech", "logistics"}) {
		t.Errorf("industries = %q, want the union sorted", got.Industries)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -tags=integration ./internal/db/ -run TestUpsertYCCompanyPreservesExistingValues -v`
Expected: FAIL — tagline is `"YC tagline"`, website is gone, industries are `{fintech}`.

- [ ] **Step 3: Change the query**

In `internal/db/queries/companies.sql`, replace the three offending lines of the
`ON CONFLICT (slug) DO UPDATE SET` block:

```sql
    -- COALESCE, not EXCLUDED: a tagline written by another source outranks the YC
    -- one-liner. NULLIF folds '' into NULL so an empty string counts as absent.
    tagline         = COALESCE(NULLIF(companies.tagline, ''), EXCLUDED.tagline),
    -- Operand order is load-bearing: a || b keeps b on key collision, so the YC
    -- keys fill gaps while anything already stored wins.
    company_info    = EXCLUDED.company_info || companies.company_info,
    -- Union, not replace: this column now has more than one writer.
    industries      = ARRAY(
        SELECT DISTINCT x
        FROM unnest(companies.industries || EXCLUDED.industries) AS x
        WHERE x <> ''
        ORDER BY x
    ),
```

Update the comment above the query: it currently claims the company-info columns
are "refreshed", which stops being true.

- [ ] **Step 4: Regenerate and canonicalize YC's industries**

Run: `make sqlc`

In `cmd/import-yc/main.go`, inside `recordToParams`, replace
`Industries: nonNil(r.Industries),` with:

```go
		Industries:    industrytag.Canonicalize(r.Industries),
```

Add `"github.com/strelov1/freehire/internal/industrytag"` to the imports. Check the
module path against the top of `go.mod` before writing it.

`Canonicalize` already returns a non-nil slice, so `nonNil` is no longer needed
here; leave the helper alone if other call sites use it.

- [ ] **Step 5: Run the tests**

Run: `go test -tags=integration ./internal/db/ -run TestUpsertYCCompany -v && go test ./cmd/import-yc/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/db/ cmd/import-yc/
go vet ./... && go test ./... && go vet -tags=integration ./...
git add internal/db/ cmd/import-yc/
git commit -m "Make UpsertYCCompany merge instead of replace, canonicalize industries"
```

---

### Task 3: The import worker

**Files:**
- Create: `cmd/import-company-industries/main.go`
- Create: `cmd/import-company-industries/main_test.go`
- Modify: `internal/db/queries/companies.sql` (add two queries)

**Interfaces:**
- Consumes: `industrytag.Canonicalize` from Task 1.
- Produces: a binary. `ListCompanyIndustriesPage(ctx, afterSlug, limit)` returns `[]struct{Slug string; Industries []string}`; `SetCompanyIndustries(ctx, slug, industries)`.

The worker does two passes. The normalization pass rewrites every existing row
through the dictionary. The merge pass reads a JSONL of `{slug, name, markets}` and
unions the canonicalized markets in. Both are idempotent.

- [ ] **Step 1: Add the queries**

In `internal/db/queries/companies.sql`:

```sql
-- name: ListCompanyIndustriesPage :many
-- Keyset page over EVERY company, ordered by slug so a run resumes from the last
-- slug it saw. Deliberately unfiltered: the normalization pass only cares about
-- rows that already have industries, but the merge pass must also reach companies
-- with none, and one query serving both keeps the two walks identical.
SELECT slug, industries
FROM companies
WHERE slug > sqlc.arg(after_slug)
ORDER BY slug
LIMIT sqlc.arg(page_limit);

-- name: SetCompanyIndustries :exec
-- Replace one company's industries. The guard keeps updated_at honest: a row whose
-- value is already correct is not rewritten, so a re-run reports zero churn.
UPDATE companies
SET industries = sqlc.arg(industries), updated_at = now()
WHERE slug = sqlc.arg(slug) AND industries IS DISTINCT FROM sqlc.arg(industries);
```

Run: `make sqlc`

- [ ] **Step 2: Write the failing test for the merge input parser**

`cmd/import-company-industries/main_test.go`:

```go
package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSource(t *testing.T) {
	in := strings.Join([]string{
		`{"slug":"circle-com","name":"Circle","markets":["Fintech","Blockchain"]}`,
		`{"slug":"acme","name":"Acme Inc","markets":["Nonsense-Tag-Nobody-Knows"]}`,
		``,
		`{"slug":"dup","name":"Dup","markets":["Retail"]}`,
	}, "\n")

	got, err := parseSource(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}

	// Both keys are emitted: their slug is domain-derived (circle-com) while ours
	// comes from the name (circle), and neither alone matches enough of our rows.
	if want := []string{"crypto", "financial-services"}; !reflect.DeepEqual(got["circle-com"], want) {
		t.Errorf("circle-com = %q, want %q", got["circle-com"], want)
	}
	if !reflect.DeepEqual(got["circle"], []string{"crypto", "financial-services"}) {
		t.Errorf("name-derived key missing: %q", got["circle"])
	}
	if _, ok := got["acme"]; ok {
		t.Error("a company whose every tag is unknown should not be emitted at all")
	}
	if !reflect.DeepEqual(got["dup"], []string{"retail"}) {
		t.Errorf("dup = %q", got["dup"])
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./cmd/import-company-industries/ -run TestParseSource -v`
Expected: FAIL — `undefined: parseSource`.

- [ ] **Step 4: Write the worker**

`cmd/import-company-industries/main.go`:

```go
// Command import-company-industries rewrites companies.industries through the
// internal/industrytag dictionary, and optionally merges a company dump into it.
//
//	import-company-industries                 # normalize the existing column only
//	import-company-industries companies.jsonl # normalize, then merge the dump
//
// Needs DATABASE_URL. Run-once and exits non-zero on failure, like every other
// cmd/ worker. Both passes are idempotent: re-running rewrites nothing.
//
// The dump is JSONL of {"slug","name","markets"}. Dump slugs are domain-derived
// (circle.com -> "circle-com") while ours come from normalize.Slug(name), so each
// record is indexed under both keys; whichever matches a company wins.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"slices"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/industrytag"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/worker"
)

const pageSize = 1000

type record struct {
	Slug    string   `json:"slug"`
	Name    string   `json:"name"`
	Markets []string `json:"markets"`
}

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)

	changed, err := normalizeExisting(ctx, q)
	if err != nil {
		log.Printf("import-company-industries: normalize: %v", err)
		return 1
	}
	log.Printf("import-company-industries: normalized, rewrote %d rows", changed)

	if len(os.Args) < 2 {
		return 0
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Printf("import-company-industries: open: %v", err)
		return 1
	}
	defer f.Close()

	byKey, err := parseSource(f)
	if err != nil {
		log.Printf("import-company-industries: parse: %v", err)
		return 1
	}

	merged, err := mergeSource(ctx, q, byKey)
	if err != nil {
		log.Printf("import-company-industries: merge: %v", err)
		return 1
	}
	log.Printf("import-company-industries done: merged %d companies from %d keys",
		merged, len(byKey))
	return 0
}

// parseSource indexes a dump under both its own slug and a slug rebuilt from the
// company name. A record whose every tag is unknown is dropped rather than stored
// empty, so it never triggers a pointless UPDATE.
func parseSource(r io.Reader) (map[string][]string, error) {
	out := map[string][]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, err
		}
		inds := industrytag.Canonicalize(rec.Markets)
		if len(inds) == 0 {
			continue
		}
		for _, k := range []string{rec.Slug, normalize.Slug(rec.Name)} {
			if k != "" {
				out[k] = inds
			}
		}
	}
	return out, sc.Err()
}

// normalizeExisting walks every company with industries and rewrites the column
// through the dictionary. Values outside it are dropped: the column becomes
// dict-only. Unknown values are tallied so the dictionary has a growth path —
// without the report, dict-only is silent data loss.
func normalizeExisting(ctx context.Context, q *db.Queries) (int, error) {
	dropped := map[string]int{}
	after, changed := "", 0
	for {
		rows, err := q.ListCompanyIndustriesPage(ctx, db.ListCompanyIndustriesPageParams{
			AfterSlug: after, PageLimit: pageSize,
		})
		if err != nil {
			return changed, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			after = row.Slug
			if len(row.Industries) == 0 {
				continue
			}
			want := industrytag.Canonicalize(row.Industries)
			if slices.Equal(want, row.Industries) {
				continue
			}
			for _, had := range row.Industries {
				if !slices.Contains(want, had) && len(industrytag.Canonicalize([]string{had})) == 0 {
					dropped[had]++
				}
			}
			if err := q.SetCompanyIndustries(ctx, db.SetCompanyIndustriesParams{
				Slug: row.Slug, Industries: want,
			}); err != nil {
				return changed, err
			}
			changed++
		}
	}
	reportDropped(dropped)
	return changed, nil
}

// mergeSource unions the dump's canonical industries into each matching company.
func mergeSource(ctx context.Context, q *db.Queries, byKey map[string][]string) (int, error) {
	after, merged := "", 0
	for {
		rows, err := q.ListCompanyIndustriesPage(ctx, db.ListCompanyIndustriesPageParams{
			AfterSlug: after, PageLimit: pageSize,
		})
		if err != nil {
			return merged, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			after = row.Slug
			extra, ok := byKey[row.Slug]
			if !ok {
				continue
			}
			want := industrytag.Canonicalize(append(slices.Clone(row.Industries), extra...))
			if slices.Equal(want, row.Industries) {
				continue
			}
			if err := q.SetCompanyIndustries(ctx, db.SetCompanyIndustriesParams{
				Slug: row.Slug, Industries: want,
			}); err != nil {
				return merged, err
			}
			merged++
		}
	}
	return merged, nil
}

// reportDropped prints what the dictionary failed to recognize, most frequent
// first, so the next dictionary edit is driven by data rather than guesswork.
func reportDropped(dropped map[string]int) {
	if len(dropped) == 0 {
		return
	}
	type kv struct {
		tag string
		n   int
	}
	all := make([]kv, 0, len(dropped))
	total := 0
	for tag, n := range dropped {
		all = append(all, kv{tag, n})
		total += n
	}
	slices.SortFunc(all, func(a, b kv) int { return b.n - a.n })
	log.Printf("import-company-industries: dropped %d distinct unknown values (%d occurrences)",
		len(all), total)
	for _, e := range all[:min(20, len(all))] {
		log.Printf("  unknown: %-40s %d", e.tag, e.n)
	}
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./cmd/import-company-industries/ -v`
Expected: PASS.

- [ ] **Step 6: Dry-run against a scratch copy of production data**

Take a backup of the column first — this pass is destructive:

```bash
ssh root@89.167.94.146 "sudo -u postgres psql -d hire -c \
  \"CREATE TABLE industries_backup_20260815 AS SELECT slug, industries FROM companies WHERE cardinality(industries) > 0;\""
```

Then run the worker against production and read the dropped-value report before
believing the result.

- [ ] **Step 7: Commit**

```bash
gofmt -w cmd/import-company-industries/ internal/db/
go vet ./... && go test ./...
git add cmd/import-company-industries/ internal/db/
git commit -m "Add import-company-industries worker: normalize and merge industries"
```

---

### Task 4: Wire the facet into Meilisearch and the API

**Files:**
- Modify: `internal/search/company.go:113-140`
- Modify: `internal/handler/companies.go`
- Test: `internal/search/company_test.go`

**Interfaces:**
- Consumes: canonical values written by Tasks 2 and 3.
- Produces: an `industries` query parameter on `/companies`, filtering by membership.

- [ ] **Step 1: Write the failing test**

In `internal/search/company_test.go`:

```go
func TestCompanySettingsExposesIndustriesFacet(t *testing.T) {
	s := companySettings()
	if !slices.Contains(s.FilterableAttributes, "industries") {
		t.Error("industries missing from FilterableAttributes: the facet cannot be filtered")
	}
}

func TestCompanyFacetsIncludesIndustries(t *testing.T) {
	var found bool
	for _, f := range companyFacets {
		if f.param == "industries" && f.attr == "industries" {
			found = true
		}
	}
	if !found {
		t.Error("industries missing from companyFacets: the query param is not mapped")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/search/ -run Industries -v`
Expected: FAIL on both.

- [ ] **Step 3: Add the attribute and the param**

In `companySettings()`, add `"industries"` to `FilterableAttributes`.
In `companyFacets`, add `{"industries", "industries"}` after the `domains` entry so
the built filter stays in a fixed order.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/search/ -v`
Expected: PASS.

- [ ] **Step 5: Commit and reindex**

```bash
gofmt -w internal/search/ internal/handler/
go vet ./... && go test ./...
git add internal/search/ internal/handler/
git commit -m "Expose industries as a company facet"
```

A new filterable attribute needs a rebuilt index before it will answer — until then
filtering on it 500s. After deploy, and **only when no other reindex is running**
(check `pgrep -af reindex` on the host; Meilisearch serves one task queue, so a
second rebuild queues behind the first and looks like a hang):

```bash
ssh root@89.167.94.146 'systemctl stop freehire-reindexw.timer'
ssh root@89.167.94.146 'cd /opt/freehire && ./bin/reindex-companies'
ssh root@89.167.94.146 'systemctl start freehire-reindexw.timer'
```

---

### Task 5: The UI filter

**Files:**
- Modify: `cmd/gen-contracts/main.go:342`
- Modify: `web/src/lib/facets.ts` (next to `const DOMAINS` at :413 and the facet list at :536-560)
- Test: `web/src/lib/facets.test.ts`

**Interfaces:**
- Consumes: the `industries` param from Task 4; `industrytag.Canonicals()` and `Label()` from Task 1.
- Produces: nothing downstream.

- [ ] **Step 0: Generate the options from Go, do not retype them**

`DOMAINS` in `facets.ts:413` is `options(DOMAIN_VALUES, DOMAIN_LABELS)`, and those
come from `web/src/lib/generated/contracts.ts`, emitted by `cmd/gen-contracts`.
Industries follow the same path — a hand-typed copy in TypeScript would drift from
the Go dictionary the first time someone adds a line to it.

In `cmd/gen-contracts/main.go`, beside the existing `emitVocab("Domain", ...)` call:

```go
	b.WriteString(emitVocab("Industry", "INDUSTRY_VALUES", industrytag.Canonicals()))
```

Read `emitVocab`'s signature and the surrounding lines before writing this: if it
also emits a labels map, pass `industrytag.Labels()`; if labels are a separate
emitter, mirror whatever `DOMAIN_LABELS` uses. Add the `industrytag` import.

Run: `make gen-contracts`
Expected: `INDUSTRY_VALUES` appears in `web/src/lib/generated/contracts.ts`.

- [ ] **Step 1: Write the failing test**

In `web/src/lib/facets.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { COMPANY_FACETS } from './facets';

describe('company facets', () => {
  it('offers an industries filter distinct from the coarse domain one', () => {
    const industries = COMPANY_FACETS.find((f) => f.param === 'industries');
    expect(industries).toBeDefined();
    expect(industries?.label).toBe('Industry (detailed)');
  });
});
```

Check the real exported name of the facet array in `facets.ts` before writing the
import — the file exports several, and the test must reference the company one.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && pnpm vitest run src/lib/facets.test.ts`
Expected: FAIL — `industries` is undefined.

- [ ] **Step 3: Add the facet entry**

Next to the existing `subindustries` entry:

```ts
  {
    param: 'industries',
    label: 'Industry (detailed)',
    control: 'select',
    options: INDUSTRIES,
    excludable: true,
    placeholder: 'Search industries',
  },
```

Define it next to `DOMAINS` at `facets.ts:413`, the same shape:

```ts
const INDUSTRIES: FacetOption[] = options(INDUSTRY_VALUES, INDUSTRY_LABELS);
```

importing `INDUSTRY_VALUES` / `INDUSTRY_LABELS` from `./generated/contracts`
alongside the existing `DOMAIN_VALUES` import, and reference `INDUSTRIES` (not
`INDUSTRY_OPTIONS`) in the facet entry above.

- [ ] **Step 4: Run the test**

Run: `cd web && pnpm vitest run src/lib/facets.test.ts`
Expected: PASS.

- [ ] **Step 5: Verify in a browser**

Run the app, open the companies page, filter by one industry, and confirm the URL
carries `industries=<slug>` and the result count changes. A facet that renders but
filters nothing is the failure this step exists to catch.

- [ ] **Step 6: Commit**

```bash
cd web && pnpm lint
git add web/src/lib/facets.ts web/src/lib/facets.test.ts
git commit -m "Add detailed industry filter to the companies page"
```

---

## Notes for whoever runs this

- Tasks 1–3 stand alone. Merging them makes the column clean even if the facet
  never ships.
- Task 3's normalization pass is destructive by design. The backup in Step 6 is not
  optional.
- The dropped-value report from Task 3 is the input to the next dictionary edit.
  Expect the first production run to name a few hundred unknown values; adding the
  frequent ones and re-running is the intended loop, not a sign of failure.
