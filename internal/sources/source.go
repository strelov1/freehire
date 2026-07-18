// Package sources holds the modular job-source adapters and the registry that maps
// a platform key to its adapter. Each adapter implements one ATS platform; adding a
// platform is a new file plus one line in All.
package sources

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// CompanyEntry is one configured board from a board file (sources/<provider>.yml): the
// company whose jobs we crawl, the platform it uses (Provider), and the platform-specific
// board id. Region is an optional per-entry hint for ATS platforms that host tenants on
// regional API domains (e.g. Lever's EU data-residency host); empty means the default host.
// Hub is an optional per-entry flag marking a board as a community/agency hub whose vacancies
// belong to many partner companies; an adapter that honours it resolves each job's employer from
// the posting and uses Company only as the hub name and per-vacancy fallback (e.g. huntflow's
// AlumniHub). It is ignored by adapters that do not implement hub resolution.
type CompanyEntry struct {
	Company  string `yaml:"company"`
	Provider string `yaml:"provider"`
	Board    string `yaml:"board"`
	Region   string `yaml:"region"`
	Hub      bool   `yaml:"hub"`
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
	// Seniority, Category, EmploymentType, Skills, and ExperienceYearsMin are the
	// platform's STRUCTURED facet signals, already mapped into freehire's controlled
	// vocabularies (enrich.SeniorityValues / enrich.CategoryValues /
	// enrich.EmploymentTypeValues / canonical skill names). They mirror WorkMode: an
	// adapter sets them only when the platform states the value in a structured field
	// (e.g. an ATS timeType / typeOfEmployment enum), never a heuristic inferred from
	// free text, and leaves them empty/nil otherwise so the pipeline's dictionaries
	// decide. The pipeline gives a set value precedence over the dictionary (Skills are unioned).
	Seniority          string
	Category           string
	EmploymentType     string
	Skills             []string
	ExperienceYearsMin *int
	// Removed marks a posting the source reports as taken down (e.g. an item flagged
	// removed in JobStream's incremental feed). A streaming, self-closing source emits
	// these so the pipeline closes the job by identity instead of upserting it; all other
	// adapters leave it false and only ever emit live postings.
	Removed bool
	// SeenRefresh marks a posting a HydratingSource re-listed but did NOT fetch fresh
	// content for (it was already ingested, so detail is skipped). The pipeline refreshes
	// the row's liveness (last_seen_at, reopen) by identity WITHOUT rewriting its content,
	// so the description and facets hydrated when it was new are preserved — a content-less
	// re-upsert would re-derive the facets from an empty description and wipe them. Only a
	// HydratingSource sets it (carrying just Title/Company/URL/ExternalID for the identity);
	// all other adapters leave it false. Mutually exclusive with Removed.
	SeenRefresh bool
}

// Source adapts one job-source platform. Provider is the platform key that selects
// the adapter (it matches CompanyEntry.Provider and the stored jobs.source); Fetch
// returns all current postings for one configured board.
type Source interface {
	Provider() string
	Fetch(ctx context.Context, e CompanyEntry) ([]Job, error)
}

// StreamingSource is a Source that can also stream its postings to a sink as it crawls, so the
// pipeline persists them incrementally rather than buffering the whole board until Fetch
// returns. An adapter with an expensive per-posting detail fan-out (eightfold, whose large
// catalogues take many minutes under the source's rate limit) implements it so a long crawl's
// progress is saved as it goes — partial work survives an interrupted or rate-limited run, and
// the catalogue converges across runs. emit is called once per ready posting and may be called
// concurrently. FetchStream returns an error only for a board-level failure (e.g. the listing
// failed); a single dropped posting is simply not emitted.
type StreamingSource interface {
	Source
	FetchStream(ctx context.Context, e CompanyEntry, emit func(Job)) error
}

// HydratingSource is a Source that fetches expensive per-posting detail (e.g. the description
// the list omits) only for postings the catalogue does not already have. The pipeline supplies a
// seen predicate — seen(externalID) reports whether that posting is already ingested for the
// provider — so a large aggregator (justjoin, ~20k live offers) issues detail requests only for
// new postings instead of on every crawl. An adapter opts in by implementing this in addition to
// Fetch (the list-only fallback used when the pipeline cannot supply a seen set); every other
// adapter is unaffected. The pipeline prefers FetchNew when the adapter implements it.
type HydratingSource interface {
	Source
	FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error)
}

