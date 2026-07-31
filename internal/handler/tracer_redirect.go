// The public side of CV link tracing: the redirect a traced link in a downloaded PDF points at.
// It is the only unauthenticated surface of the feature, and the only one a stranger ever sees.
package handler

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/tracerlink"
)

// tracerLinkStore is the three queries the redirect makes. Narrow on purpose: this surface reads
// one row and writes two, and anything wider would let it grow reach it has no business having.
type tracerLinkStore interface {
	TracerLinkByToken(ctx context.Context, token string) (db.TracerLinkByTokenRow, error)
	RecordTracerClick(ctx context.Context, arg db.RecordTracerClickParams) error
	TouchCVLastClick(ctx context.Context, id pgtype.UUID) error
}

type tracerHandlers struct {
	links tracerLinkStore
	// salt keys the visitor hash. Empty means the deployment cannot identify visitors, and the
	// click is still recorded — as a click, not as a visitor.
	salt string
}

func newTracerHandlers(queries *db.Queries, salt string) *tracerHandlers {
	return &tracerHandlers{links: queries, salt: salt}
}

// goneBody is what a recruiter sees when the CV behind a link has been deleted. They did nothing
// wrong, and a bare 404 reads as a broken site rather than as a link that has expired.
const goneBody = `<!doctype html><html lang="en"><head><meta charset="utf-8">` +
	`<title>This link is no longer active</title></head><body>` +
	`<h1>This link is no longer active</h1>` +
	`<p>The CV it pointed to has been removed by its owner.</p></body></html>`

// Redirect resolves a traced link, records the click, and sends the visitor on.
//
// Two properties hold whatever else changes here. The destination comes only from the stored
// token — no query parameter, path remainder or header can name one — so the endpoint cannot be
// aimed by whoever crafts the URL. And the click write is best-effort: the redirect happens
// whether or not it succeeds, because a broken redirect lives inside a PDF the candidate can
// neither see nor fix.
func (h *tracerHandlers) Redirect(c *fiber.Ctx) error {
	link, err := h.links.TracerLinkByToken(c.Context(), c.Params("token"))
	if err != nil {
		return c.Status(fiber.StatusGone).Type("html").SendString(goneBody)
	}

	ua := string(c.Request().Header.UserAgent())
	client := tracerlink.Classify(c.Method(), ua)

	// The redirect is served from our own origin, so a candidate following a link out of their
	// own downloaded PDF arrives carrying their session. Cookie only: an API key must not be able
	// to attribute a click, and a leaked one must not be able to hide one.
	owner, signedIn := auth.UserID(c)
	isOwner := signedIn && owner == link.OwnerID

	_ = h.links.RecordTracerClick(c.Context(), db.RecordTracerClickParams{
		TracerLinkID: link.ID,
		IsLikelyBot:  client.IsBot,
		IsOwner:      isOwner,
		DeviceType:   client.DeviceType,
		OsFamily:     client.OSFamily,
		UaFamily:     client.UAFamily,
		ReferrerHost: referrerHost(c.Get(fiber.HeaderReferer)),
		VisitorHash:  tracerlink.VisitorHash(h.salt, c.IP(), ua),
	})

	// Only a click somebody else made, by hand, moves the marker the tracking board reads.
	if !client.IsBot && !isOwner {
		_ = h.links.TouchCVLastClick(c.Context(), link.ID)
	}

	return c.Redirect(link.DestinationUrl, fiber.StatusFound)
}

// referrerHost keeps the host and discards the rest. A full referrer carries the path and query of
// whatever page the reader came from — their inbox thread, their ATS search — which is none of our
// business and would sit in our database forever.
func referrerHost(referrer string) string {
	if referrer == "" {
		return ""
	}
	u, err := url.Parse(referrer)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
