# Skill-Match Sort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Historical.** This plan was written before implementation and is kept as the
> record of what was intended. Three things went differently and the CODE is the
> authority, not this file: task 1 needed no work (`skilltag.Canonicals()` already
> existed, drawing on five alias tiers rather than two); the SPA had no sort control at
> all to extend, so one was restored; and the weights are anchored on the commonest
> skill rather than on a catalogue total, because the sum of per-skill counts inflates
> with catalogue breadth and flattens the rarity contrast. `openspec/changes/skill-match-sort/`
> carries the current specification.
>
> The generator also ended up stricter than sketched here: it takes its path as an
> argument, treats only a MISSING registry as a first run, and refuses a present-but-
> empty one — because any other read error silently rewriting positions is exactly the
> corruption the append-only rule exists to prevent. Read `internal/dict/skillvec/gen`,
> not the sketch below.

**Goal:** Order the jobs feed by how well a vacancy's skills match the signed-in candidate's, using an IDF-weighted skill vector ranked by Meilisearch's own vector search.

**Architecture:** Skills are canonical slugs from a finite dictionary, so the vector is arithmetic, not learned — each skill owns a permanent position, weighted by how rare it is in the catalogue. Vacancy vectors are built at index time and stored in the Meilisearch document; the candidate's vector is built per request from their profile. Meilisearch applies the vector ranking and every existing facet filter in one query, so pagination stays ordinary.

**Tech Stack:** Go, Meilisearch v1.49 (`userProvided` embedder), meilisearch-go v0.36.3, PostgreSQL/sqlc, SvelteKit.

## Global Constraints

- **English only** in code, comments, identifiers, docs, and commits.
- **Vector positions are permanent.** A skill's position is assigned once and never
  reused or shifted. Shifting one silently invalidates all 1.36M stored vectors.
- **Declared dimensions: 1024.** The dictionary holds 749 canonical skills; the rest
  is headroom. Storage scales with the declared width, so this costs ~+10.2 GB on the
  live index versus ~+7.5 GB at 749.
- **Dictionaries are dict-only.** Never guess; an unknown slug contributes nothing.
- **A new package must be added to `internal/platform/arch/layering/blocks.go`** or the
  layering guard fails.
- **Before committing any `*.go`:** `gofmt -w` those paths, then `go vet ./...` and
  `go test ./...`. Run `go vet -tags=integration ./...` before pushing.
- **This cannot deploy until host2 disk is resolved** — introducing an embedder requires
  a full rebuild, and `cmd/reindex` currently refuses to run (62 GiB free against a
  70 GiB floor). Ship the code; coordinate the rebuild separately.

---

## File Structure

| file | responsibility |
|---|---|
| `internal/dict/skilltag/canonicals.go` (create) | expose the dictionary's canonical slugs |
| `internal/dict/skillvec/registry.go` (create) | the permanent skill→position registry |
| `internal/dict/skillvec/skillvec.go` (create) | `Weights`, vector construction |
| `internal/dict/skillvec/AGENTS.md` (create) | why positions are permanent |
| `internal/platform/arch/layering/blocks.go` (modify) | register `skillvec` in the `dict` block |
| `internal/search/search/skillweights.go` (create) | load IDF weights from `insights_facet_stats` |
| `internal/search/search/document.go` (modify) | `JobDocument.SkillVector`, `FromJob` signature |
| `internal/search/search/client.go` (modify) | declare the embedder; accept a query vector |
| `cmd/reindex/main.go`, `cmd/search-drain/indexer.go`, `internal/ingest/linkimport/linkimport.go` (modify) | pass weights to `FromJob` |
| `internal/api/handler/search.go` (modify) | the `sort=match` path |
| `web/src/lib/facetModel.ts` + sort UI (modify) | expose the option to signed-in users |

---

### Task 1: Expose the dictionary's canonical skills

`skillvec` needs the full list of canonical slugs to build its registry and to
assert the registry stays complete. `skilltag` has no public accessor today —
the canonical values are only reachable as the *values* of two unexported alias
tables (`wordAliases`, `phraseAliases`).

**Files:**
- Create: `internal/dict/skilltag/canonicals.go`
- Test: `internal/dict/skilltag/canonicals_test.go`

**Interfaces:**
- Produces: `skilltag.Canonicals() []string` — every distinct canonical slug, sorted, a fresh slice per call.

- [ ] **Step 1: Write the failing test**

```go
package skilltag

import (
	"slices"
	"testing"
)

func TestCanonicalsIsSortedAndDeduped(t *testing.T) {
	got := Canonicals()
	if !slices.IsSorted(got) {
		t.Errorf("Canonicals() is not sorted")
	}
	for i := 1; i < len(got); i++ {
		if got[i] == got[i-1] {
			t.Fatalf("Canonicals() contains a duplicate: %q", got[i])
		}
	}
	// Sanity: the dictionary is substantial, and every entry is non-empty.
	if len(got) < 700 {
		t.Errorf("Canonicals() = %d entries, want at least 700", len(got))
	}
	for _, s := range got {
		if s == "" {
			t.Fatalf("Canonicals() contains an empty slug")
		}
	}
}

func TestCanonicalsCoversBothAliasTables(t *testing.T) {
	got := Canonicals()
	for _, want := range wordAliases {
		if !slices.Contains(got, want) {
			t.Errorf("Canonicals() is missing word-alias canonical %q", want)
		}
	}
	for _, p := range phraseAliases {
		if !slices.Contains(got, p.canonical) {
			t.Errorf("Canonicals() is missing phrase-alias canonical %q", p.canonical)
		}
	}
}

func TestCanonicalsReturnsAFreshSlice(t *testing.T) {
	a := Canonicals()
	if len(a) == 0 {
		t.Fatal("Canonicals() is empty")
	}
	a[0] = "mutated"
	if Canonicals()[0] == "mutated" {
		t.Error("Canonicals() shares backing storage between calls")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dict/skilltag/ -run TestCanonicals -v`
Expected: FAIL — `undefined: Canonicals`

- [ ] **Step 3: Write minimal implementation**

```go
package skilltag

import (
	"maps"
	"slices"
)

// canonicals is every distinct canonical slug the dictionary resolves to, computed
// once from both alias tables. It is the vocabulary internal/dict/skillvec assigns
// permanent vector positions to, so it must name every value Canonicalize can emit —
// a canonical reachable through one table but absent here would be a skill no vector
// can ever carry.
var canonicals = func() []string {
	set := make(map[string]struct{}, len(wordAliases)+len(phraseAliases))
	for _, c := range wordAliases {
		set[c] = struct{}{}
	}
	for _, p := range phraseAliases {
		set[p.canonical] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}()

// Canonicals returns every canonical skill slug in the dictionary, sorted. The
// returned slice is a copy: callers (notably the skillvec registry generator) may
// sort or filter it without corrupting the dictionary.
func Canonicals() []string { return slices.Clone(canonicals) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/dict/skilltag/ -run TestCanonicals -v`
Expected: PASS, all three tests.