// boardless marks an adapter whose API has no per-tenant board id, so config
// validation lets its entries omit board. A boardless adapter may serve one company
// (greenhouse/lever and the other multi-tenant ATS adapters are NOT boardless and
// still require a board) or aggregate many (see aggregator).
type boardless interface{ boardless() }

// aggregator marks a boardless adapter that aggregates postings from many companies
// (e.g. jobstash) rather than serving a single company. It keeps such an adapter in
// the source facet: a single-company boardless platform is redundant with the company
// filter and excluded, but filtering by an aggregator is not.
type aggregator interface{ aggregator() }

// selfClosing marks an adapter that closes its own removed postings (via a Job with
// Removed set, emitted from its stream) and therefore must be excluded from the post-run
// unseen sweep. Such a source re-reports only changed postings each run, so the sweep's
// last_seen_at cutoff would wrongly close every still-open posting it did not touch; the
// stream's removal events are the authoritative close signal instead. See SelfClosingProviders.
type selfClosing interface{ selfClosing() }

// SelfClosingProviders returns the provider names in reg that manage their own closes and
// must be skipped by the post-run unseen sweep (see selfClosing). cmd/ingest consults this
// when deciding which providers to sweep.
func SelfClosingProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(selfClosing); ok {
			out = append(out, name)
		}
	}
	return out
}

// AggregatorProviders returns the sorted provider names in reg that aggregate postings
// from many companies (see aggregator). The cross-source dedup pass uses this to tell an
// aggregator copy (which may be suppressed) from a first-party ATS posting (which wins).
func AggregatorProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(aggregator); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// FilterableProviders returns the sorted provider keys the source facet offers.
// Passing a nil client is safe: Provider() and the marker assertions never touch the
// transport.
func FilterableProviders() []string { return filterableProviders(All(nil)) }

