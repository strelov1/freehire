// The public side of CV link tracing: the redirect a traced link in a downloaded PDF points at.
// It is the only unauthenticated surface of the feature, and the only one a stranger ever sees.
package handler

import (
	"context"
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
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
const goneBody = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>This link is no longer active</title>
<style>
:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:grid;place-items:center;padding:2rem;
font:16px/1.6 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
color:#18181b;background:#fafafa}
main{max-width:32rem;text-align:center}
h1{margin:0 0 .5rem;font-size:1.375rem;font-weight:600;letter-spacing:-.01em}
p{margin:0;color:#52525b}
a{display:inline-block;margin-top:1.5rem;color:#3f3f46;font-size:.875rem}
@media(prefers-color-scheme:dark){body{color:#fafafa;background:#09090b}p{color:#a1a1aa}a{color:#d4d4d8}}
</style></head><body><main>
<h1>This link is no longer active</h1>
<p>The CV it pointed to has been removed by its owner.</p>
<a href="https://freehire.me">freehire.me</a>
</main></body></html>`

// Redirect resolves a traced link, records the click, and sends the visitor on.
//
// Two properties hold whatever else changes here. The destination comes only from the stored
// token — no query parameter, path remainder or header can name one — so the endpoint cannot be
// aimed by whoever crafts the URL. And the click write is best-effort: the redirect happens
// whether or not it succeeds, because a broken redirect lives inside a PDF the candidate can
// neither see nor fix.
func (h *tracerHandlers) Redirect(c *fiber.Ctx) error {
	link, err := h.links.TracerLinkByToken(c.Context(), c.Params("token"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusGone).Type("html").SendString(goneBody)
	}
	if err != nil {
		// Only a token that genuinely is not there earns the 410. A pool timeout or a failover
		// blip must not tell a recruiter the candidate deleted their CV — that is a false
		// statement about a person, and 410 means "gone for good", so a well-behaved gateway
		// would stop retrying a link that is perfectly alive.
		return err
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
