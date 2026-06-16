// Package sources holds the modular job-source adapters and the registry that maps
// a platform key to its adapter. Each adapter implements one ATS platform; adding a
// platform is a new file plus one line in All.
package sources

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

// CompanyEntry is one configured board from a board file (sources/<provider>.yml): the
// company whose jobs we crawl, the platform it uses (Provider), and the platform-specific
// board id.
type CompanyEntry struct {
	Company  string `yaml:"company"`
	Provider string `yaml:"provider"`
	Board    string `yaml:"board"`
}

// Job is a raw posting as an adapter yields it, before the pipeline normalizes it
// into the catalogue. ExternalID carries the platform's native posting id; the
// pipeline namespaces it by board before persisting.
type Job struct {
	ExternalID  string
	URL         string
	Title       string
	Company     string
	Location    string
	Description string
	Remote      bool
	PostedAt    *time.Time
	// WorkMode is the work arrangement when the platform states it in a STRUCTURED
	// field (a workplace-type enum or an explicit remote flag) — "remote",
	// "hybrid", or "onsite", else "". It is left empty for adapters that only
	// expose free-text location; the pipeline then falls back to parsing the
	// location string. Provenance stays clean: this carries structured signal only,
	// never the location heuristic.
	WorkMode string
}

// Source adapts one job-source platform. Provider is the platform key that selects
// the adapter (it matches CompanyEntry.Provider and the stored jobs.source); Fetch
// returns all current postings for one configured board.
type Source interface {
	Provider() string
	Fetch(ctx context.Context, e CompanyEntry) ([]Job, error)
}

// boardless marks an adapter whose source is one company's own API, with no per-tenant
// board id. Config validation lets a boardless provider's entries omit board. Multi-tenant
// ATS adapters (greenhouse, lever, …) do not implement it and still require a board.
type boardless interface{ boardless() }

// FilterableProviders returns the sorted provider keys of the multi-tenant ATS
// adapters — those NOT implementing the boardless marker. The source facet filters
// by platform, and a single-company (boardless) platform is redundant with the
// company filter, so it is excluded. Passing a nil client is safe: Provider() and
// the boardless assertion never touch the transport.
func FilterableProviders() []string {
	var out []string
	for key, src := range All(nil) {
		if _, isBoardless := src.(boardless); isBoardless {
			continue
		}
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

// All assembles the registered adapters into a provider-keyed registry, sharing one
// HTTP client across them. Adding a platform is a new adapter plus one line here.
func All(c HTTPClient) map[string]Source {
	return reg(
		NewGreenhouse(c),
		NewLever(c),
		NewAshby(c),
		NewWorkable(c),
		NewRecruitee(c),
		NewSmartRecruiters(c),
		NewGupy(c),
		NewPersonio(c),
		NewPinpoint(c),
		NewRippling(c),
		NewBambooHR(c),
		NewWorkday(c),
		NewHuntflow(c),
		NewGem(c),
		NewSuccessFactors(c),
		NewTeamtailor(c),
		NewBreezy(c),
		NewJoin(c),
		NewGlobalPayments(c),
		// International single-company adapters (boardless).
		NewUber(c),
		NewAmazon(c),
		// RU-domestic single-company adapters (boardless, except Yandex which selects
		// host+language by board).
		NewYandex(c),
		NewOzon(c),
		NewRWB(c),
		NewSber(c),
		NewAlfaBank(c),
		NewLamoda(c),
		NewKuper(c),
		NewAviasales(c),
		NewDodo(c),
		NewDomclick(c),
		NewMtslink(c),
		NewTBank(c),
		NewMTS(c),
		NewVK(c),
	)
}

// defaultDetailWorkers bounds the per-board detail-fetch fan-out for adapters whose
// list endpoint omits the description. All detail adapters share this default; an
// adapter needing a different bound reintroduces its own const at that call site.
const defaultDetailWorkers = 8

// fetchDetails maps each posting to a Job via fetch, running fetch concurrently with a
// bounded worker pool of the given size. A posting whose fetch returns ok=false is
// dropped, so one failed detail request never aborts the board. The surviving jobs keep
// their postings' relative order. Adapters whose list endpoint omits the description
// (SmartRecruiters, Rippling, BambooHR) share this so the bound and isolation behave
// identically across platforms.
func fetchDetails[P any](postings []P, workers int, fetch func(P) (Job, bool)) []Job {
	jobs := make([]*Job, len(postings))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, p := range postings {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p P) {
			defer wg.Done()
			defer func() { <-sem }()
			if j, ok := fetch(p); ok {
				jobs[i] = &j
			}
		}(i, p)
	}
	wg.Wait()

	out := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		if j != nil { // nil = detail fetch failed, skipped
			out = append(out, *j)
		}
	}
	return out
}

