package handler

import (
	"context"
	"log"
	"net/url"
	"slices"
	"strconv"

	"github.com/strelov1/freehire/internal/search/search"
)

// The per-person narrowing /jobs/search applies: facts about the CALLER, not facets of the
// posting, resolved per request.

// appliedCompanyReader reads the company slugs of the employers a caller has applied
// to. *jobtracking.Service satisfies it.
type appliedCompanyReader interface {
	AppliedCompanySlugs(ctx context.Context, userID int64) ([]string, error)
}

// seenJobs reads the job ids a caller has already opened. It is the same set the feed
// dims a card by — every user_jobs row carries viewed_at, so any interaction is a view —
// capped to the most recently touched. *jobtracking.Service satisfies it.
type seenJobs interface {
	ExcludedJobIDs(ctx context.Context, userID int64) ([]int64, error)
}

// formerEmployerLookup resolves a person's past employers to company slugs.
// *formeremployer.Store satisfies it; tests inject a fake.
type formerEmployerLookup interface {
	Slugs(ctx context.Context, userID int64) ([]string, error)
}

// watchlistLookup reads a person's watchlist as company slugs. *companywatch.Service
// satisfies it; tests inject a fake.
type watchlistLookup interface {
	Slugs(ctx context.Context, userID int64) ([]string, error)
}

// formerEmployersParam is the "Hiring again" filter: only postings at companies the
// signed-in caller has worked at and left, per their experience bank. It is not part of
// the search package's vocabulary because it is not a facet of the posting — it is a fact
// about the CALLER, resolved per request — so the endpoint reads it itself, and declares it
// as its own only when it did (see jobSearchOutcome.own).
const formerEmployersParam = "former_employers"

// hideAppliedCompaniesParam asks /jobs/search to leave out the postings of every
// employer the caller has applied to. It is a per-person preference, not a filter:
// search.FilterFromValues does not read it, so a saved search carrying it narrows
// nothing and its digest ignores it. Only the public /jobs/search reads it; the agent
// search reports it like any other unread param.
const hideAppliedCompaniesParam = "hide_applied_companies"

// hideSeenParam asks /jobs/search to leave out the jobs the caller has already opened.
// It is a per-person preference, not a filter: search.FilterFromValues does not read it,
// so a saved search carrying it narrows nothing and its digest ignores it. Only the
// public /jobs/search reads it; the agent search reports it like any other unread param.
const hideSeenParam = "hide_seen"

// appliedCompanyExclusionFor resolves ?hide_applied_companies=true into the filter
// fragment that keeps the caller's applied-to employers out of this search, and
// reports whether the param has to be named in meta.ignored_params instead.
//
// The exclusion runs inside the index query rather than over the returned page, so
// meta.total and every later page stay honest: dropping hits after the fact would
// shrink a page below its limit and leave the total counting what the caller never
// sees.
//
// It is honoured for a signed-in caller only, because the set is that account's own
// applications. An anonymous caller, an unparseable value, or a record that could not
// be read all fall back to the unhidden search and say so, rather than refusing a
// shared link or quietly widening. A caller with no applications gets "" — nothing to
// exclude, and nothing ignored either.
func (h *searchHandlers) appliedCompanyExclusionFor(ctx context.Context, userID int64, vals url.Values) (frag string, ignored bool) {
	raw := vals.Get(hideAppliedCompaniesParam)
	if !vals.Has(hideAppliedCompaniesParam) {
		return "", false
	}
	want, err := strconv.ParseBool(raw)
	if err != nil {
		return "", true
	}
	if !want {
		return "", false
	}
	if userID == 0 || h.appliedCompanies == nil {
		return "", true
	}
	slugs, err := h.appliedCompanies.AppliedCompanySlugs(ctx, userID)
	if err != nil {
		log.Printf("search: hide_applied_companies: read applied companies of user %d: %v", userID, err)
		return "", true
	}
	// The answer now depends on who asked, under a URL that does not say so.
	if len(slugs) == 0 {
		return "", false
	}
	return search.NotInStrings("company_slug", slugs), false
}

// seenExclusionFor resolves ?hide_seen=true into the job ids to keep out of this search,
// and reports whether the param has to be named in meta.ignored_params instead.
//
// The exclusion happens inside the index query rather than over the returned page, so
// meta.total and every later page stay honest: hiding hits after the fact would shrink
// a page below its limit and leave the total counting what the caller never sees.
//
// It is honoured for a signed-in caller only, because "seen" is the server's record of
// what that account opened — the same record the feed dims cards by. An anonymous
// caller, an unparseable value, or a record that could not be read all fall back to the
// unhidden search and say so, rather than refusing a shared link or quietly widening.
func (h *searchHandlers) seenExclusionFor(ctx context.Context, userID int64, vals url.Values) (ids []int64, ignored bool) {
	if !vals.Has(hideSeenParam) {
		return nil, false
	}
	// A present but empty `?hide_seen=` is unreadable like any other non-boolean.
	want, err := strconv.ParseBool(vals.Get(hideSeenParam))
	if err != nil {
		return nil, true
	}
	if !want {
		return nil, false
	}
	if userID == 0 || h.seen == nil {
		return nil, true
	}
	ids, err = h.seen.ExcludedJobIDs(ctx, userID)
	if err != nil {
		log.Printf("search: hide_seen: read seen jobs of user %d: %v", userID, err)
		return nil, true
	}
	// The answer now depends on who asked, under a URL that does not say so.
	return ids, false
}

// formerEmployerSlugsFor resolves the "Hiring again" filter for this request. applied is false
// when the param is absent or false, or when nothing can read it — no session, no lookup
// wired — and the caller then searches as if it were absent (and reports it, see own).
// Applied with no slugs is a real answer: nothing banked resolves to a hiring company.
func (h *searchHandlers) formerEmployerSlugsFor(ctx context.Context, userID int64, vals url.Values) (slugs []string, applied bool, err error) {
	if h.formerEmployers == nil || vals.Get(formerEmployersParam) != "true" {
		return nil, false, nil
	}
	if userID == 0 {
		return nil, false, nil
	}
	// The answer is this person's own history; no shared cache may keep it.
	slugs, err = h.formerEmployers.Slugs(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return slugs, true, nil
}

// watchlistSlugsFor resolves `watchlist=true` for this request, with formerEmployerSlugs's
// contract: applied is false when the param is absent, anonymous or unreadable, and applied
// with no slugs is a real answer — an empty list — that narrows to nothing.
func (h *searchHandlers) watchlistSlugsFor(ctx context.Context, userID int64, vals url.Values) (slugs []string, applied bool, err error) {
	if h.watchlist == nil || !search.WantsWatchlist(vals) || userID == 0 {
		return nil, false, nil
	}
	slugs, err = h.watchlist.Slugs(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return slugs, true, nil
}

// personalCompanies combines the two per-caller company sets a search can be narrowed to:
// the companies the caller left ("Hiring again") and the ones they watch. Asked for both,
// a posting must satisfy both, so the set is their intersection. applied is false when
// neither was asked for.
func personalCompanies(former []string, hasFormer bool, watch []string, hasWatch bool) (slugs []string, applied bool) {
	switch {
	case hasFormer && hasWatch:
		return slices.DeleteFunc(slices.Clone(former), func(s string) bool { return !slices.Contains(watch, s) }), true
	case hasFormer:
		return former, true
	case hasWatch:
		return watch, true
	}
	return nil, false
}
