package sponsored

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// DefaultMinCPCCents is the smallest cost per click offered when SPONSORED_MIN_CPC_CENTS is unset.
const DefaultMinCPCCents = 10

var (
	// ErrInvalid is a campaign setting the buyer must change; its message is written for them.
	ErrInvalid = errors.New("sponsored: invalid campaign")
	// ErrJobUnavailable is a job that does not exist, is closed, or is private: it cannot be promoted.
	ErrJobUnavailable = errors.New("sponsored: job cannot be promoted")
)

// MinCPCFromEnv reads SPONSORED_MIN_CPC_CENTS (default DefaultMinCPCCents).
func MinCPCFromEnv() int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SPONSORED_MIN_CPC_CENTS")), 10, 64)
	if err != nil || n <= 0 {
		return DefaultMinCPCCents
	}
	return n
}

// WithMinCPC sets the smallest cost per click Create accepts.
func (s *Service) WithMinCPC(cents int64) *Service {
	s.minCPC = cents
	return s
}

// Create opens a draft campaign for an open, public job named by its public slug. Nothing is
// charged: the buyer pays in through a top-up.
func (s *Service) Create(ctx context.Context, ownerID int64, jobSlug string, cpcCents int64, dailyCapCents *int64) (db.SponsoredCampaign, error) {
	q := db.New(s.pool)
	minCPC := cmpDefault(s.minCPC, DefaultMinCPCCents)
	switch {
	case cpcCents < minCPC:
		return db.SponsoredCampaign{}, fmt.Errorf("%w: the cost per click is at least %d cents", ErrInvalid, minCPC)
	case dailyCapCents != nil && *dailyCapCents < cpcCents:
		return db.SponsoredCampaign{}, fmt.Errorf("%w: the daily cap must cover at least one click (%d cents)", ErrInvalid, cpcCents)
	}
	job, err := q.GetJobBySlug(ctx, jobSlug)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (job.ClosedAt.Valid || job.IsPrivate)) {
		return db.SponsoredCampaign{}, ErrJobUnavailable
	}
	if err != nil {
		return db.SponsoredCampaign{}, err
	}
	return q.CreateSponsoredCampaign(ctx, db.CreateSponsoredCampaignParams{
		JobID: job.ID, OwnerUserID: ownerID, CpcCents: cpcCents, DailyCapCents: pgconv.Int8(dailyCapCents),
	})
}

func cmpDefault(v, def int64) int64 {
	if v > 0 {
		return v
	}
	return def
}
