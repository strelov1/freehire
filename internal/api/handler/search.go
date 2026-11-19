package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/engage/companywatch"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/formeremployer"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"
)

// searchHandlers serves the Meilisearch-backed job search surfaces: the keyword
// search, the agent search (always full descriptions in a selectable format), the
// similar-jobs read, and the facet distribution. search/facets are two narrow
// views of the same client, kept separate so the concerns stay decoupled; both
// nil when Meilisearch is unconfigured (the endpoints then report 503).
type searchHandlers struct {
	search searcher
	// descriptions loads full job descriptions by internal id, to rehydrate the
	// truncated search preview on the agent search endpoint. *db.Queries in
	// production; a fake in tests.
	descriptions jobDescriptions
	queries      *db.Queries
	facets       facetCounter
	// companyNames names the top companies of a company_slug facet request.
	// *db.Queries in production; nil names each company by its slug.
	companyNames companyNamer
	// cache holds facet distributions for facetCacheTTL. Best-effort and optional:
	// a nil cache (search configured but no Redis) simply recomputes every request,
	// the shape every facet test but the caching ones runs under.
	cache cache.Cache
	// userProfile reads the caller's profile skills for the match sort. Nil disables
	// that sort entirely, which is exactly how it degrades everywhere else.
	userProfile profileReader
	// agentViews receives the postings /agent/jobs/search serves; nil records nothing.
	agentViews agentview.Sink
	// appliedCompanies reads the employers a signed-in caller has applied to, for
	// /jobs/search's hide_applied_companies. Nil leaves the param unhonoured, and
	// reported as such.
	appliedCompanies appliedCompanyReader
	// formerEmployers resolves a signed-in caller's experience bank to the companies the
	// "Hiring again" filter narrows to. Nil leaves former_employers unread, so it is
	// reported in meta.ignored_params like any other param nothing reads.
	formerEmployers formerEmployerLookup
	// watchlist reads a signed-in caller's company watchlist, which `watchlist=true`
	// narrows to. Nil leaves the param unread, and reported in meta.ignored_params.
	watchlist watchlistLookup
	// seen reads the jobs a signed-in caller has already opened, for /jobs/search's
	// hide_seen. Nil leaves the param unhonoured, and reported as such.
	seen seenJobs
	// removals reads which of a page's jobs are already closed but still indexed. Nil
	// skips the check, which is how a search without a database behind it runs.
	removals pendingRemovals
}

// profileReader loads a caller's profile. *userprofile.Service satisfies it; tests
// inject a fake. Only the skills are read here — the match sort has no opinion on
// location preferences or specializations.
type profileReader interface {
	Get(ctx context.Context, userID int64) (userprofile.Profile, error)
}

func newSearchHandlers(search searcher, facets facetCounter, queries *db.Queries, c cache.Cache, profiles profileReader) *searchHandlers {
	h := &searchHandlers{
		search: search, descriptions: queries, queries: queries, facets: facets, cache: c,
		userProfile: profiles,
	}
	// Assign only when set: a nil *db.Queries in the interface would be non-nil.
	if queries != nil {
		h.companyNames = queries
		h.formerEmployers = formeremployer.NewStore(queries)
		h.watchlist = companywatch.New(queries)
	}
	return h
}

// searchRecordTimeout bounds the fire-and-forget demand write. Short: it is one
// upsert on a small table, and a write still running past this is a database in
// trouble rather than a slow row.
const searchRecordTimeout = 5 * time.Second

func (h *searchHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Literal routes are registered before the /jobs/:slug param route (see
	// Register) so they are not read as slugs.
	api.Get("/jobs/search", mw.optional, contract)
	// Agent search: same query as /jobs/search, but always full descriptions in a
	// selectable format for programmatic consumers. Public, like the other reads.
	api.Get("/agent/jobs/search",
		agentViews(h.agentViews, agentview.SurfaceAgentAPI), contract)
	api.Get("/jobs/facets", contract)
	api.Get("/jobs/:slug/similar", contract)
	// The CV-first onboarding's match search: ranked by a posted skill list, so a visitor
	// with no account (and no stored profile) still gets the match sort. Public.
	api.Post("/onboarding/matches", contract)
}

// searcher is the search backend the handler depends on. *search.Client
// satisfies it; tests inject a fake. A nil searcher means search is not
// configured (no MEILI_MASTER_KEY) and search endpoints report 503. Similar jobs
// use the same Search method with mandatory location and specialization filters.
type searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
}

