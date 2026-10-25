package buddy

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/dict/classify"
)

// Similarity weights. A first guess, not a measurement: category outranks geography
// because a peer in the same line of work is the point of the network, and a shared
// company is worth as much as a shared country. They are handed to the SQL as
// parameters, which is the only place the score is computed.
const (
	weightCategory  = 3
	weightCountries = 2
	weightRegions   = 1
	weightCompany   = 2
	// SharedCompanyCap bounds both the companies that score and the ones listed.
	SharedCompanyCap = 5

	// MaxSimilarLimit is the largest page Similar returns.
	MaxSimilarLimit = 50
	// DefaultSimilarLimit is the page size when the caller asks for none.
	DefaultSimilarLimit = 20
)

// Company is a company two members both applied to.
type Company struct {
	Slug string
	Name string
}

// CardSummary is the one-line view of a member shown in lists. Country is the first of
// the member's stored countries: the full card carries no geography, so this is the one
// place a list reveals it, and a single country keeps it coarse.
type CardSummary struct {
	Category  string
	Seniority string
	Country   string
}

// SimilarMember is one entry of the similar list.
type SimilarMember struct {
	Handle string
	Card   CardSummary
	Score  int
	Shared []Company
}

// SimilarRow is a Repository's raw answer for one member, before the card is projected.
type SimilarRow struct {
	Handle    string
	Countries []string
	CV        resumeextract.Structured
	Score     int
	Shared    []Company
}

// Similar lists the active members most like userID, best first. Members who share
// nothing still appear, after everyone with a positive score. limit is clamped to
// 1..MaxSimilarLimit (zero means the default) and a negative offset reads as 0.
func (s *Service) Similar(ctx context.Context, userID int64, limit, offset int) ([]SimilarMember, error) {
	m, ok, err := s.repo.Member(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ok || !m.Active {
		return nil, ErrNotMember
	}
	rows, err := s.repo.Similar(ctx, userID, clampLimit(limit), max(offset, 0))
	if err != nil {
		return nil, err
	}
	out := make([]SimilarMember, len(rows))
	for i, r := range rows {
		out[i] = SimilarMember{Handle: r.Handle, Card: cardSummary(r), Score: r.Score, Shared: r.Shared}
	}
	return out, nil
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultSimilarLimit
	case limit > MaxSimilarLimit:
		return MaxSimilarLimit
	}
	return limit
}

func cardSummary(r SimilarRow) CardSummary {
	primary := classify.Parse(talentnetwork.PrimaryTitle(r.CV))
	c := CardSummary{Category: primary.Category, Seniority: primary.Seniority}
	if len(r.Countries) > 0 {
		c.Country = r.Countries[0]
	}
	return c
}