- [ ] **Step 5: Format, vet, full package test, commit**

```bash
gofmt -w internal/dict/skilltag/canonicals.go internal/dict/skilltag/canonicals_test.go
go vet ./internal/dict/skilltag/
go test ./internal/dict/skilltag/
git add internal/dict/skilltag/canonicals.go internal/dict/skilltag/canonicals_test.go
git commit -m "feat(skilltag): expose the dictionary's canonical slugs"
```

---

### Task 2: The permanent skill→position registry

**Files:**
- Create: `internal/dict/skillvec/registry.go`
- Create: `internal/dict/skillvec/gen/main.go`
- Test: `internal/dict/skillvec/registry_test.go`
- Modify: `internal/platform/arch/layering/blocks.go` (the `dict` block list)

**Interfaces:**
- Consumes: `skilltag.Canonicals() []string` (Task 1).
- Produces:
  - `skillvec.Dimensions` — `const Dimensions = 1024`
  - `skillvec.Position(skill string) (int, bool)` — the skill's permanent position
  - `skillvec.RegistrySize() int` — how many positions are assigned

The registry is a generated, committed list. Order in the file **is** the position:
index 0 is position 0. A generator appends newly-added dictionary skills to the end
and never reorders — that is what makes positions permanent.

- [ ] **Step 1: Write the failing test**

```go
package skillvec

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

func TestRegistryHasNoDuplicates(t *testing.T) {
	seen := make(map[string]int, len(registry))
	for i, s := range registry {
		if prev, dup := seen[s]; dup {
			t.Fatalf("registry[%d] = %q duplicates registry[%d]", i, s, prev)
		}
		seen[s] = i
	}
}

func TestRegistryCoversEveryCanonicalSkill(t *testing.T) {
	in := make(map[string]bool, len(registry))
	for _, s := range registry {
		in[s] = true
	}
	for _, s := range skilltag.Canonicals() {
		if !in[s] {
			t.Errorf("canonical skill %q has no vector position — run go generate ./internal/dict/skillvec/", s)
		}
	}
}

func TestRegistryFitsTheDeclaredDimensions(t *testing.T) {
	if len(registry) > Dimensions {
		t.Fatalf("registry holds %d skills, past the declared %d dimensions; widening Dimensions requires a full reindex",
			len(registry), Dimensions)
	}
}

func TestPosition(t *testing.T) {
	got, ok := Position(registry[0])
	if !ok || got != 0 {
		t.Errorf("Position(%q) = %d, %v; want 0, true", registry[0], got, ok)
	}
	last := len(registry) - 1
	if got, ok := Position(registry[last]); !ok || got != last {
		t.Errorf("Position(%q) = %d, %v; want %d, true", registry[last], got, ok, last)
	}
	if _, ok := Position("definitely-not-a-skill"); ok {
		t.Error("Position() resolved an unknown slug; unknowns must report false")
	}
}

func TestRegistrySize(t *testing.T) {
	if RegistrySize() != len(registry) {
		t.Errorf("RegistrySize() = %d, want %d", RegistrySize(), len(registry))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dict/skillvec/ -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the generator**

```go
// Command gen regenerates internal/dict/skillvec/registry.go.
//
// It APPENDS canonical skills that have no position yet and never reorders the ones
// that do: a position is permanent, and shifting one silently invalidates every
// vector already stored in the search index. Run it after the dictionary grows (see
// the mine-skill-dictionary skill), then commit the regenerated file.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"slices"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

const target = "internal/dict/skillvec/registry.go"

var entry = regexp.MustCompile(`\t"([^"]+)",`)

func main() {
	existing := readExisting()
	known := make(map[string]bool, len(existing))
	for _, s := range existing {
		known[s] = true
	}
	added := 0
	for _, s := range skilltag.Canonicals() {
		if !known[s] {
			existing = append(existing, s)
			known[s] = true
			added++
		}
	}

	var buf bytes.Buffer
	fmt.Fprint(&buf, header)
	for _, s := range existing {
		fmt.Fprintf(&buf, "\t%q,\n", s)
	}
	fmt.Fprint(&buf, "}\n")

	src, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen: format:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(target, src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen: write:", err)
		os.Exit(1)
	}
	fmt.Printf("gen: %d positions (%d newly appended)\n", len(existing), added)
}

// readExisting parses the positions already assigned, in order. A missing file is
// the first run and yields none.
func readExisting() []string {
	b, err := os.ReadFile(target)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range entry.FindAllSubmatch(b, -1) {
		out = append(out, string(m[1]))
	}
	return slices.Clip(out)
}

const header = `// Code generated by internal/dict/skillvec/gen. DO NOT EDIT BY HAND.
//
// A skill's index in this slice IS its permanent vector position. Entries are only
// ever APPENDED: reordering or removing one invalidates every vector stored in the
// search index, silently — the feed would simply start ranking wrongly. Regenerate
// with ` + "`go generate ./internal/dict/skillvec/`" + ` after the dictionary grows.

package skillvec

var registry = []string{
`
```

- [ ] **Step 4: Write the package's hand-written half**

Create `internal/dict/skillvec/skillvec.go` with the registry accessors (vector
construction lands in Task 3):

```go
// Package skillvec turns a set of canonical skill slugs into a fixed-width vector,
// so a vacancy's skills and a candidate's can be compared by cosine in the search
// engine rather than by a set operation in application code.
//
// A skill's position is PERMANENT (see registry.go). The weights that fill those
// positions are not: they express how rare a skill is in the live catalogue and are
// recomputed as it changes. Shifting a position corrupts every stored vector;
// shifting a weight only nudges the ranking.
package skillvec

//go:generate go run ./gen

// Dimensions is the declared width of a skill vector — deliberately wider than the
// dictionary so new skills get positions without a re-declaration. Meilisearch stores
// the full declared width whether or not the tail is occupied, so this is not free:
// at the live catalogue's scale each 256 dimensions costs roughly 2.5 GB of index.
// Widening it later requires a full reindex.
const Dimensions = 1024