// The pagination guard the search endpoints apply is maxPageWindow (api.go). It is decoupled from the index's maxTotalHits (which only caps the reported total), so the total can read the true filtered count while deep offset paging stays refused.

// matchVector builds the caller's skill vector for ?sort=match, or nil when the sort
// was not asked for or cannot be served.
//
// EVERY "cannot" degrades to the default feed: no session, no profile service wired,
// no profile, no skills, no recognised skills, no weights. A saved search or a shared
// link carrying sort=match must never 400 when opened by someone it cannot be served
// for — the same reason the jobs list ignores unknown filters rather than refusing
// them.
func (h *searchHandlers) matchVectorFor(ctx context.Context, userID int64, vals url.Values) []float32 {
	vector, _ := h.matchProfileFor(ctx, userID, vals)
	return vector
}

// matchProfileFor is matchVectorFor plus the profile skills the vector was built from,
// which the minimum-skill-match scan needs to state each posting's percent.
func (h *searchHandlers) matchProfileFor(ctx context.Context, userID int64, vals url.Values) (vector []float32, skills []string) {
	if vals.Get("sort") != sortMatch || h.userProfile == nil {
		return nil, nil
	}
	// auth.UserID is what mw.optional populates; it reports false for an anonymous caller.
	// requireUserID is the wrong helper: its 401 is the outcome this must avoid.
	if userID == 0 {
		return nil, nil
	}
	profile, err := h.userProfile.Get(ctx, userID)
	if err != nil || len(profile.Skills) == 0 {
		return nil, nil
	}
	// ProfileVector, never JobVector: the ballast belongs to the stored side only.
	return skillvec.ProfileVector(profile.Skills), profile.Skills
}

// SearchJobs runs a full-text/faceted keyword search over the jobs index. It is
// public (unauthenticated) like the other job reads. Response: {"data": [job
// view...], "meta": {total, limit, offset}} — results carry public_slug and never
// the internal id.
func (h *searchHandlers) SearchJobs(ctx context.Context, _ apigen.SearchJobsParams) (*apigen.JobSearchPageHeaders, error) {
	vals := contractQuery(ctx)
	userID, _ := auth.UserIDFromContext(ctx)
	seen, seenIgnored := h.seenExclusionFor(ctx, userID, vals)
	appliedFrag, appliedIgnored := h.appliedCompanyExclusionFor(ctx, userID, vals)
	match, matchIgnored := h.skillMatchFilterFor(ctx, userID, vals)
	// The answer depends on who asked, under a URL that does not say so.
	private := match != nil ||
		(userID != 0 && !seenIgnored && wantsParam(vals, hideSeenParam)) ||
		(userID != 0 && !appliedIgnored && wantsParam(vals, hideAppliedCompaniesParam))
	out, err := h.runJobSearch(ctx, vals, userID, seen, appliedFrag, match)
	if err != nil {
		return nil, err
	}
	res, limit, offset, dropped := out.res, out.limit, out.offset, out.dropped
	if appliedIgnored {
		dropped = append(dropped, search.UnknownParam{Param: hideAppliedCompaniesParam})
	}
	if seenIgnored {
		dropped = append(dropped, search.UnknownParam{Param: hideSeenParam})
	}
	if matchIgnored {
		dropped = append(dropped, search.UnknownParam{Param: minSkillMatchParam})
	}

	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}
	h.attachGhost(ctx, res.Hits, views)
	data, err := marshalRaws(views)
	if err != nil {
		return nil, err
	}

	ignored := mergedJobSearchIgnored(vals, out.own(publicJobSearchParams), dropped)
	meta := apigen.JobSearchMeta{
		Total: res.Total, Limit: int64(limit), Offset: int64(offset),
		IgnoredParams: contractIgnoredParams(ignored),
		IgnoredValues: contractIgnoredValues(ignoredJobValues(vals)),
	}
	if res.TotalIsLowerBound {
		meta.TotalIsLowerBound = apigen.NewOptBool(true)
	}
	return &apigen.JobSearchPageHeaders{
		CacheControl: optString(privateCacheControl(private || out.private)),
		Response:     apigen.JobSearchPage{Data: data, Meta: meta},
	}, nil
}