// filterableProviders selects the source-facet provider keys from a registry. A
// single-company boardless platform is redundant with the company filter and excluded;
// a board-based platform or a multi-company aggregator stays listed.
func filterableProviders(registry map[string]Source) []string {
	var out []string
	for key, src := range registry {
		if _, isBoardless := src.(boardless); isBoardless {
			if _, isAggregator := src.(aggregator); !isAggregator {
				continue
			}
		}
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

// All assembles the registered adapters into a provider-keyed registry, sharing one
// HTTP client across them. Adding a platform is a new adapter plus one line here.
func All(c HTTPClient) map[string]Source {
	registry := reg(
		NewGreenhouse(c),
		NewLever(c),
		NewAshby(c),
		NewWorkable(c),
		NewWorkableMarketplace(c),
		NewRecruitee(c),
		NewSmartRecruiters(c),
		NewGupy(c),
		NewSolides(c),
		NewPersonio(c),
		NewPeopleForce(c),
		NewCatsone(c),
		NewOdoo(c),
		NewTalentLyft(c),
		NewPinpoint(c),
		NewRippling(c),
		NewBambooHR(c),
		NewWorkday(c),
		NewHuntflow(c),
		NewInhire(c),
		NewGem(c),
		NewSuccessFactors(c),
		NewTeamtailor(c),
		NewHurma(c),
		NewICIMS(c),
		// careerspage is rate-paced (pacedCareerPageGetter); the proxied path paces it too.
		NewCareerPage(pacedCareerPageGetter(c)),
		NewCleverstaff(c),
		NewNorthstone(c),
		NewBriefHQ(c),
		NewDjinni(c),
		NewTalentAdore(c),
		NewLoxo(c),
		NewHireology(c),
		NewIsolvedHire(c),
		NewApplicantPro(c),
		NewApploi(c),
		NewPaylocity(c),
		NewJibe(c),
		NewPhenom(c),
		NewAvature(c),
		NewComeet(c),
		NewCornerstone(c),
		NewRadancy(c),
		NewJazzHR(c),
		NewWPYoast(c),
		NewBreezy(c),
		NewJoin(c),
		NewGlobalPayments(c),
		NewRapyd(c),
		NewCareerPlug(c),
		NewPaycom(c),
		NewLuxoft(c),
		NewEPAM(c),
		NewADP(c),
		NewITechArt(c),
		NewVention(c),
		NewClinch(c),
		NewOracle(c),
		NewEightfold(c),
		NewFreshteam(c),
		NewEarcu(c),
		NewPageUp(c),
		NewNeogov(c),
		NewDeel(c),
		NewVouch(c),
		NewRecruitingSolutions(c),
		NewUKG(c),
		NewSenior(c),
		NewTrakstar(c),
		NewFactorial(c),
		NewZoho(c),
		NewTraffit(c),
		NewErecruiter(c),
		NewQuickin(c),
		NewSpark(c),
		NewMindsight(c),
		NewEnlizt(c),
		NewJobvite(c),
		// Ashby boards whose public Posting API is disabled, served via the embed GraphQL.
		NewAshbyGraphQL(c),
		// Multi-company aggregators (boardless): one global feed, company per posting.
		NewTecla(c),
		NewTeamex(c),
		NewTopco(c),
		NewGetmatch(c),
		NewGetmanfred(c),
		NewHabrCareer(c),
		NewGeekjob(c),
		NewWorkAtAStartup(c),
		NewJobStash(c),
		NewArbeitnow(c),
		NewRemoteOK(c),
		NewJobicy(c),
		NewWeWorkRemotely(c),
		NewJobspresso(c),
		NewTheHub(c),
		NewGetonbrd(c),
		NewVagas(c),
		NewWantedKR(c),
		NewMyCareersFuture(c),
		NewWorkingNomads(c),
		NewHimalayas(c),
		NewRemotive(c),
		NewJustJoin(c),
		NewNoFluffJobs(c),
		NewWantapply(c),
		NewInfoJobs(c),
		NewJobtech(c),
		NewJobnet(c),
		NewJobdanmark(c),
		NewTyomarkkinatori(c),
		NewLikeit(c),
		NewArbeitsagentur(c),
		// International single-company adapters (boardless).
		NewTelegramCareers(c),
		NewUber(c),
		NewAmazon(c),
		NewGoogle(c),
		NewApple(c),
		NewLumenalta(c),
		NewDataArt(c),
		NewOnstrider(c),
		NewAlignerr(c),
		NewMicro1(c),
		NewBairesDev(c),
		// RU federal open-data aggregator: board-based, sharded per region (board = OKATO code).
		NewTrudvsem(c),
		// RU-domestic single-company adapters (boardless, except Yandex which selects
		// host+language by board).
		NewYandex(c),
		NewOzon(c),
		NewAvito(c),
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
		NewTwoGIS(c),
	)
	// USAJobs and Reed are the keyed sources: register each only when its API key is
	// configured, so unconfigured environments (tests, local dev) leave it absent rather than
	// listing a provider that cannot crawl. The keys are secrets, read from the environment.
	if key := os.Getenv("USAJOBS_API_KEY"); key != "" {
		registry["usajobs"] = NewUSAJobs(c, key)
	}
	if key := os.Getenv("REED_API_KEY"); key != "" {
		registry["reed"] = NewReed(c, key)
	}
	// taleo needs a cookie-persisting client (its searchjobs POST requires the session cookie
	// a careersection GET sets), so it cannot use the shared jar-less client. Build a dedicated
	// one for a real crawl; on the transport-free listing path (c == nil) register a bare adapter
	// — Provider()/marker assertions never touch the transport.
	if c == nil {
		registry["taleo"] = NewTaleo(nil)
	} else {
		registry["taleo"] = NewTaleo(newCookieClient())
	}
	// meta is NOT served by the shared client: Meta's edge 400s the default Go TLS+HTTP/2
	// fingerprint, so it needs the shared Chrome-fingerprint transport (fingerprintHTTP, also used
	// by the bayt/gulftalent aggregators). Build it only when there is a real client to serve (the
	// c == nil marker/listing path — e.g. FilterableProviders — must stay transport-free, so meta
	// registers with a nil client there; Provider()/boardless() never touch it). If the
	// deterministic transport build ever fails, meta is left unregistered so config validation
	// fails fast on the "meta" entry, rather than registering a client guaranteed to be rejected by
	// Meta's edge.
	if c == nil {
		registry["meta"] = NewMetaCareers(nil)
		registry["bayt"] = NewBayt(nil)
		registry["gulftalent"] = NewGulfTalent(nil)
	} else if fp, err := newFingerprintHTTP(); err == nil {
		registry["meta"] = NewMetaCareers(fp)
		registry["bayt"] = NewBayt(fp)
		registry["gulftalent"] = NewGulfTalent(fp)
	}
	return registry
}

// proxiedProviders is the opt-in allowlist of providers whose crawl must egress through
// the configured proxy because the prod datacenter IP is anti-bot IP-blocklisted by their
// edge (eightfold 403s every prod-IP request while a residential IP is served normally).
// Membership here is the per-provider opt-in: only these route through the proxy when one
// is configured; every other provider stays on the direct IP. Each value rebuilds the
// adapter over the proxied client, so adding the next blocked provider is one line.
//
// Only providers served by the standard client belong here — the fingerprint-client
// providers (bayt/gulftalent) would need proxy support wired into fingerprintHTTP instead.
var proxiedProviders = map[string]func(HTTPClient) Source{
	"eightfold": func(c HTTPClient) Source { return NewEightfold(c) },
	"wantapply": func(c HTTPClient) Source { return NewWantapply(c) },
	// djinni.co IP-blocklists the prod datacenter IP: a fast crawl escalates to a hard "your
	// IP has been blocked" page, while a residential IP is served the full JSON-LD listing.
	"djinni": func(c HTTPClient) Source { return NewDjinni(c) },
	// 2gis tarpits datacenter IPs (the prod IP times out at 25s); the residential proxy is
	// served in ~2s. It uses the standard HTMLGetter, so the proxied client serves it directly.
	"2gis": func(c HTTPClient) Source { return NewTwoGIS(c) },
	// careers-page.com rate-limits the prod datacenter IP (429) once the per-board detail
	// fan-out exceeds its window — every large board then under-fills silently (board_health
	// stays green) and even the single-request listing 429s during the cooldown. Unlike the
	// others this is volume rate-limiting, not a hard blocklist (spaced requests from the prod
	// IP pass), so egressing through a fresh proxy IP keeps its crawl off the penalised prod IP.
	// Also rate-paced (pacedCareerPageGetter) so a full run stays under the window even on the
	// fresh proxy IP — concurrency limits the burst, pacing limits the total-per-window.
	"careerspage": func(c HTTPClient) Source { return NewCareerPage(pacedCareerPageGetter(c)) },
	// cleverstaff.net is untested from the prod datacenter IP (the spike ran from a residential
	// IP). It is pre-wired here so that, if the prod IP is blocked like djinni's, setting
	// SOURCES_PROXY_URL routes only this provider through the proxy with no code change; while
	// the proxy is unset this entry is inert.
	"cleverstaff": func(c HTTPClient) Source { return NewCleverstaff(c) },
	// peopleforce.io rate-limits the prod datacenter IP (429): a full 61-board run's
	// listing+detail volume poisons the IP after the first few boards, and every later
	// board's single listing GET then 429s. Like careerspage this is volume rate-limiting,
	// not a hard blocklist, so egressing through a fresh proxy IP keeps the crawl off the
	// penalised prod IP; the adapter's narrow detail pool bounds the per-board burst.
	"peopleforce": func(c HTTPClient) Source { return NewPeopleForce(c) },
	// onstrider.com sits behind Cloudflare and is untested from the prod datacenter IP (the
	// spike ran from a residential IP). It is pre-wired here so that, if the prod IP is blocked
	// like djinni's, setting SOURCES_PROXY_URL routes only this provider through the proxy with
	// no code change; while the proxy is unset this entry is inert.
	"onstrider": func(c HTTPClient) Source { return NewOnstrider(c) },
	// geekjob.ru is a Russian board reached only over the prod datacenter IP in production and is
	// untested from it (the spike ran from a residential IP). Like its RU sibling habr_career it
	// fetches a per-vacancy detail page for the description, so a WAF challenge on the detail HTML
	// would leave jobs with empty descriptions. It is pre-wired here so that, if the prod IP is
	// blocked, setting SOURCES_PROXY_URL routes only this provider through the proxy with no code
	// change; while the proxy is unset this entry is inert. A fixed, trusted host (SSRF caveat).
	"geekjob": func(c HTTPClient) Source { return NewGeekjob(c) },
	// career.habr.com sits behind Qrator, which challenges the per-vacancy detail HTML from the
	// prod datacenter IP (the listing JSON passes, but the description parse fails, leaving jobs
	// with empty descriptions and so no derived skills/geo/enrichment). A residential IP is served
	// the full page, so the whole crawl egresses through the proxy — like the others, listing and
	// detail alike. A fixed, trusted host, so it satisfies the SSRF caveat above.
	"habr_career": func(c HTTPClient) Source { return NewHabrCareer(c) },
	// vagas.com.br blocks the prod datacenter IP (403 on the first listing GET) AND rate-limits
	// by a per-IP request budget (429 once a full crawl's volume hits one IP). So it egresses
	// through the proxy like careerspage AND is rate-paced (pacedVagasGetter) to hold its
	// aggregate request rate under vagas's window on the single proxy IP — concurrency bounds the
	// burst, pacing bounds the total-per-window. NewVagas takes an HTMLGetter, which HTTPClient
	// satisfies, and the pacer wraps it before NewVagas.
	"vagas": func(c HTTPClient) Source { return NewVagas(pacedVagasGetter(c)) },
}

// ApplyProxyEgress rewires the proxiedProviders in registry to egress through the proxy
// named by SOURCES_PROXY_URL (form http://user:pass@host:port). It is a no-op when the
// variable is empty (every provider stays direct) and returns an error — for fail-fast at
// worker startup — when the variable is set but unparseable, rather than silently leaving a
// meant-to-be-proxied provider on the blocked direct IP. The proxy endpoint and its
// credentials live entirely in the environment; nothing about the proxy is hardcoded.
func ApplyProxyEgress(registry map[string]Source) error {
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Redacted() strips any password so the invalid value can be surfaced safely.
		return fmt.Errorf("sources: invalid SOURCES_PROXY_URL %q", redactProxy(raw))
	}
	proxied := NewProxyClient(u)
	for name, build := range proxiedProviders {
		if _, ok := registry[name]; ok {
			registry[name] = build(proxied)
		}
	}
	return nil
}

// redactProxy returns a proxy URL string with any credentials removed, so a malformed
// value can appear in an error without leaking the password.
func redactProxy(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Redacted()
	}
	// Unparseable: fall back to the scheme+host prefix so we never echo embedded creds.
	if i := strings.Index(raw, "@"); i >= 0 {
		return "<redacted>" + raw[i:]
	}
	return raw
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

// parseRFC3339 parses an RFC3339 timestamp (the common ATS format). RFC3339Nano
// accepts both fractional and plain-second forms (Z and colon offsets), a strict
// superset of RFC3339. As a fallback it accepts the RFC822-style numeric offset
// WITHOUT a colon (e.g. iCIMS careers-home's "2026-06-30T15:42:00+0000"), which
// RFC3339 rejects — the fallback layout only matches numeric offsets, so a Z or
// colon-offset string that reached here (parse or NotFuture failure) still yields nil.
func parseRFC3339(s string) *time.Time {
	if t := parseLayout(time.RFC3339Nano, s); t != nil {
		return t
	}
	return parseLayout("2006-01-02T15:04:05.999999999-0700", s)
}

// parseRFC3339OrDate parses a JobPosting datePosted that a board may emit as either a full
// RFC3339 timestamp or a bare "2006-01-02" date, trying the timestamp first. Shared by the
// ld+json adapters whose datePosted format varies by tenant (briefhq, northstone).
func parseRFC3339OrDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseDate(s)
}