// positions indexes the registry for lookup.
var positions = func() map[string]int {
	m := make(map[string]int, len(registry))
	for i, s := range registry {
		m[s] = i
	}
	return m
}()

// Position reports the permanent vector position of a canonical skill slug, and
// whether the slug has one. An unknown slug has none — dictionaries are dict-only,
// so an unrecognised skill contributes nothing rather than being guessed at.
func Position(skill string) (int, bool) {
	i, ok := positions[skill]
	return i, ok
}

// RegistrySize is how many positions are assigned. Dimensions minus this is the
// headroom left for dictionary growth before a reindex-forcing re-declaration.
func RegistrySize() int { return len(registry) }
```

- [ ] **Step 5: Generate the registry**

```bash
go generate ./internal/dict/skillvec/
```

Expected output: `gen: 749 positions (749 newly appended)`.
Verify `internal/dict/skillvec/registry.go` now holds 749 quoted slugs.

- [ ] **Step 6: Register the package in the layering table**

In `internal/platform/arch/layering/blocks.go`, add `"skillvec"` to the `"dict"`
block's list, keeping the list alphabetical:

```go
	"dict": {
		"classify", "companyname", "industrytag", "lang", "location", "normalize",
		"roletag", "roletype", "skilladjacency", "skillbundle", "skilltag",
		"skillvec", "vocab", "wordmatch",
	},
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/dict/skillvec/ ./internal/platform/arch/layering/ -v`
Expected: PASS. The layering test must not report `skillvec` as unplaced.

- [ ] **Step 8: Write the package AGENTS.md**

Create `internal/dict/skillvec/AGENTS.md`:

```markdown
# Skill vectors

## Scope
Turns a set of canonical `skilltag` slugs into a fixed-width vector, so the search
engine can rank vacancies by cosine against a candidate's skills instead of the
application scoring a window by hand.

## The one rule everything turns on

> **A position is permanent. A weight is not.**

`registry.go` is generated and APPEND-ONLY: a skill's index in it IS its vector
position. Reordering or removing one invalidates every vector already stored in the
search index — silently, with no error, the feed simply starts ranking wrongly.
Weights, by contrast, express catalogue rarity and are expected to drift; a stale
weight nudges the order, it does not corrupt it.

Regenerate after the dictionary grows (see the `mine-skill-dictionary` skill):
`go generate ./internal/dict/skillvec/`, then commit the result. The generator only
appends; it cannot reorder.

## Dimensions
`Dimensions` (1024) is wider than the dictionary (749) so growth needs no
re-declaration. It is NOT free — Meilisearch stores the declared width whether or not
the tail is occupied, and at catalogue scale each 256 dimensions costs roughly 2.5 GB
of index. Widening it forces a full reindex.

## Weights
`Weights` holds one factor per canonical skill, derived from how many open jobs name
it (`insights_facet_stats`, populated by `cmd/rollup-facets`). Rare skills weigh more:
an overlap on `git` says nothing, an overlap on `erlang` says a great deal. The zero
`Weights` yields nil vectors, which is the correct degradation — no rollup means no
match sort, and everything else keeps working.
```

- [ ] **Step 9: Format, vet, commit**

```bash
gofmt -w internal/dict/skillvec/ internal/platform/arch/layering/blocks.go
go vet ./...
go test ./internal/dict/skillvec/ ./internal/platform/arch/layering/
git add internal/dict/skillvec/ internal/platform/arch/layering/blocks.go
git commit -m "feat(skillvec): permanent skill-to-position registry"
```

---

### Task 3: IDF weights and vector construction

**Files:**
- Modify: `internal/dict/skillvec/skillvec.go`
- Test: `internal/dict/skillvec/skillvec_test.go`

**Interfaces:**
- Consumes: `skillvec.Position` and `Dimensions` (Task 2).
- Produces:
  - `type Weights struct { … }` — zero value is usable and yields nil vectors
  - `func WeightsFromCounts(counts map[string]int64, openJobs int64) Weights`
  - `func (w Weights) Vector(skills []string) []float32` — L2-normalised, nil when unusable

Normalising to unit length inside `Vector` is what makes the cosine the intended
"overlap AND coverage" score: the denominator `‖A‖·‖B‖` penalises both a one-tag
vacancy and a thirty-tag requirements dump.

- [ ] **Step 1: Write the failing test**

```go
package skillvec

import (
	"math"
	"testing"
)

// counts models a catalogue where `git` is everywhere and `erlang` is rare.
func testWeights() Weights {
	return WeightsFromCounts(map[string]int64{
		registry[0]: 90_000,
		registry[1]: 10_000,
		registry[2]: 1_000,
		registry[3]: 10,
	}, 100_000)
}

func TestVectorIsUnitLength(t *testing.T) {
	v := testWeights().Vector([]string{registry[0], registry[2]})
	if len(v) != Dimensions {
		t.Fatalf("Vector() width = %d, want %d", len(v), Dimensions)
	}
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if math.Abs(math.Sqrt(sum)-1) > 1e-5 {
		t.Errorf("Vector() length = %f, want 1", math.Sqrt(sum))
	}
}

func TestVectorPlacesWeightsAtRegistryPositions(t *testing.T) {
	v := testWeights().Vector([]string{registry[2]})
	if v[2] == 0 {
		t.Error("Vector() left position 2 empty for a skill that occupies it")
	}
	for i, x := range v {
		if i != 2 && x != 0 {
			t.Fatalf("Vector() wrote %f at position %d; only position 2 should be set", x, i)
		}
	}
}

func TestRarerSkillWeighsMore(t *testing.T) {
	w := testWeights()
	common := w.Vector([]string{registry[0], registry[1]})
	rare := w.Vector([]string{registry[0], registry[3]})
	// Against a query naming only the rare skill, the rare-bearing vector must win.
	q := w.Vector([]string{registry[3]})
	if dot(rare, q) <= dot(common, q) {
		t.Errorf("rare-skill vector scored %f, not more than the common-skill vector's %f",
			dot(rare, q), dot(common, q))
	}
}

// TestCosineOrdersOverlapAndCoverage is the worked example from the design doc:
// a well-targeted vacancy must outrank both a one-tag vacancy and a requirements dump.
func TestCosineOrdersOverlapAndCoverage(t *testing.T) {
	counts := map[string]int64{}
	for _, s := range registry[:12] {
		counts[s] = 5_000
	}
	w := WeightsFromCounts(counts, 100_000)

	profile := w.Vector(registry[:5])
	oneTag := w.Vector(registry[:1])
	targeted := w.Vector(registry[:5])
	dump := w.Vector(registry[:12])

	if dot(targeted, profile) <= dot(oneTag, profile) {
		t.Errorf("one-tag vacancy (%f) outranked the targeted one (%f)",
			dot(oneTag, profile), dot(targeted, profile))
	}
	if dot(targeted, profile) <= dot(dump, profile) {
		t.Errorf("requirements dump (%f) outranked the targeted vacancy (%f)",
			dot(dump, profile), dot(targeted, profile))
	}
}

