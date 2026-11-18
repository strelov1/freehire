package handler

import (
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/search"
)

// companyListItemFromRow projects the Postgres read onto the wire shape. The row already carries
// null-ness, so the two nullable columns pass through as-is.
func companyListItemFromRow(r db.ListCompaniesRow) apigen.CompanyListItem {
	return apigen.CompanyListItem{
		Slug:               r.Slug,
		Name:               r.Name,
		JobCount:           int64(r.JobCount),
		Tagline:            nilString(pgconv.TextPtr(r.Tagline)),
		Industries:         r.Industries,
		HqCountry:          nilString(pgconv.TextPtr(r.HqCountry)),
		Collections:        r.Collections,
		FeedbackCount:      int64(r.FeedbackCount),
		FeedbackRatingAvg:  nilFloat32Ptr(pgconv.Float4Ptr(r.FeedbackRatingAvg)),
		AiInterviewReports: optInt64(int64(r.AiInterviewReports)),
	}
}

// companyListItemFromDoc projects a search document onto the same shape. An absent scalar is stored as the empty string and
// must serialize as null like Postgres NULL; an absent array arrives nil and must serialize as [], or the backer marks
// (driven by collections) disappear on search.
func companyListItemFromDoc(d search.CompanyDocument) apigen.CompanyListItem {
	return apigen.CompanyListItem{
		Slug:              d.Slug,
		Name:              d.Name,
		JobCount:          int64(d.JobCount),
		Tagline:           nilString(presentOrNil(d.Tagline)),
		Industries:        orEmpty(d.Industries),
		HqCountry:         nilString(presentOrNil(d.HqCountry)),
		Collections:       orEmpty(d.Collections),
		FeedbackCount:     int64(d.FeedbackCount),
		FeedbackRatingAvg: nilFloat32Ptr(presentOrNilFloat(d.FeedbackRatingAvg)),
	}
}

func nilFloat32Ptr(p *float32) apigen.NilFloat32 {
	if p == nil {
		return apigen.NilFloat32{Null: true}
	}
	return apigen.NewNilFloat32(*p)
}

// presentOrNilFloat treats 0 as absent — CompanyDocument's "no rating" sentinel
// (see its doc comment): a real average is never 0, since ratings are 1-5.
func presentOrNilFloat(f float32) *float32 {
	if f == 0 {
		return nil
	}
	return &f
}

// presentOrNil treats the empty string as absent — the search document's way of spelling NULL.
func presentOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// orEmpty normalizes a nil array so it serializes as [] rather than null.
func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