// privateCacheControl is the header of an answer that depends on who asked, or "" for one
// that does not.
func privateCacheControl(private bool) string {
	if private {
		return "private, no-store"
	}
	return ""
}

// wantsParam reads a boolean query param the way Fiber's QueryBool does: the first value,
// false when it is absent or not a boolean.
func wantsParam(vals url.Values, key string) bool {
	want, err := strconv.ParseBool(vals.Get(key))
	return err == nil && want
}

// jobSearchOutcome is what runJobSearch hands back: the raw hits, the applied
// limit/offset and the value-level ignored-param reports.
type jobSearchOutcome struct {
	res     search.SearchResult
	limit   int
	offset  int
	dropped []search.UnknownParam
	// formerEmployers reports that the "Hiring again" filter was applied, which makes
	// former_employers one of the endpoint's own params rather than an ignored one.
	formerEmployers bool
	// watchlist is the same for `watchlist=true`.
	watchlist bool
	// private is set by runJobSearch when either of those was applied: the answer is
	// one person's own.
	private bool
}

// own is the endpoint's own params for this request: base, plus former_employers and
// watchlist when the filter read them. Anonymous, they were not read and the report must
// say so: the answer is then the whole catalogue, which a caller who asked for their
// employers or watchlist must not mistake for them.
func (o jobSearchOutcome) own(base []string) []string {
	out := slices.Clip(base)
	if o.formerEmployers {
		out = append(out, formerEmployersParam)
	}
	if o.watchlist {
		out = append(out, search.WatchlistParam)
	}
	return out
}

