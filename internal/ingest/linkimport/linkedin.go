package linkimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/ingestsched"
	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// LinkedInResolution separates reading a discovery from writing it. Intake inspects
// the destination board before the write, preserving contribution novelty checks.
type LinkedInResolution struct {
	Posting       sources.Job
	Native        *NativeDestination
	VerifiedBoard *sources.CompanyEntry
	Board         Board
	BoardURL      string
}

func linkedInURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com")
}

// PrepareLinkedIn never falls back to generic JSON-LD. Only explicit internal
// applications may become LinkedIn jobs; other discoveries need their native ATS.
func (im *Importer) PrepareLinkedIn(ctx context.Context, raw string) (LinkedInResolution, error) {
	var out LinkedInResolution
	id := sources.LinkedInPostingID(raw)
	if id == "" {
		return out, fmt.Errorf("invalid LinkedIn posting URL")
	}
	src := im.ingest["linkedin"]
	if src == nil {
		src = im.linkedin
	}
	reader, ok := src.(interface {
		Posting(context.Context, string) (sources.Job, error)
	})
	if !ok {
		return out, fmt.Errorf("LinkedIn posting reader unavailable")
	}
	j, err := reader.Posting(ctx, raw)
	if err != nil {
		return out, err
	}
	if j.ExternalID != id || j.Removed || j.Unreadable || strings.TrimSpace(j.Title) == "" || strings.TrimSpace(j.Company) == "" || strings.TrimSpace(j.Description) == "" {
		return out, fmt.Errorf("LinkedIn did not return the requested live posting")
	}
	j.URL = "https://www.linkedin.com/jobs/view/" + id + "/"
	out.Posting = j
	if j.ApplyMethod == sources.ApplyOnSite {
		return out, nil
	}
	if j.ExternalApplyURL != "" {
		if source, board, canonical, found := atsboard.Recognize(j.ExternalApplyURL); found {
			out.Board, out.BoardURL = Board{Source: source, Board: board}, canonical
		}
		native, err := im.ResolveNative(ctx, db.Job{Source: "linkedin", Title: j.Title, Company: j.Company, URL: j.URL}, j.ExternalApplyURL, j.CompanyWebsite)
		if native.Entry.Provider != "" {
			out.VerifiedBoard = &native.Entry
			out.Board = Board{Source: native.Entry.Provider, Board: native.Entry.Board}
			if out.BoardURL == "" {
				out.BoardURL = j.ExternalApplyURL
			}
		}
		if err != nil {
			return out, err
		}
		out.Native = &native
		out.Board = Board{Source: native.Entry.Provider, Board: native.Entry.Board}
		return out, nil
	}
	board, canonical, err := im.LinkedInEmployerBoard(ctx, j)
	out.Board, out.BoardURL = board, canonical
	return out, err
}

// LinkedInEmployerBoard discovers observed employer career links without inventing
// a posting or fetching an entire ATS board. It is shared by scheduled discovery
// and on-demand intake; provider pause checks remain in the ordinary write path.
func (im *Importer) LinkedInEmployerBoard(ctx context.Context, j sources.Job) (Board, string, error) {
	website := j.CompanyWebsite
	if website == "" && im.q != nil {
		company, err := im.q.GetCompany(ctx, normalize.CompanySlug(j.Company))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Board{}, "", err
		}
		if err == nil {
			var info struct {
				Website string `json:"website"`
			}
			if json.Unmarshal(company.CompanyInfo, &info) == nil {
				website = info.Website
			}
		}
	}
	if website == "" && j.CompanyProfileURL != "" {
		src := im.ingest["linkedin"]
		if src == nil {
			src = im.linkedin
		}
		if reader, ok := src.(interface {
			CompanyWebsite(context.Context, string) (string, error)
		}); ok {
			var err error
			website, err = reader.CompanyWebsite(ctx, j.CompanyProfileURL)
			if err != nil {
				return Board{}, "", err
			}
		}
	}
	resolver, ok := im.boards.(interface {
		ResolveCompany(context.Context, string) (string, string, string, bool)
	})
	if website == "" || !ok {
		return Board{}, "", nil
	}
	source, board, canonical, found := resolver.ResolveCompany(ctx, website)
	if !found || im.ingest[source] == nil || sources.ProviderKind(im.ingest, source) != "ats" {
		return Board{}, "", nil
	}
	return Board{Source: source, Board: board}, canonical, nil
}

func (im *Importer) WriteLinkedIn(ctx context.Context, prepared LinkedInResolution) (Result, bool, error) {
	if prepared.Native != nil {
		res, err := im.WriteNative(ctx, nil, *prepared.Native)
		return res, err == nil, err
	}
	if prepared.Posting.ApplyMethod != sources.ApplyOnSite {
		if err := im.AdoptLinkedInBoard(ctx, prepared.Board); err != nil {
			return Result{}, false, err
		}
		return Result{}, false, nil
	}
	return im.Write(ctx, linksource.Resolved{Source: "linkedin", Job: prepared.Posting})
}

// AdoptLinkedInBoard schedules a recorded destination independently of whether
// its exact posting could be read. Existing operator pauses remain unchanged.
func (im *Importer) AdoptLinkedInBoard(ctx context.Context, board Board) error {
	if !board.known() || im.catalog == nil {
		return nil
	}
	_, found, err := im.catalog.Entry(ctx, board.Source, board.Board)
	if err != nil || !found {
		return err
	}
	return ingestsched.NewQueriesRepository(im.q).AdoptDiscoveredProvider(ctx, board.Source)
}
