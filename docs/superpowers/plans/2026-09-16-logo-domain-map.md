# Logo Domain Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make company logos resolve by the company's own web domain instead of a raw adapter name string, so one company shows one correct logo on every surface.

**Architecture:** `hire` publishes a JSON snapshot mapping normalized company-name spellings to bare domains, produced by a daily worker from `companies.company_info->>'website'` and the spellings observed in open jobs. `freehire-logo` loads that snapshot and, when a request carries no `?domain=`, supplies the mapped domain to its existing resolver. No call site changes in either repo.

**Tech Stack:** Go, pgx, sqlc, systemd timers. Two repositories: `hire` (this one) and `freehire-logo` (`~/Projects/freehire-logo`).

**Design doc:** `docs/superpowers/specs/2026-09-16-logo-domain-map-design.md`

## Global Constraints

- English only in all code, comments, identifiers, docs and commit messages.
- Every new package must be added to the block table in `internal/platform/arch/layering/blocks.go`, or the layering guard fails.
- `internal/platform/db` is generated: edit `internal/platform/db/queries/*.sql`, then run `make sqlc`. Never hand-edit generated files.
- Before committing any `*.go`: `gofmt -w` those paths, then `go vet ./...` and `go test ./...`.
- Before pushing: `go vet -tags=integration ./...`.
- The snapshot file format version is `1` and its normalization identifier is the exact string `lower-collapse-ws`.
- Name normalization must be exactly: lower-case, split on unicode whitespace runs, rejoin with a single space. Punctuation is never touched.
- The worker env knobs are `LOGO_DOMAIN_MAP_OUT` (path; unset = no-op that never opens the pool). The proxy env knob is `LOGO_DOMAIN_MAP` (path; unset = map never loaded).

---

## File Structure

**`hire` (this repo)**

| File | Responsibility |
|---|---|
| `internal/job/logodomain/logodomain.go` | Pure: name normalization, domain extraction from a stored website value, and building a collision-free map. No I/O. |
| `internal/job/logodomain/snapshot.go` | The on-disk format: the `Snapshot` struct, `Encode`, and the atomic `WriteFile`. |
| `internal/job/logodomain/logodomain_test.go` | Table tests for normalization, domain extraction, collisions. |
| `internal/job/logodomain/snapshot_test.go` | Round-trip encode, atomic write leaves no partial file. |
| `internal/platform/db/queries/logodomain.sql` | The two reads. |
| `cmd/publish-logo-domains/main.go` | The worker: read, build, write, report. |
| `internal/platform/arch/layering/blocks.go` | Add `logodomain` to the `job` block. |
| `deploy/systemd/freehire-publish-logo-domains.service` / `.timer` | Daily run. |
| `deploy/bin/release.sh` | Add the binary to the build list. |
| `CLAUDE.md` | One worker-gotchas bullet. |

**`freehire-logo` (`~/Projects/freehire-logo`)**

| File | Responsibility |
|---|---|
| `internal/domainmap/domainmap.go` | Load, validate, reload-on-mtime, `Lookup`. |
| `internal/domainmap/domainmap_test.go` | Validation, missing file, reload. |
| `internal/server/server.go` | One branch: consult the map when `?domain=` is absent. |
| `cmd/logo-proxy/main.go` | Read `LOGO_DOMAIN_MAP`, build the map, pass it to `server.New`. |
| `README.md` | Document the map. |

---

# Part A — `hire`: publish the map

## Task A1: The pure `logodomain` package

**Files:**
- Create: `internal/job/logodomain/logodomain.go`
- Create: `internal/job/logodomain/logodomain_test.go`
- Modify: `internal/platform/arch/layering/blocks.go` (the `"job"` block list, around line 153-186)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func NormalizeName(name string) string`
  - `func Domain(website string) string`
  - `type Spelling struct { Slug, Name string }`
  - `func Build(websites map[string]string, spellings []Spelling) (entries map[string]string, dropped int)`

- [ ] **Step 1: Write the failing test**

Create `internal/job/logodomain/logodomain_test.go`:

```go
package logodomain