// runJobSearch performs the request handling shared by both job-search endpoints
// (availability check, pagination-window guard, index query), so they cannot drift. The
// guards return a fiber *Error the caller can return directly; `dropped` carries q_fields
// naming a field outside the searchable vocabulary plus any filter params dropped by the
// degrade below, for the caller to merge into its ignoredParams(vals, own) report.
//
// exclude names job ids to keep out of the result (hide_seen); nil excludes nothing.
// personal is the applied-company exclusion fragment; empty leaves the result unchanged.
// match, when set, keeps only postings at or above the caller's minimum skill match (see
// skillMatchFilter).
func (h *searchHandlers) runJobSearch(ctx context.Context, vals url.Values, userID int64, exclude []int64, personal string, match *skillMatchFilter) (jobSearchOutcome, error) {
	if h.search == nil {
		return jobSearchOutcome{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	limit, offset, err := contractPageWindowed(ctx, defaultLimit, maxLimit)
	if err != nil {
		return jobSearchOutcome{}, err
	}

	// QueryBool historically accepts the alternate strconv boolean spellings.
	formerVals := vals
	if wantsParam(vals, formerEmployersParam) {
		formerVals = maps.Clone(vals)
		formerVals.Set(formerEmployersParam, "true")
	}
	former, formerApplied, err := h.formerEmployerSlugsFor(ctx, userID, formerVals)
	if err != nil {
		return jobSearchOutcome{}, err
	}
	// Only the spelling search.WantsWatchlist reads, the one Narrows and cmd/notify read too.
	watch, watchApplied, err := h.watchlistSlugsFor(ctx, userID, vals)
	if err != nil {
		return jobSearchOutcome{}, err
	}
	out, err := h.executeJobSearch(ctx, vals, jobSearchInput{limit: limit, offset: offset, former: former, hasFormer: formerApplied, watch: watch, hasWatch: watchApplied, vector: h.matchVectorFor(ctx, userID, vals), exclude: exclude, personal: personal, match: match})
	// Both are one person's own sets; no shared cache may keep the answer.
	out.private = formerApplied || watchApplied
	return out, err
}

type jobSearchInput struct {
	limit, offset int
	former        []string
	hasFormer     bool
	watch         []string
	hasWatch      bool
	vector        []float32
	exclude       []int64
	personal      string
	strict        bool
	match         *skillMatchFilter
}

func (h *searchHandlers) executeJobSearch(ctx context.Context, vals url.Values, in jobSearchInput) (jobSearchOutcome, error) {
	limit, offset := in.limit, in.offset
	vector, exclude, personal, strict := in.vector, in.exclude, in.personal, in.strict

	companies, companiesApplied := personalCompanies(in.former, in.hasFormer, in.watch, in.hasWatch)
	if companiesApplied && len(companies) == 0 {
		// Narrow to nothing rather than drop the filter: the set the caller asked for is
		// empty, and searching without it would answer with strangers.
		return jobSearchOutcome{limit: limit, offset: offset, formerEmployers: in.hasFormer, watchlist: in.hasWatch}, nil
	}

	sort := searchSortValues(vals, vector != nil)
	base := search.FilterFromValues(vals)
	// companyFilter is the personal company group alone, kept for the degrade below.
	filter, companyFilter := base, any(nil)
	if companiesApplied {
		filter = search.AndInStrings(base, "company_slug", companies)
		companyFilter = search.AndInStrings(nil, "company_slug", companies)
	}
	qFields, qFieldsIgnored := search.QFieldsFromValues(vals)
	params := search.SearchParams{
		Query:   vals.Get("q"),
		Filter:  withExcludedIDs(withFilterGroup(filter, personal), exclude),
		Sort:    sort,
		Vector:  vector,
		QFields: qFields,
		Prefix:  wantsParam(vals, qPrefixParam),
		Limit:   limit,
		Offset:  offset,
	}

	if strict && len(qFieldsIgnored) > 0 {
		return jobSearchOutcome{}, fmt.Errorf("search fields cannot be enforced")
	}
	dropped := qFieldsIgnored
	var (
		res search.SearchResult
		err error
	)
	if in.match != nil {
		res, err = in.match.scan(ctx, h.search, params)
	} else {
		res, err = h.search.Search(ctx, params)
	}

	if err != nil {
		// Degrade, not fail, when the filter itself is why Meilisearch refused the query: the realistic trigger is a filterable attribute declared in code before the live index has it. base == nil means nothing to blame, and retrying unchanged would fail identically.
		// The personal company group ("Hiring again", the watchlist) survives the retry: company_slug has always been filterable, and dropping it would answer a former-employers request with the whole catalogue.
		if strict || base == nil || !errors.Is(err, search.ErrBadQuery) {
			// Returned as is so the cause stays visible to logging; RenderError makes it a 500 (400 for ErrBadQuery).
			return jobSearchOutcome{}, err
		}
		// Both personal exclusions survive the retry: id and company_slug have always been filterable.
		retry := search.SearchParams{
			Query: vals.Get("q"), Filter: withExcludedIDs(withFilterGroup(companyFilter, personal), exclude),
			Sort: sort, Vector: vector, QFields: qFields, Prefix: params.Prefix, Limit: limit, Offset: offset,
		}
		if in.match != nil {
			res, err = in.match.scan(ctx, h.search, retry)
		} else {
			res, err = h.search.Search(ctx, retry)
		}
		if err != nil {
			return jobSearchOutcome{}, err
		}
		// Which single param Meilisearch minded is not knowable from its own
		// error, so every active filter param is reported rather than a guess.
		dropped = append(dropped, search.ActiveFilterParams(vals)...)
	}

	res = h.dropPendingRemovals(ctx, res)
	h.recordQuery(vals.Get("q"))
	return jobSearchOutcome{
		res: res, limit: limit, offset: offset, dropped: dropped,
		formerEmployers: in.hasFormer,
		watchlist:       in.hasWatch,
	}, nil
}

// recordQuery notes what the catalogue was asked for, so the suggestion dictionary can
// rank by demand rather than only by supply.
//
// Three properties, all deliberate:
//
//   - It CANNOT fail the search. A write error is logged and discarded: the result is
//     what the visitor asked for and this is a by-product.
//   - It CANNOT delay the response. It runs on its own goroutine with its own bounded
//     context, because c.Context() is released when the handler returns.
//   - It identifies nobody. The row is the phrase and a counter; no user, no session,
//     no address — and `Recordable` refuses text that is not a search phrase at all,
//     so a pasted email address or job description never reaches the table.
//
// The key is the same normalisation the builder applies to mined titles, so a typed
// query and the title it names land on one row.
func (h *searchHandlers) recordQuery(raw string) {
	q := demandKey(raw)
	if !suggest.Recordable(q) || h.queries == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), searchRecordTimeout)
		defer cancel()
		if err := h.queries.RecordSearchQuery(ctx, q); err != nil {
			log.Printf("search: record query: %v", err)
		}
	}()
}