// isRemote infers a job's remote flag from its location text. Adapters share it so
// the heuristic stays consistent across platforms. It matches the English "remote" and
// the Russian "удал…" (удалённо/удалённая/удалёнка) so RU-segment boards flag correctly.
func isRemote(location string) bool {
	l := strings.ToLower(location)
	return strings.Contains(l, "remote") || strings.Contains(l, "удал")
}

// workModeFromRemote maps an adapter's STRUCTURED remote flag to a work mode:
// "remote" when set, else "" (a false flag does not imply onsite vs hybrid, so it
// is left unknown for the parser/LLM to resolve). Adapters whose API exposes an
// explicit remote boolean (Ashby, Recruitee, SmartRecruiters, Workable) use this.
func workModeFromRemote(remote bool) string {
	if remote {
		return "remote"
	}
	return ""
}

// workplaceTypeMode maps an ATS workplace-type enum (as Lever exposes) to our work
// mode vocabulary; an unspecified/unknown value yields "".
func workplaceTypeMode(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "on-site", "onsite", "on site":
		return "onsite"
	default:
		return ""
	}
}

// maxPostedAtSkew is how far past "now" a posted_at may sit before NotFuture treats it as
// bogus rather than clock skew or a timezone-ahead date-only value (a UTC+14 "today" parses
// to a UTC instant a few hours ahead). Comfortably larger than any real skew, far smaller
// than the month-scale errors a misread deadline/validThrough or wrong year produces.
const maxPostedAtSkew = 48 * time.Hour

// NotFuture drops a posted_at that lies meaningfully in the future: a posting cannot be
// published after now, so such a value is a source or parse artifact (a deadline misread as
// the publish date, a wrong year) that would otherwise sort the job to the very top of the
// "freshest first" browse. It returns nil — the job reads as undated rather than freshest —
// instead of clamping to now, so a bogus date can't masquerade as fresh. A nil or in-range
// time passes through unchanged. Every adapter's date parser funnels through it.
func NotFuture(t *time.Time) *time.Time {
	if t != nil && t.After(time.Now().Add(maxPostedAtSkew)) {
		return nil
	}
	return t
}

// parseLayout parses a platform timestamp with the given layout into a posted_at,
// returning nil on an empty or unparseable value (posted_at is nullable — a missing or
// malformed date is not an error).
func parseLayout(layout, s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}

// parseRFC3339 parses an RFC3339 timestamp (the common ATS format).
func parseRFC3339(s string) *time.Time { return parseLayout(time.RFC3339, s) }

// parseDate parses a date-only timestamp ("2006-01-02", as Workable emits).
func parseDate(s string) *time.Time { return parseLayout("2006-01-02", s) }

// parseSpaceTime parses a space-separated, zone-named timestamp ("2006-01-02 15:04:05
// MST", as Recruitee emits). Recruitee emits UTC; an unrecognized zone abbreviation
// would be read as offset 0, acceptable for an approximate posted_at.
func parseSpaceTime(s string) *time.Time { return parseLayout("2006-01-02 15:04:05 MST", s) }

// joinNonEmpty joins the non-empty parts with ", ", so a location built from
// separate city/state/country fields skips blanks.
func joinNonEmpty(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}

// firstNonEmpty returns the first part that is not the empty string, or "" when every
// part is empty. Adapters use it for the common "this value, else fall back to that one"
// choice (e.g. a posting's own employer name, else the configured company). The check is
// exact-empty (not whitespace-trimmed), so it is a drop-in for the inline
// `if x == "" { x = fallback }` idiom it replaces; unlike joinNonEmpty it does not treat a
// whitespace-only value as blank.
func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if p != "" {
			return p
		}
	}
	return ""
}

// parseEpochMillis converts a Unix-millisecond timestamp into a posted_at, returning
// nil for a zero value (treated as "no date").
func parseEpochMillis(ms int64) *time.Time {
	if ms == 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return NotFuture(&t)
}

// parseEpochSeconds converts a Unix-second timestamp into a posted_at, returning nil for
// a zero value (treated as "no date"). Gem dates postings with firstPublishedTsSec.
func parseEpochSeconds(s int64) *time.Time {
	if s == 0 {
		return nil
	}
	t := time.Unix(s, 0).UTC()
	return NotFuture(&t)
}

// reg indexes sources by provider key. A duplicate key means two adapters claim the
// same platform — a programming error — so it panics rather than silently dropping one.
func reg(sources ...Source) map[string]Source {
	m := make(map[string]Source, len(sources))
	for _, s := range sources {
		if _, dup := m[s.Provider()]; dup {
			panic("sources: duplicate provider " + s.Provider())
		}
		m[s.Provider()] = s
	}
	return m
}