func TestUnusableInputsYieldNil(t *testing.T) {
	w := testWeights()
	if got := w.Vector(nil); got != nil {
		t.Errorf("Vector(nil) = %v, want nil", got)
	}
	if got := w.Vector([]string{"definitely-not-a-skill"}); got != nil {
		t.Errorf("Vector() of only-unknown skills = %v, want nil", got)
	}
	if got := (Weights{}).Vector([]string{registry[0]}); got != nil {
		t.Errorf("zero Weights produced %v, want nil", got)
	}
}

func TestUnknownSkillsAreIgnoredNotGuessed(t *testing.T) {
	w := testWeights()
	with := w.Vector([]string{registry[0], "definitely-not-a-skill"})
	without := w.Vector([]string{registry[0]})
	for i := range with {
		if with[i] != without[i] {
			t.Fatalf("an unknown skill changed the vector at position %d", i)
		}
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dict/skillvec/ -run 'TestVector|TestRarer|TestCosine|TestUnusable|TestUnknown' -v`
Expected: FAIL — `undefined: Weights`, `undefined: WeightsFromCounts`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/dict/skillvec/skillvec.go`:

```go
import "math"

// Weights holds one factor per canonical skill: how much an overlap on it is worth.
// The zero value is usable and yields nil vectors — the correct degradation when the
// rarity rollup has not run, since a missing weight table must not fail indexing.
type Weights struct {
	// byPosition is indexed by vector position, so Vector needs no map lookup per
	// skill beyond resolving the position itself.
	byPosition []float32
}

// WeightsFromCounts derives rarity weights from how many open jobs name each skill.
// counts is keyed by canonical slug (the `skills` slice of insights_facet_stats);
// openJobs is the catalogue size those counts were taken over.
//
// The factor is the standard inverse-document-frequency shape, floored at 1 so a
// skill every posting names still contributes something rather than vanishing:
//
//	idf(s) = ln((openJobs + 1) / (count(s) + 1)) + 1
//
// A skill absent from counts is treated as unseen (count 0), which makes it maximally
// rare. That is deliberate: a skill in the dictionary but not in the rollup is either
// brand new or genuinely obscure, and both deserve weight.
func WeightsFromCounts(counts map[string]int64, openJobs int64) Weights {
	if openJobs <= 0 {
		return Weights{}
	}
	byPosition := make([]float32, Dimensions)
	for i, skill := range registry {
		idf := math.Log(float64(openJobs+1)/float64(counts[skill]+1)) + 1
		byPosition[i] = float32(idf)
	}
	return Weights{byPosition: byPosition}
}

// Vector builds the L2-normalised vector for a set of canonical skill slugs.
//
// Normalising is what makes the cosine of two such vectors read as "how many of my
// skills does it engage AND what share of its requirements do I cover": the length
// division penalises both a vacancy carrying one tag and a vacancy dumping thirty.
//
// Returns nil — never a zero vector — when the result would be meaningless: no
// weights loaded, no skills given, or no skill recognised. A nil vector is an absence
// the caller omits from the document, not a document that ranks against everything.
func (w Weights) Vector(skills []string) []float32 {
	if len(w.byPosition) == 0 || len(skills) == 0 {
		return nil
	}
	v := make([]float32, Dimensions)
	var sumSq float64
	for _, s := range skills {
		pos, ok := Position(s)
		if !ok {
			continue
		}
		if v[pos] != 0 {
			continue // the same skill listed twice must not weigh double
		}
		x := w.byPosition[pos]
		v[pos] = x
		sumSq += float64(x) * float64(x)
	}
	if sumSq == 0 {
		return nil
	}
	norm := float32(math.Sqrt(sumSq))
	for i := range v {
		if v[i] != 0 {
			v[i] /= norm
		}
	}
	return v
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/dict/skillvec/ -v`
Expected: PASS, every test including Task 2's.

- [ ] **Step 5: Format, vet, commit**

```bash
gofmt -w internal/dict/skillvec/
go vet ./internal/dict/skillvec/
go test ./internal/dict/skillvec/
git add internal/dict/skillvec/
git commit -m "feat(skillvec): IDF-weighted, unit-length skill vectors"
```

---

### Task 4: Load the weights from the facet-stats rollup

**Files:**
- Create: `internal/search/search/skillweights.go`
- Test: `internal/search/search/skillweights_test.go`

**Interfaces:**
- Consumes: `skillvec.WeightsFromCounts` (Task 3); `db.Queries.ListFacetStats` (existing).
- Produces: `func LoadSkillWeights(ctx context.Context, r FacetStatsReader) (skillvec.Weights, error)` and `type FacetStatsReader interface`.

`insights_facet_stats` already carries `facet='skills'` rows with per-value open-job
counts, populated by `cmd/rollup-facets`. Nothing new needs computing.

- [ ] **Step 1: Write the failing test**

```go
package search

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeFacetStats struct {
	rows []db.InsightsFacetStat
	err  error
}

func (f fakeFacetStats) ListFacetStats(context.Context) ([]db.InsightsFacetStat, error) {
	return f.rows, f.err
}

func TestLoadSkillWeightsReadsOnlyTheSkillsFacet(t *testing.T) {
	w, err := LoadSkillWeights(context.Background(), fakeFacetStats{rows: []db.InsightsFacetStat{
		{Facet: "skills", Value: "go", Count: 5000},
		{Facet: "skills", Value: "erlang", Count: 12},
		{Facet: "countries", Value: "DE", Count: 40000},
	}})
	if err != nil {
		t.Fatalf("LoadSkillWeights() error = %v", err)
	}
	rare := w.Vector([]string{"erlang"})
	common := w.Vector([]string{"go"})
	if rare == nil || common == nil {
		t.Fatal("LoadSkillWeights() produced weights that build no vectors")
	}
}

func TestLoadSkillWeightsWithNoSkillRowsDegradesToZero(t *testing.T) {
	w, err := LoadSkillWeights(context.Background(), fakeFacetStats{rows: []db.InsightsFacetStat{
		{Facet: "countries", Value: "DE", Count: 40000},
	}})
	if err != nil {
		t.Fatalf("LoadSkillWeights() error = %v", err)
	}
	if got := w.Vector([]string{"go"}); got != nil {
		t.Errorf("with no skill rows, Vector() = %v, want nil", got)
	}
}

func TestLoadSkillWeightsPropagatesTheError(t *testing.T) {
	sentinel := errors.New("boom")
	if _, err := LoadSkillWeights(context.Background(), fakeFacetStats{err: sentinel}); !errors.Is(err, sentinel) {
		t.Errorf("LoadSkillWeights() error = %v, want it to wrap %v", err, sentinel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/search/ -run TestLoadSkillWeights -v`
Expected: FAIL — `undefined: LoadSkillWeights`.

- [ ] **Step 3: Write minimal implementation**

```go
package search

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

// FacetStatsReader reads the facet-distribution snapshot. *db.Queries satisfies it;
// tests inject a fake.
type FacetStatsReader interface {
	ListFacetStats(ctx context.Context) ([]db.InsightsFacetStat, error)
}

// skillsFacet is the facet name cmd/rollup-facets writes the per-skill open-job
// counts under.
const skillsFacet = "skills"

// LoadSkillWeights derives skill-rarity weights from the facet-distribution snapshot
// (insights_facet_stats), which cmd/rollup-facets already maintains — the counts this
// needs are the same ones the public /open page renders, so nothing new is computed.
//
// The catalogue size is taken as the sum of the skill counts rather than an open-job
// total: a job naming three skills contributes to three counts, so this sum is the
// count of (job, skill) pairs. That is the right denominator for an
// inverse-document-frequency over skills, and it keeps the read to one query.
//
// A snapshot with no skill rows yields the zero Weights, which builds no vectors —
// the intended degradation before the first rollup, never an error.
func LoadSkillWeights(ctx context.Context, r FacetStatsReader) (skillvec.Weights, error) {
	rows, err := r.ListFacetStats(ctx)
	if err != nil {
		return skillvec.Weights{}, fmt.Errorf("search: list facet stats: %w", err)
	}
	counts := make(map[string]int64)
	var total int64
	for _, row := range rows {
		if row.Facet != skillsFacet {
			continue
		}
		counts[row.Value] = row.Count
		total += row.Count
	}
	if total == 0 {
		return skillvec.Weights{}, nil
	}
	return skillvec.WeightsFromCounts(counts, total), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/search/search/ -run TestLoadSkillWeights -v`
Expected: PASS.

- [ ] **Step 5: Format, vet, commit**

```bash
gofmt -w internal/search/search/skillweights.go internal/search/search/skillweights_test.go
go vet ./internal/search/search/
go test ./internal/search/search/
git add internal/search/search/skillweights.go internal/search/search/skillweights_test.go
git commit -m "feat(search): derive skill-rarity weights from the facet rollup"
```

---

### Task 5: Carry the vector on the search document

**Files:**
- Modify: `internal/search/search/document.go:36-77` (the struct), `:86-111` (`FromJob`), `:17-26` (the size comment)
- Test: `internal/search/search/document_test.go`

**Interfaces:**
- Consumes: `skillvec.Weights` (Task 3).
- Produces: `FromJob(j db.Job, w skillvec.Weights) (JobDocument, error)` — **the signature changes**, and `JobDocument.Vectors map[string][]float32` with JSON tag `_vectors`.

The signature change is deliberate. `doc.Reality` is attached by the caller because it
needs a clock and cluster counts; weights are just a value, and threading them through
the signature makes the compiler catch any indexer that forgets — a document silently
missing its vector would simply drop out of the match feed with no error anywhere.

- [ ] **Step 1: Write the failing test**

```go
func TestFromJobCarriesTheSkillVector(t *testing.T) {
	w := skillvec.WeightsFromCounts(map[string]int64{"go": 5000, "erlang": 12}, 100000)
	doc, err := FromJob(db.Job{ID: 1, Title: "Engineer", Skills: []string{"go", "erlang"}}, w)
	if err != nil {
		t.Fatalf("FromJob() error = %v", err)
	}
	v, ok := doc.Vectors[SkillEmbedder]
	if !ok {
		t.Fatalf("FromJob() set no %q vector; document keys = %v", SkillEmbedder, doc.Vectors)
	}
	if len(v) != skillvec.Dimensions {
		t.Errorf("vector width = %d, want %d", len(v), skillvec.Dimensions)
	}
}

func TestFromJobWithoutWeightsOmitsTheVectorEntirely(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, Title: "Engineer", Skills: []string{"go"}}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob() error = %v", err)
	}
	if doc.Vectors != nil {
		t.Errorf("FromJob() with zero weights set Vectors = %v, want nil", doc.Vectors)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte(`"_vectors"`)) {
		t.Error("a vector-less document still serialised a _vectors key")
	}
}

func TestFromJobWithNoRecognisedSkillsOmitsTheVector(t *testing.T) {
	w := skillvec.WeightsFromCounts(map[string]int64{"go": 5000}, 100000)
	doc, err := FromJob(db.Job{ID: 1, Title: "Engineer", Skills: []string{"not-a-skill"}}, w)
	if err != nil {
		t.Fatalf("FromJob() error = %v", err)
	}
	if doc.Vectors != nil {
		t.Errorf("Vectors = %v, want nil for a job with no recognised skills", doc.Vectors)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/search/ -run TestFromJob -v`
Expected: FAIL — `too many arguments in call to FromJob`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/search/search/document.go`:

```go
// SkillEmbedder is the name of the Meilisearch embedder carrying skill vectors. It
// keys both the document's _vectors object and the hybrid search request, so the two
// cannot drift.
const SkillEmbedder = "skills"
```

Add to the `JobDocument` struct, after `CompanySlugFolded`:

```go
	// Vectors carries the job's skill vector under Meilisearch's reserved `_vectors`
	// key — the userProvided embedder that backs the match sort (see
	// internal/dict/skillvec). It is omitted entirely when there is no vector to
	// carry: `omitempty` matters, because a zero-length vector is a document
	// Meilisearch would reject, not one that simply does not participate.
	Vectors map[string][]float32 `json:"_vectors,omitempty"`
```

Change `FromJob`'s signature and body:

```go
func FromJob(j db.Job, w skillvec.Weights) (JobDocument, error) {
```

and, just before `return doc, nil`:

```go
	if v := w.Vector(j.Skills); v != nil {
		doc.Vectors = map[string][]float32{SkillEmbedder: v}
	}
```

Update the `maxIndexedDescriptionRunes` comment (`document.go:17-26`), whose closing
sentence claims the inverted index over `description` dominates the index size — the
skill vectors now add ~10 GB of their own:

```go
// keeps a fresh rebuild small enough to swap in within the host's free disk. The skill
// vectors (see JobDocument.Vectors) are the other large contributor, and unlike this
// one their width is fixed rather than tunable.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/search/search/ -run TestFromJob -v`
Expected: PASS. `go build ./...` still fails — the three callers have not been updated. That is Task 6.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/search/search/document.go internal/search/search/document_test.go
git add internal/search/search/document.go internal/search/search/document_test.go
git commit -m "feat(search): carry an IDF-weighted skill vector on the job document"
```

---

### Task 6: Update the three indexers

**Files:**
- Modify: `cmd/reindex/main.go:479`
- Modify: `cmd/search-drain/indexer.go:109`
- Modify: `internal/ingest/linkimport/linkimport.go:319`

**Interfaces:**
- Consumes: `search.LoadSkillWeights` (Task 4), the new `FromJob` signature (Task 5).
- Produces: nothing new — this restores a compiling build.

Each indexer loads the weights **once per run** (they are a snapshot of catalogue
rarity, not per-document state) and threads them to every `FromJob` call.

- [ ] **Step 1: Verify the build is broken in exactly three places**

Run: `go build ./... 2>&1 | grep FromJob`
Expected: three errors, at `cmd/reindex/main.go`, `cmd/search-drain/indexer.go`, and `internal/ingest/linkimport/linkimport.go`.

- [ ] **Step 2: Update cmd/reindex**

Load the weights once, before the row loop that reaches `search.FromJob(j)` at line
479, and pass them in. Where the run's other one-off setup happens:

```go
	// Skill-rarity weights for the match sort's vectors: one snapshot per run, since
	// they describe the catalogue rather than any single posting. A failure here is
	// NOT fatal — a rebuild that drops the match sort is far better than no rebuild,
	// and this one already refuses to run on a tight disk.
	skillWeights, err := search.LoadSkillWeights(ctx, queries)
	if err != nil {
		log.Printf("reindex: skill weights unavailable, match sort will be absent from this rebuild: %v", err)
	}
```

then at line 479: `doc, err := search.FromJob(j, skillWeights)`.

- [ ] **Step 3: Update cmd/search-drain**

Same shape: load once where the drain sets up, pass at line 109.

```go
	skillWeights, err := search.LoadSkillWeights(ctx, queries)
	if err != nil {
		log.Printf("search-drain: skill weights unavailable, this wave's documents carry no skill vector: %v", err)
	}
```

then `doc, err := search.FromJob(job, skillWeights)`.

- [ ] **Step 4: Update linkimport**

`linkimport` writes one job at a time on a user-facing path, so a per-call rollup read
would be a query per import. Load the weights where the importer is constructed and
hold them on the struct; if that is awkward, load lazily once and cache. At line 319:

```go
	doc, err := search.FromJob(saved.Job, s.skillWeights)
```

- [ ] **Step 5: Verify the build and the full suite**

```bash
go build ./...
go vet ./...
go test ./...
go vet -tags=integration ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
gofmt -w cmd/reindex/main.go cmd/search-drain/indexer.go internal/ingest/linkimport/linkimport.go
git add cmd/reindex/main.go cmd/search-drain/indexer.go internal/ingest/linkimport/linkimport.go
git commit -m "feat(search): thread skill weights through the three indexers"
```

---

### Task 7: Declare the embedder and accept a query vector

**Files:**
- Modify: `internal/search/search/client.go:535` (`facetSettings`), `:440-447` (`SearchParams`), `:472-500` (`Search`), `:27` and `:3` (the "NO embedder" comments)
- Test: `internal/search/search/settings_test.go`

**Interfaces:**
- Consumes: `skillvec.Dimensions` (Task 2), `SkillEmbedder` (Task 5).
- Produces: `SearchParams.Vector []float32`.

- [ ] **Step 1: Write the failing test**

```go
func TestFacetSettingsDeclaresTheSkillEmbedder(t *testing.T) {
	s := facetSettings()
	e, ok := s.Embedders[SkillEmbedder]
	if !ok {
		t.Fatalf("facetSettings() declares no %q embedder", SkillEmbedder)
	}
	if e.Source != meilisearch.UserProvidedEmbedderSource {
		t.Errorf("embedder source = %v, want userProvided — no model may be called at index time", e.Source)
	}
	if e.Dimensions != skillvec.Dimensions {
		t.Errorf("embedder dimensions = %d, want %d", e.Dimensions, skillvec.Dimensions)
	}
	if e.BinaryQuantized {
		t.Error("binary quantization must stay off: on vectors this sparse it drops recall@20 from 95% to 10%")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/search/ -run TestFacetSettings -v`
Expected: FAIL — `s.Embedders` is nil.

- [ ] **Step 3: Declare the embedder**

In `facetSettings()`, after `RankingRules`:

```go
		// The skill-match sort's embedder. userProvided means Meilisearch never calls a
		// model: the vectors are arithmetic over a finite dictionary (internal/dict/skillvec),
		// written by the indexers. Dimensions are the registry's declared width and CANNOT
		// change without a full rebuild. Binary quantization is deliberately off — these
		// vectors carry 2-12 non-zeros out of 749, and a sign-bit quantiser measured
		// recall@20 of 10% against 95% unquantized, taking the rare skills with it.
		Embedders: map[string]meilisearch.Embedder{
			SkillEmbedder: {
				Source:     meilisearch.UserProvidedEmbedderSource,
				Dimensions: skillvec.Dimensions,
			},
		},
```

Update the two "NO embedder" comments (`client.go:3` and `:27`) to say the index
carries one userProvided embedder for skill vectors and no model-backed one.

- [ ] **Step 4: Add the query vector**

Extend `SearchParams` (`client.go:440-447`):

```go
	// Vector, when set, ranks results by cosine against it using the SkillEmbedder
	// instead of by text relevance — the match sort. It composes with Filter: the
	// engine applies both in one query, so the facets need no separate pass.
	Vector []float32
```

In `Search` (`client.go:472`), after building `req`:

```go
	if len(p.Vector) > 0 {
		req.Vector = p.Vector
		// semanticRatio 1.0 asks for pure vector ranking. Anything less blends in the
		// keyword score, which for an empty query is noise.
		req.Hybrid = &meilisearch.SearchRequestHybrid{Embedder: SkillEmbedder, SemanticRatio: 1.0}
	}
```

- [ ] **Step 5: Write the query-vector test**

```go
func TestSearchParamsVectorRequestsHybridRanking(t *testing.T) {
	// The fake Meili records the request it received; assert Vector and Hybrid are set
	// together, since a Vector without an embedder name is a 400 from the engine.
	// (Follow the existing fake-transport pattern in this package's client tests.)
}
```

Implement it against whatever fake transport the package's existing client tests use;
if there is none, assert on a small helper extracted from `Search` that builds the
`*meilisearch.SearchRequest`, and call that helper from `Search`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/search/search/ -v`
Expected: PASS.

- [ ] **Step 7: Format, vet, commit**

```bash
gofmt -w internal/search/search/client.go internal/search/search/settings_test.go
go vet ./...
go test ./...
git add internal/search/search/client.go internal/search/search/settings_test.go
git commit -m "feat(search): declare the skill embedder and accept a query vector"
```

---

### Task 8: The sort=match request path

**Files:**
- Modify: `internal/api/handler/search.go:44` (route), `:90-97` (`searchSortable`), `:120-147` (`runJobSearch`), `:150-172` (`searchSort`)
- Test: `internal/api/handler/search_test.go`

**Interfaces:**
- Consumes: `SearchParams.Vector` (Task 7), `search.LoadSkillWeights` (Task 4), `userprofile.Service.Get` (existing).
- Produces: `?sort=match` on `/jobs/search`.

`/jobs/search` is public today. It gains `mw.optional` — attaches the caller when
signed in, never rejects — the same middleware `/jobs/:slug` already uses.

**Degradation rule:** `sort=match` from a caller with no session, no profile, or no
recognised skills falls back to the default feed. It never errors. `/jobs` already
ignores unknown filters for exactly this reason: a shared or saved link carrying the
param must not 400.

- [ ] **Step 1: Write the failing test**

```go
func TestSearchSortMatchAnonymousFallsBackToTheDefaultFeed(t *testing.T) {
	fake := &fakeSearcher{}
	app := newTestApp(fake)

	status, _ := doGet(t, app, "/jobs/search?sort=match")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — an unusable match sort degrades, never errors", status)
	}
	if len(fake.lastParams.Vector) != 0 {
		t.Errorf("anonymous request carried a vector of %d floats, want none", len(fake.lastParams.Vector))
	}
	if !slices.Equal(fake.lastParams.Sort, []string{"posted_at:desc"}) {
		t.Errorf("sort = %v, want the default freshest-first feed", fake.lastParams.Sort)
	}
}

func TestSearchSortMatchSignedInSendsTheVector(t *testing.T) {
	fake := &fakeSearcher{}
	app := newTestAppWithProfile(fake, userprofile.Profile{Skills: []string{"go", "docker"}})

	status, _ := doGet(t, app, "/jobs/search?sort=match")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.lastParams.Vector) != skillvec.Dimensions {
		t.Errorf("vector width = %d, want %d", len(fake.lastParams.Vector), skillvec.Dimensions)
	}
	if fake.lastParams.Sort != nil {
		t.Errorf("sort = %v, want nil — an explicit sort would override the vector ranking", fake.lastParams.Sort)
	}
}

func TestSearchSortMatchWithNoSkillsFallsBack(t *testing.T) {
	fake := &fakeSearcher{}
	app := newTestAppWithProfile(fake, userprofile.Profile{Skills: nil})

	status, _ := doGet(t, app, "/jobs/search?sort=match")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.lastParams.Vector) != 0 {
		t.Error("a skill-less profile still produced a vector")
	}
}

func TestSearchSortMatchComposesWithFacetFilters(t *testing.T) {
	fake := &fakeSearcher{}
	app := newTestAppWithProfile(fake, userprofile.Profile{Skills: []string{"go"}})

	doGet(t, app, "/jobs/search?sort=match&countries=DE&seniority=senior")

	if len(fake.lastParams.Vector) == 0 {
		t.Error("vector was dropped when facets were present")
	}
	if fake.lastParams.Filter == nil {
		t.Error("facet filter was dropped when the match sort was requested")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/handler/ -run TestSearchSortMatch -v`
Expected: FAIL — the anonymous case may pass incidentally, but the signed-in cases fail with an empty vector.

- [ ] **Step 3: Implement**

Attach optional auth to the route (`search.go:44`):

```go
	api.Get("/jobs/search", readLimit, mw.optional, h.SearchJobs)
```

Add the sort name and the request-time vector. `searchSortable` maps names to index
attributes and `match` has none — it is not an attribute sort — so it is handled
before that lookup:

```go
// sortMatch is the profile-match sort. Unlike every value in searchSortable it is not
// an index attribute: it ranks by cosine against the caller's own skill vector, which
// is why it is resolved from the request rather than looked up in that table.
const sortMatch = "match"

// matchVector builds the caller's skill vector for ?sort=match, or nil when the sort
// was not asked for or cannot be served. Every "cannot" degrades to the default feed:
// no session, no profile, no skills, no weights loaded. A saved search or shared link
// carrying sort=match must never 400 — the same reason /jobs ignores unknown filters.
func (h *searchHandlers) matchVector(c *fiber.Ctx) []float32 {
	if c.Query("sort") != sortMatch {
		return nil
	}
	// auth.UserID is what mw.optional populates; it reports false for an anonymous
	// caller rather than erroring. requireUserID is the wrong helper here — its 401 is
	// exactly the degradation this must avoid.
	userID, ok := auth.UserID(c)
	if !ok {
		return nil
	}
	profile, err := h.userProfile.Get(c.Context(), userID)
	if err != nil {
		return nil
	}
	weights, err := h.skillWeights(c.Context())
	if err != nil {
		return nil
	}
	return weights.Vector(profile.Skills)
}
```

`skillWeights` reads through the handler's existing `cache.Cache` — the weights are a
catalogue-wide snapshot that changes only when `cmd/rollup-facets` runs, so a
per-request query would be pure waste. Follow the TTL pattern already used for facet
distributions in `facets.go`.

In `runJobSearch`, resolve the vector once and pass it, and suppress the attribute
sort when it is present:

```go
	vector := h.matchVector(c)
	res, err := h.search.Search(c.Context(), search.SearchParams{
		Query:  c.Query("q"),
		Filter: buildSearchFilter(c),
		Sort:   searchSort(c, vector != nil),
		Vector: vector,
		Limit:  limit,
		Offset: offset,
	})
```

and in `searchSort`, take the new argument:

```go
// searchSort builds the Meilisearch sort directive from ?sort=<field>&order=<dir>.
// Without a valid sort param, a no-text browse defaults to the freshest postings
// first (posted_at desc) — relevance is meaningless for an empty query — while a
// text query keeps relevance order (nil).
//
// ranked reports that the request carries a match vector. An explicit sort directive
// takes precedence over vector ranking in Meilisearch, so returning one here would
// silently discard the match order the caller asked for.
func searchSort(c *fiber.Ctx, ranked bool) []string {
	if ranked {
		return nil
	}
	attr, ok := searchSortable[c.Query("sort")]
	...
}
```

Add `userProfile` to the `searchHandlers` struct and its constructor, wiring it where
`newSearchHandlers` is called.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/handler/ -run TestSearchSortMatch -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite including tagged tests**

```bash
gofmt -w internal/api/handler/search.go internal/api/handler/search_test.go
go vet ./...
go test ./...
go vet -tags=integration ./...
go test -tags=integration ./internal/api/handler/
```

Expected: all pass. `internal/api/handler` holds 78 integration-tagged tests that call
unexported constructors — a changed `newSearchHandlers` signature breaks them, and only
the tagged run catches it.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handler/
git commit -m "feat(api): rank /jobs/search by the caller's skill vector on sort=match"
```

---

### Task 9: Expose the option in the SPA

**Files:**
- Modify: `web/src/lib/facetModel.ts` (the sort vocabulary)
- Modify: the sort selector component (find it with `grep -rn "posted_at" web/src/lib/components/`)
- Test: `web/src/lib/facetModel.test.ts`

**Interfaces:**
- Consumes: `?sort=match` (Task 8).

Per `hire-grep-web-for-an-existing-implementation-first`: check `web/` for an existing
sort-option mapping before adding one — the vocabulary likely already lives as a plain
list beside `filtersFromParams`.

- [ ] **Step 1: Write the failing test**

```ts
it('round-trips the match sort', () => {
  expect(filtersFromParams(new URLSearchParams('sort=match')).sort).toBe('match');
});

it('keeps a match sort in the URL for an anonymous visitor — the server degrades it, the client does not strip it', () => {
  // A shared link must survive being opened signed-out and then signing in.
  const f = filtersFromParams(new URLSearchParams('sort=match&countries=DE'));
  expect(f.sort).toBe('match');
  expect(f.countries).toEqual(['DE']);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm vitest run src/lib/facetModel.test.ts`
Expected: FAIL — `sort` is undefined for an unrecognised value.

- [ ] **Step 3: Add `match` to the sort vocabulary**

Add it to the allowed sort values in `facetModel.ts`, and to the sort selector's
options — shown only when the viewer is signed in and has profile skills, since the
server degrades it to the default feed otherwise and an option that silently does
nothing is worse than an absent one.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm vitest run src/lib/facetModel.test.ts`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
cd web && pnpm lint
git add web/src/lib/facetModel.ts web/src/lib/facetModel.test.ts web/src/lib/components/
git commit -m "feat(web): offer the profile-match sort on the jobs feed"
```

---

### Task 10: Document the rollout hazard

**Files:**
- Modify: `internal/search/search/AGENTS.md`
- Modify: `CLAUDE.md` (the `reindex` bullet under "Worker gotchas")

The embedder must reach the LIVE index settings before a binary that sends a query
vector goes out. This is the same ordering hazard `role_type` documents at
`client.go:565-570`: until the live index declares the attribute, a binary that
requests it hard-500s every caller.

- [ ] **Step 1: Add the hazard to the search AGENTS.md**

```markdown
## Skill vectors and the match sort

The facet index carries a `userProvided` embedder named `skills` — no model is ever
called; the vectors are arithmetic over `internal/dict/skillvec`'s permanent position
registry, written by the three indexers.

**Two ordering hazards, both of which look like an outage:**

1. **The embedder must exist in the LIVE index before a binary queries it.** A vector
   search against an index with no such embedder is a 400 from the engine, which
   surfaces as a failing `/jobs/search` for everyone who selected the sort. Settings
   patch first, binary second — the same rule `role_type` follows.
2. **The vectors only exist after a rebuild.** Declaring the embedder does not
   retro-fill 1.36M documents; until the rebuild lands, the match sort returns the
   handful of postings re-indexed since. A rebuild adds ~10 GB and runs materially
   longer than a vector-less one (measured ~5x slower per document on a synthetic
   50k-document index), so it must be scheduled against the disk floor rather than
   dropped into the ordinary timer.

`skillvec.Dimensions` is baked into the live index settings. **Changing it requires a
full rebuild**, and until that rebuild finishes the index rejects every document whose
vector is the new width.
```

- [ ] **Step 2: Add the note to CLAUDE.md's reindex bullet**

Extend the existing `reindex` bullet under "Worker gotchas" noting that a rebuild now
also writes skill vectors, costs ~10 GB more, and runs longer.

- [ ] **Step 3: Commit**

```bash
git add internal/search/search/AGENTS.md CLAUDE.md
git commit -m "docs(search): record the skill-embedder rollout ordering hazard"
```

---

## Self-Review

**Spec coverage.** Every design section maps to a task: the vector and its IDF weights
(Tasks 2-4), storage on the document (Task 5), the indexers (Task 6), the embedder
declaration and query path (Task 7), the request behaviour including all three
degradation cases (Task 8), the UI (Task 9), the rollout hazard (Task 10). The
position registry's append-only rule is enforced by the generator (Task 2, Step 3) and
asserted by `TestRegistryCoversEveryCanonicalSkill`.

**Not covered, deliberately.** The design's "Blocked on" — host2 disk — is operational
work outside this plan. Task 10 documents the constraint so nobody deploys into it.
The design's open note about narrowing `Dimensions` from 1024 to ~800 is left to
implementation: the constant is in one place (Task 2, Step 4) and the decision needs
the measured index size, which only exists after Task 7 runs against real data.

**Type consistency.** `skillvec.Weights` flows unchanged from `WeightsFromCounts`
(Task 3) through `LoadSkillWeights` (Task 4), `FromJob` (Task 5), the three indexers
(Task 6), to `matchVector` (Task 8). `SkillEmbedder` is defined once (Task 5) and used
by both the document and the settings/query (Task 7). `skillvec.Dimensions` is
referenced by the registry test (Task 2), the vector builder (Task 3), the settings
test (Task 7), and the handler test (Task 8) — one constant throughout.

**One known soft spot.** Task 7, Step 5 does not spell out the client test because the
package's fake-transport shape is not visible from the design; the step names the
fallback (extract the request builder and test that) so the implementer is not left
guessing.
