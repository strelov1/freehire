package handler

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tracerlink"
)

// Click limits (design D7): one IP gets at most sponsoredClicksPerMinute clicks across all
// campaigns before further ones stop being charged. A click is never refused.
const (
	sponsoredClicksPerMinute = 20
	// clickTimeout bounds the database work of any click redirect; the redirect itself never waits on more.
	clickTimeout = 2 * time.Second
)

type clickCharger interface {
	Charge(ctx context.Context, campaignID int64, click sponsored.Click, now time.Time) (sponsored.Decision, error)
}

type clickQueries interface {
	GetSponsoredCampaignJobID(ctx context.Context, id int64) (int64, error)
	GetJob(ctx context.Context, id int64) (db.Job, error)
}

// sponsoredClicks serves GET /sponsored/go/:token: it validates and charges the click, then
// always redirects (302) to the job's own destination, never to anything the request names.
type sponsoredClicks struct {
	key     []byte
	charger clickCharger
	queries clickQueries
	limiter *sponsored.IPLimiter
	origin  string // frontend origin: the jobs page and freehire-hosted jobs live there
	now     func() time.Time
}

func newSponsoredClicks(key []byte, c clickCharger, q clickQueries, origin string) *sponsoredClicks {
	return &sponsoredClicks{key: key, charger: c, queries: q, origin: strings.TrimRight(origin, "/"),
		limiter: sponsored.NewIPLimiter(sponsoredClicksPerMinute, time.Minute), now: time.Now}
}

func (s *sponsoredClicks) register(api fiber.Router) {
	api.Get("/sponsored/go/:token", s.handle)
}

func (s *sponsoredClicks) redirect(c *fiber.Ctx, to string) error {
	return redirectOut(c, to)
}

func (s *sponsoredClicks) handle(c *fiber.Ctx) error {
	now := s.now()
	jobsPage := s.origin + "/jobs"
	claims, err := sponsored.VerifyToken(s.key, c.Params("token"), now)
	if err != nil {
		return s.redirect(c, jobsPage)
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), clickTimeout)
	defer cancel()

	row, err := s.queries.GetJob(ctx, claims.JobID)
	if err != nil {
		return s.redirect(c, jobsPage)
	}
	view, err := jobview.FromRow(row)
	if err != nil {
		return s.redirect(c, jobsPage)
	}
	// A freehire-hosted job has no outside destination: its Apply lives on its own page.
	target := outboundurl.Tag(view.URL)
	if target == "" {
		target = s.origin + "/jobs/" + view.PublicSlug
	}

	// A closed job costs nothing even before the hourly sweep ends its campaign.
	if row.ClosedAt.Valid {
		return s.redirect(c, target)
	}
	if jobID, err := s.queries.GetSponsoredCampaignJobID(ctx, claims.CampaignID); err != nil || jobID != claims.JobID {
		return s.redirect(c, target)
	}
	// Fiber also routes HEAD here; mail scanners and link previews probe with it. Not a click.
	if c.Method() != fiber.MethodGet {
		return s.redirect(c, target)
	}
	ip, ua := c.IP(), c.Get(fiber.HeaderUserAgent)
	click := sponsored.Click{
		Placement:       claims.Placement,
		VisitorHash:     sponsored.VisitorHash(s.key, ip, ua, now),
		PrevVisitorHash: sponsored.VisitorHash(s.key, ip, ua, now.Add(-sponsored.DuplicateWindow)),
		Bot:             ua == "" || tracerlink.IsBotUserAgent(ua),
		RateLimited:     !s.limiter.Allow(ip, now),
	}
	if _, err := s.charger.Charge(ctx, claims.CampaignID, click, now); err != nil && !errors.Is(err, context.Canceled) {
		// The seeker still gets to the job.
		log.Printf("sponsored: charge campaign %d: %v", claims.CampaignID, err)
	}
	return s.redirect(c, target)
}
