package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tracerlink"
)

// ownedCVPDF renders a CV its owner is sending somewhere: the template, the headshot a
// photo-bearing template prints, and — when the owner turned tracing on — tracer links in
// place of their own. The CV page's download and an on-site application (direct_apply.go)
// both send a CV to someone, so both render it through this one path; it is built once in
// Register, like every service two features share.
type ownedCVPDF struct {
	store    *cv.Store
	renderer cv.Renderer
	photos   *headshot.Store
	// tracer issues the tokens a traced render substitutes in; nil disables tracing.
	tracer *tracerlink.Minter
	// tracerBaseURL is the public origin a traced link points at, e.g. "https://freehire.me".
	tracerBaseURL string
}

func newOwnedCVPDF(queries *db.Queries, store *cv.Store, renderer cv.Renderer, photos *headshot.Store, baseURL string, servedHosts []string) ownedCVPDF {
	return ownedCVPDF{
		store:    store,
		renderer: renderer,
		photos:   photos,
		tracer: tracerlink.NewMinter(tracerlink.NewRepository(
			func(ctx context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error) {
				return queries.UpsertTracerLink(ctx, db.UpsertTracerLinkParams{
					CvID: cvID, UserID: userID, Token: token, SourcePath: sourcePath,
					DestinationUrl: destURL, DestinationHash: destHash,
				})
			}), servedHosts),
		tracerBaseURL: baseURL,
	}
}

// render renders userID's CV id. cv.ErrNotFound when they own no such CV. The caller checks
// that a renderer is configured: with none, this would dereference nil.
func (p ownedCVPDF) render(ctx context.Context, userID int64, id uuid.UUID) ([]byte, error) {
	rec, err := p.store.Get(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	return p.renderRecord(ctx, userID, rec)
}

// renderRecord renders a CV record its caller has already loaded for userID.
func (p ownedCVPDF) renderRecord(ctx context.Context, userID int64, rec cv.Record) ([]byte, error) {
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return nil, err
	}
	return p.renderer.Render(ctx, rec.Document, tmpl, headshotForTemplate(ctx, p.photos, userID, tmpl), p.tracedHrefs(ctx, rec, userID))
}

// tracedHrefs mints a token for each of this CV's traceable links and returns where each should
// point, or nothing at all when the candidate has not asked for tracing.
//
// It runs on every download because the PDF is never stored, so the minting underneath must be
// idempotent — otherwise three downloads would leave three tokens for one link and scatter the
// counts across them.
//
// A zero value is the ordinary render: the links still resolve absolutely, they just point where
// the candidate wrote them.
func (p ownedCVPDF) tracedHrefs(ctx context.Context, rec cv.Record, userID int64) cv.LinkHrefs {
	if !rec.TracerLinksEnabled || p.tracer == nil || p.tracerBaseURL == "" {
		return cv.LinkHrefs{}
	}
	projectLinks := make([]string, len(rec.Document.Projects))
	for i, p := range rec.Document.Projects {
		projectLinks[i] = p.Link
	}
	minted := p.tracer.Mint(ctx, rec.ID, userID, p.tracerBaseURL,
		rec.CompanySlug, rec.Document.Header.Links, projectLinks)
	return cv.LinkHrefs{Header: minted.Header, Projects: minted.Projects}
}
