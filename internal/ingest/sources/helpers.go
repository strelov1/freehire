package sources

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
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
//
// An adapter whose detail request is a posting's ONLY source must not spend that drop on a
// request it merely could not READ: it returns unreadableDetail(...) with ok=true instead, so
// the pipeline can tell a posting that is gone from one this crawl failed to see.
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

// detailUnreadable reports whether a failed detail request left the crawl WITHOUT information
// about the posting, rather than telling it something. A 404 or a 410 is the platform's own
// answer that the posting no longer exists — evidence, on which not seeing it again is the
// right reading. Anything else (a timeout, a refusal, a 5xx, an undecodable body) says only
// that this request failed, and a crawl that files it as an absence closes a live posting once
// the sweep's grace window elapses.
//
// It classifies what SURVIVED the shared client's own retries — 429, 5xx and network errors
// are retried before an error ever reaches an adapter — and matches the status structurally
// through *StatusError, the same way isRateLimited does rather than scraping the message.
func detailUnreadable(err error) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return true // a transport or decode failure states nothing about the posting
	}
	return se.Code != http.StatusNotFound && se.Code != http.StatusGone
}

// unreadableDetail is the marker a link-only adapter yields in place of a posting whose detail
// it could not read (see Job.Unreadable). It carries the identity the adapter holds without the
// detail; company is what lets the pipeline name the employer whose close scope this run may
// not license, and is the board's configured employer when nothing better is known.
//
// Two cases deliberately stay a plain drop rather than becoming a marker. A link carrying no
// posting id at all is not a posting this catalogue could ever have stored, so no close can
// reach it. And a page the platform answers 404/410 for is gone, which is the whole point of
// the distinction. Everything in between — including a 200 that carries no posting — is a
// marker: the one cause that would explain an empty page across a WHOLE board is the platform
// changing its markup, and reading that as "every posting is gone" is the mass-close this
// exists to prevent.
func unreadableDetail(externalID, url, company string) Job {
	return Job{Unreadable: true, ExternalID: externalID, URL: url, Company: company}
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

// retryDelayCap bounds the exponential backoff retryWhile applies. Past half a minute the
// wait is no longer riding out a burst, it is holding a board's whole crawl open.
const retryDelayCap = 30 * time.Second

// retryWhile runs call, retrying while retry says the error is worth another attempt and
// giving up after maxAttempts retries (so at most maxAttempts+1 calls). The wait starts at
// base and doubles up to retryDelayCap; a cancelled context ends the wait and the walk.
//
// The predicate is the caller's, not this function's: what a status code MEANS is the
// adapter's judgement (http.go's shared client deliberately leaves status branching to
// them), and the two adapters that share this loop happen to have reached the same one.
// The attempt count and the base delay stay at each adapter's own call site, where its own
// measurement of the platform's cap is written down.
func retryWhile(ctx context.Context, maxAttempts int, base time.Duration, retry func(error) bool, call func() error) error {
	delay := base
	var err error
	for attempt := 0; attempt <= maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			if delay < retryDelayCap {
				delay *= 2
			}
		}
		if err = call(); err == nil || !retry(err) {
			return err
		}
	}
	return err
}

// isRateLimited reports whether err is a rate-limit response (HTTP 403 or 429). The shared
// client surfaces a non-2xx response as a typed *StatusError, so the status code is matched
// structurally (errors.As) rather than scraped from the message.
//
// 403 is in here because both platforms that need it return 403 for a burst rather than
// 429: Eightfold caps an IP at ~290 requests per window, and some Workday tenants answer a
// burst the same way. The shared client already retries 429/5xx/network on its own; this is
// what the two adapters add on top.
func isRateLimited(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == 403 || se.Code == 429)
}