import "testing"

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "G2i", "g2i"},
		{"collapses whitespace runs", "  Network   Recruitment  ", "network recruitment"},
		{"keeps punctuation", "G2i Inc.", "g2i inc."},
		{"keeps hyphens distinct", "3-M", "3-m"},
		{"empty stays empty", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeName(tt.in); got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDomain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"full url", "https://g2i.co", "g2i.co"},
		{"url with path", "https://acme.com/careers", "acme.com"},
		{"bare host", "acme.com", "acme.com"},
		{"strips www", "https://www.acme.com/", "acme.com"},
		{"strips port", "https://acme.com:8443/", "acme.com"},
		{"strips root label", "acme.com.", "acme.com"},
		{"uppercase folds", "HTTPS://ACME.COM", "acme.com"},
		{"no dot is not a domain", "https://localhost/", ""},
		{"junk", "not a url at all", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Domain(tt.in); got != tt.want {
				t.Errorf("Domain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildMapsEverySpellingOfACompanyToOneDomain(t *testing.T) {
	websites := map[string]string{"g2i": "https://g2i.co"}
	spellings := []Spelling{
		{Slug: "g2i", Name: "g2i"},
		{Slug: "g2i", Name: "G2i"},
		{Slug: "g2i", Name: "G2i Inc."},
	}
	entries, dropped := Build(websites, spellings)
	if dropped != 0 {
		t.Fatalf("dropped = %d, want 0", dropped)
	}
	// "g2i" and "G2i" normalize onto one key, so three spellings yield two entries.
	want := map[string]string{"g2i": "g2i.co", "g2i inc.": "g2i.co"}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v, want %v", entries, want)
	}
	for k, v := range want {
		if entries[k] != v {
			t.Errorf("entries[%q] = %q, want %q", k, entries[k], v)
		}
	}
}

func TestBuildSkipsCompaniesWithNoUsableWebsite(t *testing.T) {
	websites := map[string]string{"ghost": "not a url at all"}
	entries, dropped := Build(websites, []Spelling{{Slug: "ghost", Name: "Ghost"}})
	if len(entries) != 0 {
		t.Errorf("entries = %v, want empty", entries)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0 — an unusable website is not a collision", dropped)
	}
}

func TestBuildDropsANameTwoCompaniesShare(t *testing.T) {
	// A wrong domain returns a confident logo belonging to someone else, which is the
	// exact failure this package exists to remove. Neither answer is published.
	websites := map[string]string{"acme-one": "https://acme-one.com", "acme-two": "https://acme-two.com"}
	spellings := []Spelling{
		{Slug: "acme-one", Name: "Acme"},
		{Slug: "acme-two", Name: "ACME"},
	}
	entries, dropped := Build(websites, spellings)
	if _, ok := entries["acme"]; ok {
		t.Errorf("entries kept the colliding name: %v", entries)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
}

func TestBuildKeepsANameTwoCompaniesShareWhenTheDomainAgrees(t *testing.T) {
	// Two slugs for one employer that our alias registry has not merged yet still point
	// at one website. There is nothing ambiguous to protect against.
	websites := map[string]string{"acme": "https://acme.com", "acme-inc": "https://www.acme.com/"}
	spellings := []Spelling{
		{Slug: "acme", Name: "Acme"},
		{Slug: "acme-inc", Name: "Acme"},
	}
	entries, dropped := Build(websites, spellings)
	if entries["acme"] != "acme.com" {
		t.Errorf("entries[%q] = %q, want %q", "acme", entries["acme"], "acme.com")
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /tmp/wt-logo-domain && go test ./internal/job/logodomain/`
Expected: FAIL — the package does not build (`undefined: NormalizeName`).

- [ ] **Step 3: Write the implementation**

Create `internal/job/logodomain/logodomain.go`:

```go
// Package logodomain builds the name-to-domain map the logo proxy consults.
//
// Company logos are requested from logo.freehire.me by the company's NAME, taken
// verbatim from whichever adapter ingested the row. One employer arrives spelled several
// ways ("g2i", "G2i", "G2i Inc."), and the upstream that resolves a name answers a
// confident 200 with a DIFFERENT company's mark often enough to matter — g2i's name
// resolves to an unrelated leadership-training firm. A domain removes the ambiguity at
// the source, and the proxy has accepted one since it was written; nothing has ever
// supplied it.
//
// This package owns the two facts the proxy cannot have: which name spellings belong to
// one company (the stored company_slug, computed by normalize.CompanySlug at ingest) and
// what that company's website is (companies.company_info->>'website').
package logodomain

import (
	"net/url"
	"strings"
)

// NormalizeName collapses the spellings of one company onto one lookup key. It MUST stay
// byte-for-byte equivalent to freehire-logo's store.Normalize, which is what the proxy
// hashes its cache under and what it will look this map up by: case-folded, runs of
// whitespace reduced to one space, ends trimmed.
//
// Punctuation is deliberately left alone, matching the proxy: "3M" and "3-M" may be
// different companies, and merging them would serve one company's logo for the other —
// worse than serving two keys. That is why "g2i" and "g2i inc." remain separate keys
// here and both get an entry, rather than one being folded into the other.
//
// The two copies of this rule live in two repositories and would drift silently, so the
// snapshot declares its normalization (see NormalizationID) and the proxy refuses a file
// whose value it does not implement.
func NormalizeName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// Domain reduces a stored website value to the bare registrable domain the proxy's
// resolve.Query documents — "g2i.co", no scheme, no port, no path, no leading "www.".
//
// Returns "" for anything that is not plausibly a hostname. An unusable website is not an
// error: the company simply gets no entry and the proxy keeps resolving it by name, which
// is what happens today.
func Domain(website string) string {
	trimmed := strings.TrimSpace(website)
	if trimmed == "" {
		return ""
	}
	// url.Parse only fills Host when there is an authority, and stored values are both
	// "https://acme.com/x" and a bare "acme.com".
	if !strings.Contains(trimmed, "//") {
		trimmed = "//" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	// Hostname drops the port and any [] around an IPv6 literal; userinfo lands in
	// parsed.User and is discarded with it.
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimSuffix(host, ".") // the DNS root label, which no upstream wants
	host = strings.TrimPrefix(host, "www.")
	// A registrable domain has a dot and no whitespace. This is deliberately not a public
	// suffix check: the value came from a curated company record, not from a user.
	if !strings.Contains(host, ".") || strings.ContainsAny(host, " \t/?#") {
		return ""
	}
	return host
}

// Spelling is one observed way a company's name is written in the catalogue.
type Spelling struct {
	Slug string
	Name string
}

// Build turns the company websites and the observed spellings into the published map.
//
// websites is keyed by company slug and holds the raw stored value; spellings may repeat
// a (slug, name) pair harmlessly. dropped counts the names two DIFFERENT companies share
// while disagreeing about the domain: those are omitted entirely, because publishing
// either one returns a confident logo belonging to the other company.
func Build(websites map[string]string, spellings []Spelling) (entries map[string]string, dropped int) {
	entries = make(map[string]string, len(websites))
	// conflicted remembers the keys we have already refused, so a name shared by three
	// companies is counted once rather than once per extra company.
	conflicted := make(map[string]bool)
	for _, s := range spellings {
		key := NormalizeName(s.Name)
		if key == "" || conflicted[key] {
			continue
		}
		domain := Domain(websites[s.Slug])
		if domain == "" {
			continue
		}
		switch existing, seen := entries[key]; {
		case !seen:
			entries[key] = domain
		case existing != domain:
			delete(entries, key)
			conflicted[key] = true
			dropped++
		}
	}
	return entries, dropped
}
```

- [ ] **Step 4: Register the package with the layering guard**

In `internal/platform/arch/layering/blocks.go`, inside the `"job"` block list, add
`logodomain` in alphabetical position with its justification comment. Insert it
immediately after the `"jobderive", "jobfacts", "jobhash", "jobreality", "jobview",
"liveness",` line:

```go
		// logodomain builds the company-name-to-domain map the logo proxy consults. It
		// is here and not in dict because it is not a dictionary: it reads the stored
		// company website and the spellings the catalogue happens to hold, which are
		// facts about companies and postings, the same footing as wikicompany below.
		"logodomain",
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /tmp/wt-logo-domain && go test ./internal/job/logodomain/ ./internal/platform/arch/layering/`
Expected: PASS for both.

- [ ] **Step 6: Commit**

```bash
cd /tmp/wt-logo-domain
gofmt -w internal/job/logodomain/ internal/platform/arch/layering/blocks.go
go vet ./... && go test ./...
git add internal/job/logodomain internal/platform/arch/layering/blocks.go
git commit -m "Add logodomain: name spellings to one company domain

One employer arrives spelled several ways and the logo proxy resolves by
name, which answers a confident 200 with another company's mark often
enough to matter. A domain removes the ambiguity, and the proxy has
accepted one since it was written.

A name two different companies share with different domains is dropped
rather than resolved: publishing either returns a confident logo
belonging to the other."
```

---

## Task A2: The two reads

**Files:**
- Create: `internal/platform/db/queries/logodomain.sql`
- Modify: generated files under `internal/platform/db/` (via `make sqlc`, never by hand)

**Interfaces:**
- Consumes: nothing from Task A1.
- Produces:
  - `func (q *Queries) ListCompanyWebsites(ctx context.Context) ([]ListCompanyWebsitesRow, error)` with `Slug string; Website string`
  - `func (q *Queries) ListOpenJobCompanySpellings(ctx context.Context) ([]ListOpenJobCompanySpellingsRow, error)` with `CompanySlug string; Company string`

- [ ] **Step 1: Write the queries**

Create `internal/platform/db/queries/logodomain.sql`:

```sql
-- ListCompanyWebsites returns the companies whose curated record holds a website, which
-- is the only population cmd/publish-logo-domains can publish a domain for. ~17,900 rows
-- of ~480,000 companies as of 2026-09-16.
--
-- name: ListCompanyWebsites :many
SELECT slug, company_info ->> 'website' AS website
FROM companies
WHERE company_info ? 'website'
  AND company_info ->> 'website' <> ''
  AND job_count > 0;

-- ListOpenJobCompanySpellings returns every distinct way an open posting spells its
-- company's name. 412,648 rows as of 2026-09-16.
--
-- This is a deliberate SEQUENTIAL SCAN of jobs, and the narrower-looking alternative is
-- four times slower. Measured on production 2026-09-16:
--
--   this query                                        53s  (seq scan)
--   the same joined to the 17,859 companies above    200s  (index nested loop)
--
-- Restricting to the companies we can publish drives 17,859 index searches, each fetching
-- ~102 heap rows at random: 1.59M blocks of random I/O against the seq scan's sequential
-- read of the same heap. Fewer rows, more work. The caller filters in Go instead.
--
-- name: ListOpenJobCompanySpellings :many
SELECT DISTINCT company_slug, company
FROM jobs
WHERE closed_at IS NULL
  AND company_slug <> ''
  AND company <> '';
```

- [ ] **Step 2: Regenerate and verify the generated signatures**

Run: `cd /tmp/wt-logo-domain && make sqlc`
Then: `grep -n "ListCompanyWebsites\|ListOpenJobCompanySpellings" internal/platform/db/logodomain.sql.go`
Expected: both functions present, with the row types named above.

If sqlc renders `website` as `interface{}` or `pgtype.Text` rather than `string`, add an
explicit cast in the query (`company_info ->> 'website' AS website` becomes
`(company_info ->> 'website')::text AS website`) and regenerate.

- [ ] **Step 3: Verify the whole module still builds**

Run: `cd /tmp/wt-logo-domain && go build ./... && go vet ./...`
Expected: no output.

- [ ] **Step 4: Commit**

```bash
cd /tmp/wt-logo-domain
git add internal/platform/db
git commit -m "Add the two reads the logo domain map is built from

The spellings query is a deliberate sequential scan. Joining it to the
17,859 companies that have a website is four times slower on production
(200s against 53s): the narrow plan drives 17,859 index searches fetching
~102 heap rows each at random, 1.59M blocks of random I/O against the
seq scan's sequential read of the same heap. The caller filters in Go."
```

---

## Task A3: The snapshot format

**Files:**
- Create: `internal/job/logodomain/snapshot.go`
- Create: `internal/job/logodomain/snapshot_test.go`

**Interfaces:**
- Consumes: `NormalizeName` from Task A1 (only as documentation of the contract).
- Produces:
  - `const Version = 1`
  - `const NormalizationID = "lower-collapse-ws"`
  - `type Snapshot struct { Version int; Normalization string; GeneratedAt time.Time; Entries map[string]string }`
  - `func NewSnapshot(entries map[string]string, now time.Time) Snapshot`
  - `func WriteFile(path string, s Snapshot) error`

- [ ] **Step 1: Write the failing test**

Create `internal/job/logodomain/snapshot_test.go`:

```go
package logodomain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSnapshotDeclaresTheContract(t *testing.T) {
	s := NewSnapshot(map[string]string{"g2i": "g2i.co"}, time.Unix(0, 0).UTC())
	if s.Version != Version {
		t.Errorf("Version = %d, want %d", s.Version, Version)
	}
	if s.Normalization != NormalizationID {
		t.Errorf("Normalization = %q, want %q", s.Normalization, NormalizationID)
	}
	if s.Entries["g2i"] != "g2i.co" {
		t.Errorf("Entries = %v", s.Entries)
	}
}

func TestWriteFileRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	want := NewSnapshot(map[string]string{"g2i": "g2i.co", "g2i inc.": "g2i.co"}, time.Unix(0, 0).UTC())
	if err := WriteFile(path, want); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Snapshot
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Version != want.Version || got.Normalization != want.Normalization {
		t.Errorf("header = %d/%q, want %d/%q", got.Version, got.Normalization, want.Version, want.Normalization)
	}
	if len(got.Entries) != 2 || got.Entries["g2i inc."] != "g2i.co" {
		t.Errorf("Entries = %v", got.Entries)
	}
}

func TestWriteFileLeavesNoTempFileBehind(t *testing.T) {
	// The proxy reloads on mtime and must never read a half-written map, so the write
	// goes to a temp file and renames. A leftover temp file would also be picked up by
	// the cache backup that mirrors this directory.
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.json")
	if err := WriteFile(path, NewSnapshot(map[string]string{"g2i": "g2i.co"}, time.Now())); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "domains.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only domains.json", names)
	}
}

func TestWriteFileReplacesAnExistingMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := WriteFile(path, NewSnapshot(map[string]string{"old": "old.com"}, time.Now())); err != nil {
		t.Fatalf("first WriteFile: %v", err)
	}
	if err := WriteFile(path, NewSnapshot(map[string]string{"new": "new.com"}, time.Now())); err != nil {
		t.Fatalf("second WriteFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Snapshot
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, stale := got.Entries["old"]; stale {
		t.Errorf("Entries still hold the previous map: %v", got.Entries)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /tmp/wt-logo-domain && go test ./internal/job/logodomain/ -run Snapshot`
Expected: FAIL — `undefined: NewSnapshot`.

- [ ] **Step 3: Write the implementation**

Create `internal/job/logodomain/snapshot.go`:

```go
package logodomain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Version is the snapshot format version. The proxy refuses a version it does not
// implement rather than guessing at the fields, because a map it misreads is a map that
// silently matches nothing.
const Version = 1

// NormalizationID names the rule Entries' keys were built with, so the proxy can check
// that the keys it looks up are keyed the way it hashes names. The two implementations
// live in two repositories and would otherwise drift in silence — and the symptom of
// drift is indistinguishable from the map simply not covering a company.
const NormalizationID = "lower-collapse-ws"

// Snapshot is the published map. Entries is normalized company name to bare registrable
// domain.
type Snapshot struct {
	Version       int               `json:"version"`
	Normalization string            `json:"normalization"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Entries       map[string]string `json:"entries"`
}

// NewSnapshot stamps entries with the contract the proxy validates against.
func NewSnapshot(entries map[string]string, now time.Time) Snapshot {
	return Snapshot{
		Version:       Version,
		Normalization: NormalizationID,
		GeneratedAt:   now.UTC(),
		Entries:       entries,
	}
}

// WriteFile publishes the snapshot at path, atomically.
//
// The proxy reloads whenever the file's mtime moves, so it can read at any moment: a
// plain os.WriteFile would hand it a truncated map and the JSON decode would fail for as
// long as the write lasts. Writing a temp file in the SAME directory and renaming makes
// the swap a single atomic operation — same directory because rename across filesystems
// is not atomic and not even permitted.
func WriteFile(path string, s Snapshot) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("logodomain: create temp: %w", err)
	}
	// Remove is a no-op once the rename has succeeded, and is what stops a failure
	// anywhere below leaving a temp file next to the map.
	defer os.Remove(tmp.Name())

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "")
	if err := enc.Encode(s); err != nil {
		tmp.Close()
		return fmt.Errorf("logodomain: encode: %w", err)
	}
	// Sync before the rename: the rename is atomic with respect to readers, but a crash
	// between them would publish a name pointing at unwritten blocks.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("logodomain: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("logodomain: close: %w", err)
	}
	// The temp file is created 0600; the proxy runs as a different user and must read it.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("logodomain: chmod: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("logodomain: rename: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /tmp/wt-logo-domain && go test ./internal/job/logodomain/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
cd /tmp/wt-logo-domain
gofmt -w internal/job/logodomain/
go vet ./... && go test ./...
git add internal/job/logodomain
git commit -m "Add the logo domain map's on-disk format

Written to a temp file in the same directory and renamed: the proxy
reloads on mtime and can read at any moment, so a plain write would hand
it a truncated map for the length of the write.

The snapshot declares its own normalization rule. The rule has two
implementations in two repositories, and the symptom of drift between
them is indistinguishable from the map not covering a company."
```

---

## Task A4: The `publish-logo-domains` worker

**Files:**
- Create: `cmd/publish-logo-domains/main.go`
- Create: `cmd/publish-logo-domains/main_test.go`

**Interfaces:**
- Consumes: `logodomain.{Build,Spelling,NewSnapshot,WriteFile}` (Tasks A1, A3); `db.Queries.{ListCompanyWebsites,ListOpenJobCompanySpellings}` (Task A2).
- Produces: the binary `publish-logo-domains`.

- [ ] **Step 1: Write the failing test**

Create `cmd/publish-logo-domains/main_test.go`:

```go
package main

import "testing"

func TestOutputPathIsUnsetByDefault(t *testing.T) {
	t.Setenv("LOGO_DOMAIN_MAP_OUT", "")
	if got := outputPath(); got != "" {
		t.Errorf("outputPath() = %q, want empty — the worker ships dark", got)
	}
}

func TestOutputPathReadsItsKnob(t *testing.T) {
	t.Setenv("LOGO_DOMAIN_MAP_OUT", "/var/lib/freehire-logo-map/domains.json")
	if got, want := outputPath(), "/var/lib/freehire-logo-map/domains.json"; got != want {
		t.Errorf("outputPath() = %q, want %q", got, want)
	}
}

func TestOutputPathTrimsWhitespace(t *testing.T) {
	// A trailing newline in an EnvironmentFile line would otherwise become part of the
	// path, and the resulting file would be invisible to the proxy under its real name.
	t.Setenv("LOGO_DOMAIN_MAP_OUT", "  /tmp/domains.json\n")
	if got, want := outputPath(), "/tmp/domains.json"; got != want {
		t.Errorf("outputPath() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /tmp/wt-logo-domain && go test ./cmd/publish-logo-domains/`
Expected: FAIL — `undefined: outputPath`.

- [ ] **Step 3: Write the implementation**

Create `cmd/publish-logo-domains/main.go`:

```go
// Command publish-logo-domains writes the company-name-to-domain map the logo proxy
// consults, then exits.
//
// Company logos are requested from logo.freehire.me by NAME, and the upstream that
// resolves a name answers a confident 200 with a different company's mark often enough to
// matter. The proxy has accepted a ?domain= since it was written and nothing has ever
// supplied one, because most of the ~25 call sites that draw a logo — the experience
// bank, search suggestions, community subjects, the tailored-CV list — never see a
// company slug and could not look a domain up. Publishing a map the proxy consults fixes
// every one of them without touching any.
//
// Unset LOGO_DOMAIN_MAP_OUT and this is a no-op that never opens the pool, which is both
// how the change ships dark and how it is rolled back.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/logodomain"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// outputPath is the binary's own reading of its knob. It exists so a test can assert what
// THIS worker does with THIS variable rather than re-stating the wiring.
//
// os.Getenv rather than a worker.Env helper: internal/platform/worker only wraps the
// NUMERIC knobs (EnvInt64/EnvInt32), where a set-but-unreadable value must fail the run
// instead of silently taking a default. A path has no such failure mode — it is used
// verbatim or it is empty.
func outputPath() string {
	return strings.TrimSpace(os.Getenv("LOGO_DOMAIN_MAP_OUT"))
}

func main() { worker.Main(run) }

func run() int {
	out := outputPath()
	if out == "" {
		log.Print("publish-logo-domains: LOGO_DOMAIN_MAP_OUT unset, nothing to publish")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)

	// The websites first: it is the small read, and a catalogue with none of them means
	// there is nothing this run could publish however the scan goes.
	websiteRows, err := q.ListCompanyWebsites(ctx)
	if err != nil {
		log.Printf("publish-logo-domains: websites: %v", err)
		return 1
	}
	websites := make(map[string]string, len(websiteRows))
	for _, r := range websiteRows {
		websites[r.Slug] = r.Website
	}
	log.Printf("publish-logo-domains: %d companies carry a website", len(websites))

	// ~53s and a sequential scan of jobs. See the query's own comment for why narrowing
	// it to the companies above is four times slower.
	started := time.Now()
	spellingRows, err := q.ListOpenJobCompanySpellings(ctx)
	if err != nil {
		log.Printf("publish-logo-domains: spellings: %v", err)
		return 1
	}
	log.Printf("publish-logo-domains: %d spellings scanned in %s", len(spellingRows), time.Since(started).Round(time.Second))

	spellings := make([]logodomain.Spelling, 0, len(spellingRows))
	for _, r := range spellingRows {
		spellings = append(spellings, logodomain.Spelling{Slug: r.CompanySlug, Name: r.Company})
	}
	// The companies' own names are keys too: /companies, the company header and the
	// company picker all ask the proxy with companies.name rather than a posting's
	// spelling of it.
	for slug := range websites {
		spellings = append(spellings, logodomain.Spelling{Slug: slug, Name: slug})
	}

	entries, dropped := logodomain.Build(websites, spellings)

	// A run that would publish an empty map refuses to swap. A catalogue yielding nothing
	// is a failed measurement, not an empty catalogue — the rule cmd/build-suggestions
	// already follows — and here the consequence of believing it is every logo on the
	// site reverting to a name guess at once.
	if len(entries) == 0 {
		log.Print("publish-logo-domains: refusing to publish an empty map")
		return 1
	}

	if err := logodomain.WriteFile(out, logodomain.NewSnapshot(entries, time.Now())); err != nil {
		log.Printf("publish-logo-domains: %v", err)
		return 1
	}
	log.Printf("publish-logo-domains: published %d entries to %s (%d names dropped as ambiguous)", len(entries), out, dropped)
	return 0
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /tmp/wt-logo-domain && go test ./cmd/publish-logo-domains/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /tmp/wt-logo-domain
gofmt -w cmd/publish-logo-domains/
go vet ./... && go test ./...
git add cmd/publish-logo-domains
git commit -m "Add publish-logo-domains

Writes the name-to-domain map the logo proxy consults. Unset
LOGO_DOMAIN_MAP_OUT and it is a no-op that never opens the pool, which
is both how this ships dark and how it is rolled back.

A run that would publish an empty map refuses to swap: a catalogue
yielding nothing is a failed measurement, and believing it would revert
every logo on the site to a name guess at once."
```

---

## Task A5: Ship it — unit, timer, build list, docs

**Files:**
- Create: `deploy/systemd/freehire-publish-logo-domains.service`
- Create: `deploy/systemd/freehire-publish-logo-domains.timer`
- Modify: `deploy/bin/release.sh:307` (the `for w in ...` build list)
- Modify: `CLAUDE.md` (the worker-gotchas list)

**Interfaces:**
- Consumes: the `publish-logo-domains` binary from Task A4.
- Produces: nothing other code reads.

- [ ] **Step 1: Read a neighbouring unit to copy its conventions**

Run: `cd /tmp/wt-logo-domain && cat deploy/systemd/freehire-build-suggestions.service deploy/systemd/freehire-build-suggestions.timer`
Note the `User=`, `EnvironmentFile=`, `WorkingDirectory=`, `Type=oneshot` and
`TimeoutStartSec=` values actually in use, and reuse them below rather than the
placeholders.

- [ ] **Step 2: Write the unit**

Create `deploy/systemd/freehire-publish-logo-domains.service`, matching the neighbour's
`User`/`EnvironmentFile`/`WorkingDirectory`:

```ini
[Unit]
Description=Publish the company name-to-domain map the logo proxy consults
After=network-online.target postgresql.service

[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire
EnvironmentFile=/opt/freehire/.env
# The scan is a sequential read of jobs, measured at 53s on 2026-09-16. The ceiling is
# generous because the host's bottleneck is the crawl fleet and this competes with it.
TimeoutStartSec=30min
Environment=LOGO_DOMAIN_MAP_OUT=/var/lib/freehire-logo-map/domains.json
# The proxy runs as its own user and only reads this directory. StateDirectory creates it
# 0755 under /var/lib on first start, which is what lets the two users share the file
# without either owning the other's tree.
StateDirectory=freehire-logo-map
ExecStart=/opt/freehire/bin/publish-logo-domains
```

- [ ] **Step 3: Write the timer**

Create `deploy/systemd/freehire-publish-logo-domains.timer`:

```ini
[Unit]
Description=Daily publish of the logo domain map

[Timer]
# 05:20 UTC: clear of the 03:00 pg_dump (which runs 2h42m) and of the 03:15/15:15
# reindex, and before build-suggestions at 06:45. A logo map an hour stale costs
# nothing, so this yields to everything.
OnCalendar=*-*-* 05:20:00 UTC
# A missed run is worth catching up: the map is a snapshot, not a schedule, and a host
# that was down at 05:20 would otherwise carry yesterday's map for a full extra day.
Persistent=true
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
```

- [ ] **Step 4: Add the binary to the release build list**

In `deploy/bin/release.sh:307`, append `publish-logo-domains` to the `for w in ...` list,
after `pro-welcome-mail`. A worker missing from this list is built nowhere and the deploy
succeeds without it — see the drift check the file's own comment at line 268 describes.

- [ ] **Step 5: Document the worker**

In `CLAUDE.md`, add a bullet to the worker-gotchas list (the one that starts
`- \`migrate\` — run **before** deploying...`), placed after the
`backfill-company-info-wikipedia` bullet:

```markdown
- `publish-logo-domains` — daily: writes the company-name-to-domain map `logo.freehire.me` consults, so a logo is resolved by the employer's own domain instead of by a name string. Company logos are requested by NAME (`web/src/lib/logo.ts`), taken verbatim from whichever adapter ingested the row, and one employer arrives spelled several ways — `g2i`, `G2i`, `G2i Inc.` all carry `company_slug = 'g2i'` and the first two resolved to an unrelated leadership-training firm, at HTTP 200, so no consumer could detect it and fall back to its monogram. The proxy has accepted `?domain=` since it was written and folds it into its cache key; nothing ever supplied one. **The fix is not at the call sites**: of ~25 places that draw a logo, roughly fifteen never see a company slug (the experience bank, search suggestions, community subjects, the tailored-CV list), so threading a domain would fix a third of the surfaces. Covers the **17,859** of 239,879 job-carrying companies whose `company_info` records a website; the rest keep today's name guess, and `backfill-company-info-wikipedia` grows that population daily. A name two DIFFERENT companies share while disagreeing about the domain is dropped, never resolved — a wrong domain returns a confident logo belonging to someone else, which is the failure this removes. **The spellings query is a deliberate sequential scan** (53s, 412,648 rows): joining it to the companies that have a website is four times slower (200s), because the narrow plan drives 17,859 index searches fetching ~102 heap rows each at random. Fewer rows, more I/O. A run that would publish an empty map refuses to swap. Needs `DATABASE_URL`; without `LOGO_DOMAIN_MAP_OUT` it is a **no-op that never opens the pool**, which is also the rollback. **The consumer is a different repository** (`freehire-logo`) that `release.sh` does not carry, so this half alone changes nothing visible.
```

- [ ] **Step 6: Verify nothing broke**

Run: `cd /tmp/wt-logo-domain && go build ./... && go vet ./... && go test ./... && pnpm check:links`
Expected: all pass. (`pnpm check:links` resolves the relative Markdown links; this edit adds none, so it is a regression check.)

- [ ] **Step 7: Commit**

```bash
cd /tmp/wt-logo-domain
git add deploy/systemd/freehire-publish-logo-domains.service \
        deploy/systemd/freehire-publish-logo-domains.timer \
        deploy/bin/release.sh CLAUDE.md
git commit -m "Ship publish-logo-domains: unit, timer, build list, docs

The build list in release.sh is the one that decides whether a worker
reaches the host at all; a binary missing from it deploys green and does
not exist.

Scheduled at 05:20 UTC, clear of the 03:00 pg_dump and the 03:15 reindex
and before build-suggestions at 06:45. A stale logo map costs nothing,
so it yields to everything."
```

---

# Part B — `freehire-logo`: consult the map

All paths in Part B are relative to `~/Projects/freehire-logo`. Work on a branch:

```bash
cd ~/Projects/freehire-logo && git fetch origin && git switch -c feat/domain-map origin/main
```

If `origin/main` does not exist, check `git branch -a` — the README says the service was
implemented on `feat/logo-proxy`; branch from whatever is deployed.

## Task B1: The `domainmap` package

**Files:**
- Create: `internal/domainmap/domainmap.go`
- Create: `internal/domainmap/domainmap_test.go`

**Interfaces:**
- Consumes: `store.Normalize` (existing, `internal/store`).
- Produces:
  - `type Map struct { ... }`
  - `func Load(path string, reloadEvery time.Duration) *Map`
  - `func (m *Map) Lookup(normalizedName string) string`
  - `func (m *Map) Size() int`

- [ ] **Step 1: Write the failing test**

Create `internal/domainmap/domainmap_test.go`:

```go
package domainmap

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLookupFindsAMappedName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	write(t, path, `{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co","g2i inc.":"g2i.co"}}`)
	m := Load(path, time.Minute)
	if got := m.Lookup("g2i"); got != "g2i.co" {
		t.Errorf("Lookup(%q) = %q, want %q", "g2i", got, "g2i.co")
	}
	if got := m.Lookup("g2i inc."); got != "g2i.co" {
		t.Errorf("Lookup(%q) = %q, want %q", "g2i inc.", got, "g2i.co")
	}
	if got := m.Lookup("nobody"); got != "" {
		t.Errorf("Lookup(%q) = %q, want empty", "nobody", got)
	}
}

func TestAMissingFileIsNotAnError(t *testing.T) {
	// The publisher lives in another repository and may not have run yet. No map is the
	// behaviour this service had before the map existed, so it must stay a clean state.
	m := Load(filepath.Join(t.TempDir(), "absent.json"), time.Minute)
	if got := m.Lookup("g2i"); got != "" {
		t.Errorf("Lookup = %q, want empty", got)
	}
	if got := m.Size(); got != 0 {
		t.Errorf("Size = %d, want 0", got)
	}
}

func TestAnEmptyPathLoadsNothing(t *testing.T) {
	m := Load("", time.Minute)
	if m.Size() != 0 {
		t.Errorf("Size = %d, want 0", m.Size())
	}
	if got := m.Lookup("g2i"); got != "" {
		t.Errorf("Lookup = %q, want empty", got)
	}
}

func TestAnUnknownNormalizationIsRefused(t *testing.T) {
	// The publisher's key rule and this service's store.Normalize are two copies of one
	// rule in two repositories. If they drift, the map matches nothing while looking
	// perfectly healthy — so a value this build does not implement is refused outright.
	path := filepath.Join(t.TempDir(), "domains.json")
	write(t, path, `{"version":1,"normalization":"nfkc-strip-punct","entries":{"g2i":"g2i.co"}}`)
	m := Load(path, time.Minute)
	if got := m.Size(); got != 0 {
		t.Errorf("Size = %d, want 0 — the file should have been refused", got)
	}
}

func TestAnUnknownVersionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	write(t, path, `{"version":99,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co"}}`)
	if got := Load(path, time.Minute).Size(); got != 0 {
		t.Errorf("Size = %d, want 0", got)
	}
}

func TestUnparseableJSONIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	write(t, path, `{"version":1,`)
	if got := Load(path, time.Minute).Size(); got != 0 {
		t.Errorf("Size = %d, want 0", got)
	}
}

func TestAReloadPicksUpARewrittenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	write(t, path, `{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"old.example"}}`)
	// reloadEvery 0 makes every Lookup re-stat, which is what lets this test observe a
	// reload without sleeping.
	m := Load(path, 0)
	if got := m.Lookup("g2i"); got != "old.example" {
		t.Fatalf("Lookup = %q, want %q", got, "old.example")
	}
	// A rewrite within the same filesystem timestamp granularity would not move mtime,
	// so set it explicitly rather than relying on the clock.
	write(t, path, `{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co"}}`)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if got := m.Lookup("g2i"); got != "g2i.co" {
		t.Errorf("after reload Lookup = %q, want %q", got, "g2i.co")
	}
}

func TestARefusedReloadKeepsTheMapItAlreadyHas(t *testing.T) {
	// A publisher that writes a broken file must not blank every logo on the site. The
	// last good map is better than no map, and the log line is what says so.
	path := filepath.Join(t.TempDir(), "domains.json")
	write(t, path, `{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co"}}`)
	m := Load(path, 0)
	if got := m.Lookup("g2i"); got != "g2i.co" {
		t.Fatalf("Lookup = %q, want %q", got, "g2i.co")
	}
	write(t, path, `{"version":1,`)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if got := m.Lookup("g2i"); got != "g2i.co" {
		t.Errorf("Lookup = %q after a broken reload, want the last good value %q", got, "g2i.co")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd ~/Projects/freehire-logo && go test ./internal/domainmap/`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the implementation**

Create `internal/domainmap/domainmap.go`:

```go
// Package domainmap holds the company-name-to-domain map published by `hire`.
//
// Resolving a logo by NAME sometimes answers with a different company — the reason
// resolve.Query carries a Domain at all. This service cannot derive a domain: it has no
// company database, and a guessed one is worse than none. What it can do is read a map
// somebody who does have that database published, which is what this is.
//
// Everything here degrades to "no map": an absent file, an unreadable one, a format this
// build does not implement. Without a map the service behaves exactly as it did before
// the map existed, which is what makes deleting the file a complete rollback.
package domainmap

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

// version and normalizationID are the contract this build implements. A file declaring
// anything else is refused rather than guessed at: the publisher's key rule and this
// service's store.Normalize are two copies of one rule in two repositories, and a silent
// drift between them produces a map that matches nothing while looking healthy.
const (
	version         = 1
	normalizationID = "lower-collapse-ws"
)

type snapshot struct {
	Version       int               `json:"version"`
	Normalization string            `json:"normalization"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Entries       map[string]string `json:"entries"`
}

// Map answers a normalized company name with a bare registrable domain, reloading the
// file behind it when its mtime moves.
type Map struct {
	path        string
	reloadEvery time.Duration

	mu        sync.RWMutex
	entries   map[string]string
	mtime     time.Time
	lastCheck time.Time
}

// Load opens the map at path and returns it ready to serve. It never fails: an
// unusable file is logged and the map serves nothing, which is this service's behaviour
// without a map at all. An empty path means no map was configured.
//
// reloadEvery bounds how often a Lookup may stat the file. The publisher rewrites it once
// a day, so a few minutes is plenty; zero stats on every lookup and exists for tests.
func Load(path string, reloadEvery time.Duration) *Map {
	m := &Map{path: path, reloadEvery: reloadEvery, entries: map[string]string{}}
	if path == "" {
		return m
	}
	m.refresh()
	return m
}

// Lookup returns the domain for a name ALREADY normalized by store.Normalize, or "" when
// the map has nothing for it. The caller normalizes because it has already done so to
// build its cache key, and normalizing twice here would invite the two to diverge.
func (m *Map) Lookup(normalizedName string) string {
	if m.path == "" {
		return ""
	}
	m.maybeRefresh()
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[normalizedName]
}

// Size reports how many entries are loaded. It exists for the health endpoint and for
// tests: "the map is configured" and "the map has anything in it" are different states
// and only the second one fixes a logo.
func (m *Map) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

func (m *Map) maybeRefresh() {
	m.mu.RLock()
	due := time.Since(m.lastCheck) >= m.reloadEvery
	m.mu.RUnlock()
	if due {
		m.refresh()
	}
}

// refresh re-reads the file when its mtime has moved. A failure anywhere leaves the
// entries already loaded in place: a publisher that wrote a broken file must not blank
// every logo on the site, and the last good map is the better answer.
func (m *Map) refresh() {
	now := time.Now()
	info, err := os.Stat(m.path)
	if err != nil {
		// Below Warn on purpose for the common case: the publisher lives in another
		// repository and a host where it has not run yet is an expected state, not a
		// fault of this service.
		slog.Debug("no logo domain map", "path", m.path, "err", err)
		m.mu.Lock()
		m.lastCheck = now
		m.mu.Unlock()
		return
	}

	m.mu.RLock()
	unchanged := info.ModTime().Equal(m.mtime)
	m.mu.RUnlock()
	if unchanged {
		m.mu.Lock()
		m.lastCheck = now
		m.mu.Unlock()
		return
	}

	raw, err := os.ReadFile(m.path)
	if err != nil {
		slog.Warn("could not read the logo domain map", "path", m.path, "err", err)
		m.mu.Lock()
		m.lastCheck = now
		m.mu.Unlock()
		return
	}
	var s snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		slog.Warn("the logo domain map did not parse", "path", m.path, "err", err)
		m.mu.Lock()
		m.lastCheck = now
		m.mu.Unlock()
		return
	}
	if s.Version != version || s.Normalization != normalizationID {
		// Loud: this is the drift the contract exists to catch, and the symptom without
		// this line is a map that quietly matches nothing.
		slog.Error("refusing a logo domain map this build does not implement",
			"path", m.path,
			"version", s.Version, "want_version", version,
			"normalization", s.Normalization, "want_normalization", normalizationID)
		m.mu.Lock()
		m.lastCheck = now
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	m.entries = s.Entries
	m.mtime = info.ModTime()
	m.lastCheck = now
	m.mu.Unlock()
	slog.Info("loaded the logo domain map", "path", m.path, "entries", len(s.Entries), "generated_at", s.GeneratedAt)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd ~/Projects/freehire-logo && go test ./internal/domainmap/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
cd ~/Projects/freehire-logo
gofmt -w internal/domainmap/
go vet ./... && go test ./internal/domainmap/
git add internal/domainmap
git commit -m "Add domainmap: the name-to-domain map hire publishes

This service cannot derive a domain — it has no company database, and a
guessed one is worse than none. It can read one published by something
that does.

Everything degrades to no map: an absent file, an unreadable one, a
format this build does not implement. A refused reload keeps the last
good map rather than blanking every logo on the site."
```

---

## Task B2: Consult the map in the handler

**Files:**
- Modify: `internal/server/server.go` (the `handler` struct, `New`, and `ServeHTTP` around lines 37-90)
- Modify: `internal/server/server_test.go`
- Modify: `cmd/logo-proxy/main.go`

**Interfaces:**
- Consumes: `domainmap.Map` from Task B1.
- Produces: `func New(s *store.Store, r *resolve.Resolver, domains *domainmap.Map, pools ...*resolve.Pool) http.Handler`

- [ ] **Step 1: Write the failing test**

Append to `internal/server/server_test.go`. Read the file's existing helpers first
(`go doc` will not show them — open the file) and reuse whatever it already uses to build
a handler and a fake upstream; the tests below assume a fake upstream that records the
`resolve.Query` it was handed. If no such recorder exists, add one alongside the existing
fakes rather than inventing a second style.

```go
func TestAMappedNameReachesTheResolverWithItsDomain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	up := &recordingUpstream{data: pngPixel}
	h := New(newTestStore(t), resolve.New([]resolve.Upstream{up}), domainmap.Load(path, time.Minute))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/g2i", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if up.last.Domain != "g2i.co" {
		t.Errorf("resolver saw Domain = %q, want %q", up.last.Domain, "g2i.co")
	}
}

func TestAnExplicitDomainBeatsTheMap(t *testing.T) {
	// The map is a default, not an override: a caller that knows the company's domain
	// knows more than a snapshot taken yesterday.
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	up := &recordingUpstream{data: pngPixel}
	h := New(newTestStore(t), resolve.New([]resolve.Upstream{up}), domainmap.Load(path, time.Minute))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/g2i?domain=elsewhere.example", nil))

	if up.last.Domain != "elsewhere.example" {
		t.Errorf("resolver saw Domain = %q, want the caller's %q", up.last.Domain, "elsewhere.example")
	}
}

func TestAnUnmappedNameIsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	up := &recordingUpstream{data: pngPixel}
	h := New(newTestStore(t), resolve.New([]resolve.Upstream{up}), domainmap.Load(path, time.Minute))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/Somebody%20Else", nil))

	if up.last.Domain != "" {
		t.Errorf("resolver saw Domain = %q, want empty", up.last.Domain)
	}
}

func TestSpellingsOfOneCompanyAllReachTheSameDomain(t *testing.T) {
	// The bug this whole change exists for: three spellings, three logos, two of them
	// another company's.
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"normalization":"lower-collapse-ws","entries":{"g2i":"g2i.co","g2i inc.":"g2i.co"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/g2i", "/G2i", "/G2i%20Inc."} {
		up := &recordingUpstream{data: pngPixel}
		h := New(newTestStore(t), resolve.New([]resolve.Upstream{up}), domainmap.Load(path, time.Minute))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, name, nil))
		if up.last.Domain != "g2i.co" {
			t.Errorf("%s: resolver saw Domain = %q, want %q", name, up.last.Domain, "g2i.co")
		}
	}
}

func TestANilMapBehavesAsBefore(t *testing.T) {
	up := &recordingUpstream{data: pngPixel}
	h := New(newTestStore(t), resolve.New([]resolve.Upstream{up}), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/g2i", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if up.last.Domain != "" {
		t.Errorf("resolver saw Domain = %q, want empty", up.last.Domain)
	}
}

// recordingUpstream answers with fixed bytes and remembers the last query, which is what
// the tests above assert on: the point is WHICH domain reached the resolver, not what
// came back.
type recordingUpstream struct {
	data []byte
	last resolve.Query
}

func (u *recordingUpstream) Fetch(_ context.Context, q resolve.Query) ([]byte, error) {
	u.last = q
	return u.data, nil
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd ~/Projects/freehire-logo && go test ./internal/server/`
Expected: FAIL — `New` takes the wrong number of arguments.

- [ ] **Step 3: Change `New` and the handler**

In `internal/server/server.go`:

Add the import `"github.com/strelov1/freehire-logo/internal/domainmap"`.

Add a field to `handler`:

```go
	// domains is the map hire publishes: normalized company name to the employer's own
	// domain. It supplies what the caller could not — most of the SPA's logo call sites
	// only have a name string. May be nil, which is "no map configured".
	domains *domainmap.Map
```

Change `New`:

```go
// New builds the handler. It is the only thing that knows the three units compose in the
// order cache → resolve → render. domains may be nil, in which case a request resolves
// exactly as it did before the map existed. pools are reported by the health endpoint and
// are only read, never used to resolve anything.
func New(s *store.Store, r *resolve.Resolver, domains *domainmap.Map, pools ...*resolve.Pool) http.Handler {
	return &handler{store: s, resolver: r, domains: domains, pools: pools}
}
```

In `ServeHTTP`, immediately after the existing `?domain=` block (after the
`slog.Warn("ignoring an unusable ?domain=", ...)` branch closes), insert:

```go
	// Fall back to the published map. It is consulted only when the caller sent nothing
	// usable: a caller that knows the company's domain knows more than a snapshot taken
	// yesterday, and an unusable ?domain= has already been reported above.
	//
	// The lookup key is store.Normalize(name) — the same key the cache is hashed under,
	// and the key the publisher builds its entries with.
	if domain == "" && h.domains != nil {
		if mapped := normalizeDomain(h.domains.Lookup(store.Normalize(name))); mapped != "" {
			domain = mapped
		}
	}
```

`normalizeDomain` is applied to the map's value deliberately: the publisher already emits
a bare domain, and running it through the same funnel every other domain goes through
means one malformed entry cannot reach the upstream in a shape nothing else ever produces.

- [ ] **Step 4: Update `cmd/logo-proxy/main.go`**

Add the import `"github.com/strelov1/freehire-logo/internal/domainmap"`, and before the
`httpServer` is built:

```go
	// The map hire publishes (see ../hire, cmd/publish-logo-domains). Unset and there is
	// no map, which is exactly how this service behaved before it existed — and is the
	// rollback. Five minutes is well inside the publisher's daily cadence and costs one
	// stat per lookup at most.
	domains := domainmap.Load(os.Getenv("LOGO_DOMAIN_MAP"), 5*time.Minute)
```

and change the handler line to:

```go
		Handler:           server.New(cache, resolve.New(upstreams), domains, logoDevPool),
```

and add `"domains", domains.Size()` to the final `slog.Info("listening", ...)` call, so a
started process says whether it actually has a map rather than only that it was
configured with a path.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd ~/Projects/freehire-logo && go build ./... && go test ./...`
Expected: PASS. If `internal/server`'s existing tests fail to compile, they call `New`
with the old signature — add `nil` as the third argument to each.

Note: `go test ./...` here needs libwebp headers (`apt-get install libwebp-dev`, or
`brew install webp` on macOS). If the render package will not build, run
`go test ./internal/domainmap/ ./internal/server/` and say so in the handoff rather than
reporting a clean suite.

- [ ] **Step 6: Commit**

```bash
cd ~/Projects/freehire-logo
gofmt -w internal/server cmd/logo-proxy internal/domainmap
git add internal/server cmd/logo-proxy
git commit -m "Consult the published domain map when the caller sent none

Three spellings of g2i asked for three logos and two of them belonged to
an unrelated leadership-training firm, at HTTP 200. The caller could not
fix it: most of the SPA's ~25 logo call sites only ever have a name.

An explicit ?domain= still wins — a caller that knows the domain knows
more than yesterday's snapshot. Unset LOGO_DOMAIN_MAP and nothing here
changes, which is the rollback."
```

---

## Task B3: Document the map

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add a section**

After the "Status" section in `README.md`:

```markdown
## The domain map

Resolving a logo by name sometimes answers with a different company — `g2i` returns an
unrelated leadership-training firm's mark, at HTTP 200, which no caller can detect.
`?domain=` removes the ambiguity, and `../hire` now publishes the domains it knows:

```
LOGO_DOMAIN_MAP=/var/lib/freehire-logo-map/domains.json
```

The file is written daily by `../hire`'s `cmd/publish-logo-domains` and holds normalized
company names against bare domains. `internal/domainmap` reloads it when its mtime moves.

Everything about it degrades to "no map": unset the variable, delete the file, or write
one declaring a `version`/`normalization` this build does not implement, and the service
resolves exactly as it did before the map existed. That is the rollback.

An explicit `?domain=` from a caller always wins over the map.
```

- [ ] **Step 2: Commit**

```bash
cd ~/Projects/freehire-logo
git add README.md
git commit -m "Document the domain map and its rollback"
```

---

# Part C — Verify against production

## Task C1: Prove the fix on real data

**Files:** none — this task changes nothing and produces evidence.

- [ ] **Step 1: Record the current wrong answer**

```bash
curl -s -o /dev/null -w "%{http_code} %{size_download}\n" https://logo.freehire.me/g2i
```
Expected today: `200 1814` — the wrong company.

- [ ] **Step 2: Prove the correct answer is reachable**

```bash
curl -s -o /dev/null -w "%{http_code} %{size_download}\n" "https://logo.freehire.me/g2i?domain=g2i.co"
```
Expected: `200 2984` — the correct mark. This is the target state for step 5.

- [ ] **Step 3: Run the publisher once on the host, by hand**

Per `hire`'s memory of running prod workers by hand, use `systemd-run` rather than
`env $(cat .env)`, which exposes secrets in the process table:

```bash
ssh root@89.167.94.146 'systemd-run --unit=publish-logo-domains-manual --property=Type=oneshot \
  --property=User=freehire --property=WorkingDirectory=/opt/freehire \
  --property=EnvironmentFile=/opt/freehire/.env \
  --property=StateDirectory=freehire-logo-map \
  --setenv=LOGO_DOMAIN_MAP_OUT=/var/lib/freehire-logo-map/domains.json \
  /opt/freehire/bin/publish-logo-domains'
ssh root@89.167.94.146 'journalctl -u publish-logo-domains-manual -n 20 --no-pager'
```

Expected: a line reporting the entry count, and a file at
`/var/lib/freehire-logo-map/domains.json`. Confirm g2i is in it:

```bash
ssh root@89.167.94.146 "python3 -c \"import json;d=json.load(open('/var/lib/freehire-logo-map/domains.json'));print(d['version'],d['normalization'],len(d['entries']));print({k:v for k,v in d['entries'].items() if 'g2i' in k})\""
```

- [ ] **Step 4: Deploy the proxy and point it at the map**

The proxy is not carried by `release.sh`. Build it, copy it, set the variable, restart:

```bash
cd ~/Projects/freehire-logo && GOOS=linux GOARCH=amd64 go build -o /tmp/logo-proxy ./cmd/logo-proxy
scp /tmp/logo-proxy root@89.167.94.146:/opt/freehire/bin/logo-proxy.new
ssh root@89.167.94.146 'mv /opt/freehire/bin/logo-proxy.new /opt/freehire/bin/logo-proxy && \
  grep -q LOGO_DOMAIN_MAP /opt/freehire/.env || echo "LOGO_DOMAIN_MAP=/var/lib/freehire-logo-map/domains.json" >> /opt/freehire/.env && \
  systemctl restart freehire-logo && sleep 2 && journalctl -u freehire-logo -n 10 --no-pager'
```

Expected in the log: `loaded the logo domain map` with a non-zero `entries`, and
`listening` reporting a non-zero `domains`.

Note: the proxy runs as `freehire` and the map is created by a `StateDirectory` the
worker owns. If the log says the file could not be read, check the directory mode
(`ls -ld /var/lib/freehire-logo-map`) — it must be readable by the proxy's user.

- [ ] **Step 5: Confirm the fix end to end**

```bash
for u in g2i G2i "G2i%20Inc."; do
  echo -n "$u -> "
  curl -s -o /dev/null -w "%{http_code} %{size_download}\n" "https://logo.freehire.me/$u"
done
```

Expected: all three answer `200 2984`. Before this change they answered `200 1814`,
`200 1814`, `200 2984`.

Then open `https://freehire.me/jobs?company_slug=g2i&cb=$RANDOM` in a browser — the cache
buster matters, `max-age=3600` will otherwise serve the old page for an hour — and confirm
one logo across all rows.

- [ ] **Step 6: Enable the timer**

```bash
ssh root@89.167.94.146 'systemctl daemon-reload && systemctl enable --now freehire-publish-logo-domains.timer && systemctl list-timers freehire-publish-logo-domains --no-pager'
```

- [ ] **Step 7: Check for drift between the repo and the host**

```bash
cd /tmp/wt-logo-domain && ./deploy/check-drift.sh
```
Expected: the new unit and timer match. Anything else it reports was already there.

---

## Self-review notes

**Spec coverage**

| Spec section | Task |
|---|---|
| Publisher worker, `LOGO_DOMAIN_MAP_OUT`, empty-map refusal | A4 |
| `internal/job/logodomain`: normalization, domain extraction, collisions | A1 |
| Snapshot format, atomic write, `version`/`normalization` declaration | A3 |
| The full-scan query and its measurement | A2 |
| Daily unit and timer | A5 |
| `release.sh` build list | A5 |
| `internal/domainmap`: load, validate, reload | B1 |
| Handler branch, explicit `?domain=` wins | B2 |
| Rollback documented | B3 |
| Cache behaviour (no busting needed) | verified in C1 step 5 — the three spellings answering 2984 is the proof |
| Deployment order (publisher first) | C1 steps 3-4 |

**Known gap accepted deliberately:** the plan adds no integration test over a seeded
database for the worker itself. The two queries are plain reads with no branching, and
every decision the worker makes lives in `logodomain`, which is unit-tested. C1 is the
end-to-end check.

**Out of scope, tracked separately:** the `is_tech`/category gate hiding 21,636 open ashby
postings from search, found in the same investigation.
