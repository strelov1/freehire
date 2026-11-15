package handler

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
)

// pageParamsBounded reads and clamps the shared limit/offset query params, with
// caller-supplied bounds for endpoints sized differently from the shared list cap (the
// unpaginated tracking board; the role-cluster copies list).
//
// The unbounded-depth reader: every PUBLIC list calls pageParamsWindowed instead; what is
// left here are one user's own resources (tracking board, inbox, notifications).
//
// It is the ONLY place the offset param is read, and a test enforces that. The clamp to
// MaxInt32 matters: paginated columns bind as Postgres int4, QueryInt accepts a 64-bit value,
// and the int32 conversion wraps NEGATIVE, which Postgres rejects, so ?offset=3000000000
// would be a 500 rather than an empty page.
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

// pageParamsWindowed is pageParamsBounded plus the deep-pagination refusal: 400 rather than
// serving a page whose offset reaches past maxPageWindow.
//
// The clamp bounds the VALUE, not the WORK: `LIMIT n OFFSET k` reads and discards k rows, so
// a caller-chosen offset sets how much of `jobs` one request walks (minutes of disk on
// millions of rows). Deep-offset walkers exhaust the pool (defaultMaxConns = 10) and starve
// every other endpoint. Request frequency does not bound per-request cost (see
// public_read_limit.go, whose budgets split by cost, not by path).
//
// Refusing rather than clamping is deliberate: a clamped page answers 200 with rows nobody
// asked for, so a walker would loop on the same page and a person would see the wrong slice.
// One constant decides for both the Meili and Postgres lists.
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

// pageParams is pageParamsWindowed at the shared list caps. It returns an error where the
// old pageParams returned two values, so any call site left behind fails to compile rather
// than skipping the window.
func pageParams(c *fiber.Ctx) (limit, offset int, err error) {
	return pageParamsWindowed(c, defaultLimit, maxLimit)
}

// listResponseWithIgnored is the shared list envelope plus the query params the filter did
// not read and the facet values outside their closed vocabulary (values: see
// search.IgnoredValues; nil for an endpoint that builds no job filter). Each is omitted when
// empty: an always-present empty array is a field every reader learns to skip, including the
// one time it carries a warning.
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
