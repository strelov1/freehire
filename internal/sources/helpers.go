package sources

import (
	"regexp"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/location"
)

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

// fetchDetailsStream maps each posting to a Job via fetch, concurrently with a bounded worker
// pool, emitting each successful Job to emit as soon as its detail completes (a posting whose
// fetch returns ok=false is dropped). Unlike fetchDetails it does not buffer or preserve order,
// so a streaming adapter persists postings incrementally. emit is called from worker goroutines
// — the caller must make it concurrency-safe. It blocks until every posting has been attempted.
func fetchDetailsStream[P any](postings []P, workers int, fetch func(P) (Job, bool), emit func(Job)) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, p := range postings {
		wg.Add(1)
		sem <- struct{}{}
		go func(p P) {
			defer wg.Done()
			defer func() { <-sem }()
			if j, ok := fetch(p); ok {
				emit(j)
			}
		}(p)
	}
	wg.Wait()
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
// is left unknown for the parser/LLM to resolve). Adapters whose API exposes a remote
// boolean and nothing better (Workable, Breezy) use this. Reach for it only after
// checking that the API carries no richer field: an ATS that also reports hybrid needs
// workModeFromRemoteHybrid, and one with a workplace-type enum needs workplaceTypeMode —
// a lone boolean often means "not onsite", which is not the same as "remote".
func workModeFromRemote(remote bool) string {
	if remote {
		return "remote"
	}
	return ""
}

// workModeFromRemoteHybrid maps the PAIR of booleans an ATS exposes when it tracks remote
// and hybrid separately (Recruitee, SmartRecruiters) to a work mode. Reading the remote one
// alone silently drops every hybrid posting, which is most of them on some boards.
//
// Remote wins when a posting sets both — around 2% of Recruitee offers do, and Recruitee
// itself renders each of them as "Remote job", so the broader arrangement is what the
// employer means. Both false stays "" rather than "onsite": an ATS cannot distinguish
// "marked as office" from "not marked at all", and the dictionary contract forbids the guess.
func workModeFromRemoteHybrid(remote, hybrid bool) string {
	switch {
	case remote:
		return "remote"
	case hybrid:
		return "hybrid"
	default:
		return ""
	}
}

// workplaceTypeMode maps an ATS workplace-type enum (as Lever and JustJoin expose) to our
// work mode vocabulary; an unspecified/unknown value yields "".
func workplaceTypeMode(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "on-site", "onsite", "on site", "office":
		return "onsite"
	default:
		return ""
	}
}

// countryFromCode normalizes an ATS-supplied country code (Lever/Workday alpha-2, Ashby
// alpha-3) into the []string Job.Countries expects, via location.NormalizeCountry.
// Returns nil for an empty or unresolved code — never a one-element slice holding "" —
// so an adapter can wire it straight into Job.Countries without an extra empty check.
func countryFromCode(code string) []string {
	c := location.NormalizeCountry(code)
	if c == "" {
		return nil
	}
	return []string{c}
}

// countriesFromCodes is the multi-place counterpart of countryFromCode: it normalizes a
// posting's whole set of ATS-supplied country codes, dropping unresolved and duplicate ones and
// keeping first-seen order. A board that states several locations for one posting states several
// countries (landing.jobs lists a single role across Munich, Lisbon and Cologne), and keeping
// only the first would hide that posting from a filter on any of the others. Returns nil when
// nothing resolves, so an adapter can wire it straight into Job.Countries.
func countriesFromCodes(codes []string) []string {
	var out []string
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		c := location.NormalizeCountry(code)
		if c == "" {
			continue
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

// distinctJoin maps each item to a label, drops blank and duplicate labels (keeping first-seen
// order), and joins the rest with sep. Adapters that build a location from a list of place
// objects share it (getmatch, habrcareer) instead of each re-looping.
func distinctJoin[T any](items []T, sep string, label func(T) string) string {
	var kept []string
	seen := map[string]struct{}{}
	for _, it := range items {
		l := strings.TrimSpace(label(it))
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		kept = append(kept, l)
	}
	return strings.Join(kept, sep)
}

// trimURLSuffix drops any query string or fragment from a URL, leaving just the path. Adapters
// that extract an id from the end of a URL path use it so a tracking suffix (?utm=…) or a #anchor
// does not defeat an end-anchored id pattern.
func trimURLSuffix(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

// firstSubmatch returns the first capture group of pattern in s, or "". Adapters that pull a
// native posting id out of a URL with a single-group regex funnel through it, so the "match,
// else empty" idiom is written once.
func firstSubmatch(pattern *regexp.Regexp, s string) string {
	if m := pattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

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
