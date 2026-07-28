package sources

import (
	"regexp"
	"strings"
	"sync"
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
// is left unknown for the parser/LLM to resolve). Adapters whose API exposes an
// explicit remote boolean (Ashby, Recruitee, SmartRecruiters, Workable) use this.
func workModeFromRemote(remote bool) string {
	if remote {
		return "remote"
	}
	return ""
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