// parseDate parses a date-only timestamp ("2006-01-02", as Workable emits).
func parseDate(s string) *time.Time { return parseLayout("2006-01-02", s) }

// ParseDate, ParseEpochMillis, and ParseRFC3339 are the exported forms used by
// sibling packages (internal/linksource) that resolve a single posting and need
// the same posted_at funnel — date-only, epoch-millis, and RFC3339 inputs all
// pass through NotFuture.
func ParseDate(s string) *time.Time        { return parseDate(s) }
func ParseEpochMillis(ms int64) *time.Time { return parseEpochMillis(ms) }
func ParseRFC3339(s string) *time.Time     { return parseRFC3339(s) }

// parseSpaceTime parses a space-separated, zone-named timestamp ("2006-01-02 15:04:05
// MST", as Recruitee emits). Recruitee emits UTC; an unrecognized zone abbreviation
// would be read as offset 0, acceptable for an approximate posted_at.
func parseSpaceTime(s string) *time.Time { return parseLayout("2006-01-02 15:04:05 MST", s) }

// parsePubDate parses an RSS <pubDate>, an RFC1123 timestamp with or without a numeric zone
// offset. Shared by the RSS-feed adapters (trakstar, earcu, likeit).
func parsePubDate(s string) *time.Time {
	if t := parseLayout(time.RFC1123Z, s); t != nil {
		return t
	}
	return parseLayout(time.RFC1123, s)
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
