package handler

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
)

// pageParamsBounded reads and clamps the shared limit/offset pagination query params, with
// caller-supplied bounds for endpoints whose page is sized differently from the shared list
// cap (the tracking board, which is unpaginated and needs the whole set; the role-cluster
// copies list, which pages a handful of city openings).
//
// The unbounded-depth reader. Every PUBLIC list calls pageParamsWindowed below instead; what
// is left here are the caller's own resources — the tracking board, the inbox, the
// notification list — where the set is one user's and the depth is theirs to walk.
//
// It is the ONLY place the offset query param is read, and a test enforces that. The clamp to
// MaxInt32 is the reason: every paginated column binds as a Postgres int4, Fiber's QueryInt is
// a plain strconv.Atoi that happily accepts a 64-bit value, and the int32 conversion then wraps
// it NEGATIVE — which Postgres rejects, turning ?offset=3000000000 into a 500 rather than an
// empty page. Naming that in a comment did not stop a second call site re-implementing the
// parse without it.
func pageParamsBounded(c *fiber.Ctx, fallback, ceiling int) (limit, offset int) {
	return clampPage(c.QueryInt("limit", fallback), c.QueryInt("offset", 0), ceiling)
}

// contractPageBounded is pageParamsBounded for a contract operation, which declares its page
// params as strings so one that does not read falls back to the default, as Fiber's QueryInt
// does, rather than being refused.
func contractPageBounded(ctx context.Context, fallback, ceiling int) (limit, offset int) {
	query := contractQuery(ctx)
	return clampPage(queryIntOr(query, "limit", fallback), queryIntOr(query, "offset", 0), ceiling)
}

// contractPageWindowed is pageParamsWithin at the shared window for a contract operation.
func contractPageWindowed(ctx context.Context, fallback, ceiling int) (limit, offset int, err error) {
	limit, offset = contractPageBounded(ctx, fallback, ceiling)
	if offset+limit > maxPageWindow {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}
	return limit, offset, nil
}

func clampPage(limit, offset, ceiling int) (int, int) {
	return min(max(limit, 1), ceiling), min(max(offset, 0), math.MaxInt32)
}

// queryIntOr is Fiber's QueryInt over a parsed query: the first value, or fallback when it
// is absent or not an integer.
func queryIntOr(query url.Values, key string, fallback int) int {
	n, err := strconv.Atoi(query.Get(key))
	if err != nil {
		return fallback
	}
	return n
}

// pageParamsWindowed is pageParamsBounded plus the deep-pagination refusal: it answers 400
// rather than serving a page whose offset reaches past maxPageWindow.
//
// The clamp pageParamsBounded already applies bounds the VALUE, so Postgres accepts it; it
// does not bound the WORK. `LIMIT n OFFSET k` reads and discards k rows before returning
// any, so an offset the caller chooses sets how much of a table one request walks — and on
// `jobs` (millions of open rows, `SELECT *`, so every skipped tuple is a heap fetch) that is
// minutes of disk per request rather than milliseconds.
//
// That is not a hypothetical. On 2026-09-14 a crawler walked /api/v1/jobs?offset= in steps of
// 100 up to ~180,000, eight requests at a time. Each one pinned a pooled connection for over
// two minutes; ten of them exhausted the pool (defaultMaxConns = 10), every other endpoint
// queued behind them — /api/v1/threads was served in 15m41s — and nginx answered 504 to
// 14,007 requests over 54 minutes. Request frequency does not bound query cost: it
// bounds requests per minute, and this endpoint's COST PER REQUEST is what the caller
// controls (see public_read_limit.go, whose budgets are split "by cost, not by path" — a
// split that assumes a path HAS a fixed cost).
//
// Refusing rather than clamping to the ceiling is the deliberate half. A clamped page answers
// 200 with rows the caller did not ask for, which reads as success: a walker would loop over
// the same page forever and a person paging would silently see the wrong slice. That
// reasoning first landed on the Meili-backed lists, under a constant of their own; this
// is that constant, moved here so one number decides for both stores.
func pageParamsWindowed(c *fiber.Ctx, fallback, ceiling int) (limit, offset int, err error) {
	return pageParamsWithin(c, fallback, ceiling, maxPageWindow)
}

// pageParamsWithin is pageParamsWindowed with the window named by the caller, for an
// endpoint whose rows cost more per skipped offset than the shared window allows for.
func pageParamsWithin(c *fiber.Ctx, fallback, ceiling, window int) (limit, offset int, err error) {
	limit, offset = pageParamsBounded(c, fallback, ceiling)
	if offset+limit > window {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}
	return limit, offset, nil
}

// pageParams is pageParamsWindowed at the shared list caps — the form six of its seven call
// sites want, and the same convenience the package carried before the window existed.
//
// It returns an error where the old pageParams returned two values, which is deliberate: any
// call site left behind by the change fails to compile rather than silently skipping the
// window.
func pageParams(c *fiber.Ctx) (limit, offset int, err error) {
	return pageParamsWindowed(c, defaultLimit, maxLimit)
}

// listResponseWithIgnored is the shared list envelope plus the query params the filter did
// not read and the facet values outside their closed vocabulary (values: see
// search.IgnoredValues; nil for an endpoint that builds no job filter), each
// omitted entirely when there are none.
//
// Omitted rather than sent empty: a caller only needs the key when something
// actually went wrong, and an always-present empty array is a field every reader
// learns to skip — at which point the one response that does carry a warning
// gets skipped too.
func listResponseWithIgnored(c *fiber.Ctx, data any, total int64, limit, offset int, ignored []search.UnknownParam, values map[string][]string) error {
	return c.JSON(fiber.Map{"data": data, "meta": listMeta(total, limit, offset, ignored, values)})
}

// listMeta is listResponseWithIgnored's meta block, for a list endpoint that has one
// more thing of its own to say beside it.
func listMeta(total int64, limit, offset int, ignored []search.UnknownParam, values map[string][]string) fiber.Map {
	meta := fiber.Map{
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}
	addIgnored(meta, ignored, values)
	return meta
}

// dataResponseWithIgnored is listResponseWithIgnored for the single-item
// endpoints: they answer a bare {"data": ...} with no meta at all, so a clean
// request keeps exactly that shape and the block appears only to carry a
// warning.
func dataResponseWithIgnored(c *fiber.Ctx, data any, ignored []search.UnknownParam, values map[string][]string) error {
	meta := fiber.Map{}
	addIgnored(meta, ignored, values)
	if len(meta) == 0 {
		return c.JSON(fiber.Map{"data": data})
	}
	return c.JSON(fiber.Map{"data": data, "meta": meta})
}

// addIgnored writes the two reports into a response meta, each only when it has
// something to say.
func addIgnored(meta fiber.Map, ignored []search.UnknownParam, values map[string][]string) {
	if len(ignored) > 0 {
		meta["ignored_params"] = ignored
	}
	if len(values) > 0 {
		meta["ignored_values"] = values
	}
}

// ignoredJobValues is search.IgnoredValues over this request's query, for the
// endpoints that build a job filter from it.
func ignoredJobValues(vals url.Values) map[string][]string {
	return search.IgnoredValues(vals)
}
