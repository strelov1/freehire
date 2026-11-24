// The outbound click redirect: GET /api/v1/go/<slug>?src=<surface>. Every link that sends a visitor from
// freehire to an employer's site points here, so the click is counted per job and per surface.
package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tracerlink"
)

// goStore is what the redirect needs: resolve a slug (jobSlugReader) and write one click.
type goStore interface {
	jobSlugReader
	RecordJobOutboundClick(ctx context.Context, arg db.RecordJobOutboundClickParams) error
}

type goHandlers struct {
	store goStore
	// salt keys the visitor hash, shared with the CV tracer. Empty means visitors cannot be told
	// apart; the click is still recorded.
	salt string
}

func newGoHandlers(queries *db.Queries, salt string) *goHandlers {
	return &goHandlers{store: queries, salt: salt}
}

func (h *goHandlers) register(api fiber.Router) { api.Get("/go/:slug", h.Redirect) }

// redirectOut is how every click redirect (this one, the sponsored one) sends a visitor on: never
// cached, never indexed, and with the referrer the employer's analytics expect.
func redirectOut(c *fiber.Ctx, to string) error {
	c.Set("Cache-Control", "no-store")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Referrer-Policy", "no-referrer-when-downgrade")
	return c.Redirect(to, fiber.StatusFound)
}

// Redirect records the click and sends the visitor to the job's application destination.
//
// Two invariants. The destination comes only from the stored job, never from the request, so the
// endpoint cannot be aimed at an arbitrary site by a crafted link. And the click write is
// best-effort: the visitor is sent on whether or not it succeeds, since the link lives in an
// email, a chat answer or an agent's output that the reader cannot retry.
//
// A closed job still redirects: the link was handed out while it was open, the employer's own page
// tells the visitor it is closed, and an extra stop on our page would only hide the click. Only a
// job with no destination to send anyone to (unknown, private, or one that takes applications on
// freehire itself) gets the page.
func (h *goHandlers) Redirect(c *fiber.Ctx) error {
	row, err := jobBySlugOrCode(c.Context(), h.store, c.Params("slug"))
	if err == nil {
		err = jobVisibleToCaller(row, 0, false)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusGone).Type("html").SendString(gonePage("This job posting is no longer available on freehire."))
	}
	if err != nil {
		// A pool timeout is not "gone": it must not tell a reader the posting was removed.
		return err
	}

	dest := outboundurl.Tag(jobview.RowDestination(row))
	if !isWebURL(dest) {
		return c.Status(fiber.StatusGone).Type("html").SendString(gonePage("This job posting has no application link."))
	}

	ua := string(c.Request().Header.UserAgent())
	client := tracerlink.Classify(c.Method(), ua)
	// Its own deadline: a saturated pool must delay the click record, never the redirect.
	ctx, cancel := context.WithTimeout(c.UserContext(), clickTimeout)
	defer cancel()
	_ = h.store.RecordJobOutboundClick(ctx, db.RecordJobOutboundClickParams{
		JobID:        row.ID,
		Src:          string(outboundurl.ParseSrc(c.Query("src"))),
		IsLikelyBot:  client.IsBot,
		DeviceType:   client.DeviceType,
		OsFamily:     client.OSFamily,
		UaFamily:     client.UAFamily,
		ReferrerHost: referrerHost(c.Get(fiber.HeaderReferer)),
		Country:      country(c.Get("CF-IPCountry")),
		VisitorHash:  tracerlink.VisitorHash(h.salt, c.IP(), ua),
	})
	return redirectOut(c, dest)
}

// isWebURL keeps a stored value that is not an http(s) address (a crawler or moderator typo, a
// javascript: URL) from being sent to a browser.
func isWebURL(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://")
}

// country is Cloudflare's two-character CF-IPCountry (ISO code, or XX / T1), upper-cased. Anything
// else is dropped: where the request did not come through Cloudflare the header is the caller's to
// write, and the column is for the country, not for whatever a client puts there.
func country(header string) string {
	h := strings.ToUpper(strings.TrimSpace(header))
	if len(h) != 2 {
		return ""
	}
	for _, r := range h {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return h
}
