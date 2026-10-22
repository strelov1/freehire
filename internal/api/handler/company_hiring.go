package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// lifetimeSampleGate is how many employer-side closures a company needs before its
// median posting lifetime is served at all. Below it the field is absent: a median over
// two postings says which two postings happened to close, not how the company hires.
//
// Like responseSampleGate and replySampleGate this is a judgement, not a measurement,
// and should be revisited once the figure has been read against real companies.
const lifetimeSampleGate = 5

// companyHiringResponse is the Hiring tab's payload. The reply rate is not repeated
// here: the page reads the company payload for its header anyway, and that payload
// already carries the gated `response` — one source, one gate.
type companyHiringResponse struct {
	// Open, OpenPrev and Growth come from insights_company_growth's 30-day window.
	// A company with no row has nothing open now or then, so zeros are the measurement,
	// not a fallback.
	Open     int32           `json:"open"`
	OpenPrev int32           `json:"open_prev"`
	Growth   int32           `json:"growth"`
	Series   []activityPoint `json:"series"`
	// Lifetime is absent below lifetimeSampleGate, never zero: a zero-day median would
	// read as "fills roles instantly".
	Lifetime *companyLifetime `json:"lifetime,omitempty"`
}

type companyLifetime struct {
	Closed     int32   `json:"closed"`
	MedianDays float32 `json:"median_days"`
}

// CompanyHiring serves GET /api/v1/companies/:slug/hiring: one company's added vs.
// removed series (the /trends chart scoped to an employer), its open count and 30-day
// change, and how long its postings typically stay open. The window is parsed exactly
// as /stats/jobs-activity parses it, so the two charts accept the same params.
func (h *companiesHandlers) CompanyHiring(c *fiber.Ctx) error {
	window, err := parseActivityQuery(c.Query("granularity"), c.Query("from"), c.Query("to"), time.Now().UTC())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	slug := c.Params("slug")
	if _, redirected, err := h.lookupCompany(c, slug); err != nil || redirected {
		return err
	}

	rows, err := h.queries.ListCompanyActivity(c.Context(), db.ListCompanyActivityParams{
		CompanySlug: slug,
		Unit:        window.Granularity,
		FromTs:      pgtype.Timestamp{Time: window.From, Valid: true},
		ToTs:        pgtype.Timestamp{Time: window.To, Valid: true},
	})
	if err != nil {
		return err
	}
	out := companyHiringResponse{Series: make([]activityPoint, len(rows))}
	for i, r := range rows {
		out.Series[i] = activityPoint{Period: r.Period.Time.Format(dateLayout), Added: r.Added, Removed: r.Removed}
	}

	growth, err := h.queries.GetCompanyGrowth(c.Context(), slug)
	switch {
	case err == nil:
		out.Open, out.OpenPrev, out.Growth = growth.OpenCount, growth.OpenCountPrev, growth.OpenCount-growth.OpenCountPrev
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	lifetime, err := h.queries.GetCompanyLifetime(c.Context(), slug)
	switch {
	case err == nil:
		if lifetime.Closed >= lifetimeSampleGate {
			out.Lifetime = &companyLifetime{Closed: lifetime.Closed, MedianDays: lifetime.MedianDays}
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	return c.JSON(fiber.Map{"data": out})
}