// demandKey normalises a raw `q` into the same key suggest.Title gives a mined posting title. A query wrapped in a matching pair of `"` (a title suggestion click) is unwrapped first, since that is Meilisearch quoting, not part of the phrase; any other quoting is left alone.
func demandKey(raw string) string {
	if len(raw) >= 2 && strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		raw = raw[1 : len(raw)-1]
	}
	return suggest.Title(raw)
}

// buildSearchFilter turns the request's facet query params into a Meilisearch
// filter by delegating to the shared, pure search.FilterFromValues — the same
// translation the notification matcher applies to a saved search's stored query,
// so the two cannot drift. Returns nil when no facet is set.
func buildSearchFilter(vals url.Values) any {
	return search.FilterFromValues(vals)
}

// attachGhost attaches the ghost signal to a page of search hits. Costs two extra reads per page: the outcome evidence (sparse) and the absence stamps.
//
// The stamps come from Postgres because cmd/reindex pushes only documents whose content_hash moved and no adapter writes ats_absent_at. The reality class IS on the document (search.FromJob promotes it), so it is read from the hit.
//
// Best-effort: a failed lookup leaves the signal off the page, but is logged (see logGhostLookup for why cancellation is told apart), since otherwise the badge vanishes while the endpoint keeps answering 200.
func (h *searchHandlers) attachGhost(ctx context.Context, hits []search.JobDocument, views []jobview.Job) {
	if len(hits) == 0 || h.queries == nil {
		return
	}
	ids := make([]int64, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}

	stampRows, err := h.queries.ListJobGhostStamps(ctx, ids)
	if err != nil {
		logGhostLookup(ctx, "absence stamps", len(ids), err)
		return
	}
	stamps := make(map[int64]db.ListJobGhostStampsRow, len(stampRows))
	for _, r := range stampRows {
		stamps[r.ID] = r
	}
	evidence := ghostEvidenceFor(ctx, h.queries, ids)

	now := time.Now()
	for i, hit := range hits {
		realityClass := ""
		if hit.Reality != nil {
			realityClass = hit.Reality.Class
		}
		row := stamps[hit.ID]
		views[i].Ghost = jobview.ClassifyGhost(jobview.GhostInput{
			Now: now,
			// Read from Postgres, not the hit: a sweep-closed job stays in the index until a reindex, so the index cannot say a posting is still up.
			Closed:       row.ClosedAt.Valid,
			RealityClass: realityClass,
			ATSAbsentAt:  row.AtsAbsentAt.Time,
			HasATSAbsent: row.AtsAbsentAt.Valid,
			Evidence:     evidence[hit.ID],
		})
	}
}

// withFilterGroup ANDs one more fragment onto buildSearchFilter's result — either a
// [][]string of facet groups or nil. An empty fragment leaves the filter untouched.
func withFilterGroup(base any, frag string) any {
	if frag == "" {
		return base
	}
	groups, _ := base.([][]string) // nil base → nil groups; append allocates
	return append(groups, []string{frag})
}
