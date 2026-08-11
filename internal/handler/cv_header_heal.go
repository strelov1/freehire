package handler

import (
	"context"
	"log"
	"reflect"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

// resumeContactHeader returns contact fields preferring candidate-owned contacts, then
// a current structured résumé, then provisional contacts while extract is pending.
func (h *cvHandlers) resumeContactHeader(ctx context.Context, userID int64) (cv.Header, bool, error) {
	if h.resume == nil {
		return cv.Header{}, false, nil
	}
	if owned, err := h.resume.CandidateContacts(ctx, userID); err != nil {
		return cv.Header{}, false, err
	} else if !owned.Empty() {
		hdr := contactHeaderFromStructured(owned.AsStructured())
		return hdr, !contactHeaderEmpty(hdr), nil
	}
	if st, ok, err := h.resume.Structured(ctx, userID); err != nil {
		return cv.Header{}, false, err
	} else if ok {
		hdr := contactHeaderFromStructured(st)
		return hdr, !contactHeaderEmpty(hdr), nil
	}
	contacts, ok, err := h.resume.ProvisionalContacts(ctx, userID)
	if err != nil || !ok {
		return cv.Header{}, false, err
	}
	hdr := contactHeaderFromStructured(contacts)
	return hdr, !contactHeaderEmpty(hdr), nil
}

func contactHeaderEmpty(h cv.Header) bool {
	return h.FullName == "" && h.Email == "" && h.Phone == "" && h.Location == "" && len(h.Links) == 0
}

// healRecordHeader fills empty header contact fields from résumé identity (owned,
// else current, else provisional) and persists when the merge changes anything.
//
// Owner GET of a CV may write here. That is deliberate: tailored copies minted
// before contacts existed would otherwise reopen with a blank name forever. The
// fill is keep-first (never overwrite a typed field), body/template/typography
// stay put, and a second GET is a no-op. List and PDF do not heal. See
// internal/resume/AGENTS.md.
func (h *cvHandlers) healRecordHeader(ctx context.Context, userID int64, rec cv.Record) (cv.Record, error) {
	if h.editor == nil {
		return rec, nil
	}
	seedHdr, ok, err := h.resumeContactHeader(ctx, userID)
	if err != nil || !ok {
		return rec, err
	}
	merged := fillEmptyHeaderFields(rec.Document.Header, seedHdr)
	if reflect.DeepEqual(merged, rec.Document.Header) {
		return rec, nil
	}
	doc := rec.Document
	doc.Header = merged
	if _, _, err := h.editor.CommitDocument(ctx, rec.ID, userID,
		cvedit.ActorCandidate, cvedit.OriginImport,
		cvedit.State{Title: rec.Title, TemplateID: rec.TemplateID, Document: doc}); err != nil {
		return rec, err
	}
	out, err := h.cvStore.Get(ctx, rec.ID, userID)
	if err != nil {
		return rec, err
	}
	return out, nil
}

// fillEmptyHeaderFields copies résumé identity into blank fields on an existing CV.
// Non-empty fields on keep stay put — heal must not overwrite a name the candidate
// already wrote. Contrast mergeSeedHeader, which is seed-first for reset-from-résumé.
func fillEmptyHeaderFields(keep, seed cv.Header) cv.Header {
	out := keep
	if out.FullName == "" {
		out.FullName = seed.FullName
	}
	if out.Email == "" {
		out.Email = seed.Email
	}
	if out.Phone == "" {
		out.Phone = seed.Phone
	}
	if out.Location == "" {
		out.Location = seed.Location
	}
	if len(out.Links) == 0 {
		out.Links = seed.Links
	}
	return out
}

// healBaseHeaderIfNeeded fills an empty/partial base CV header from résumé contacts so
// the next tailored copy does not reintroduce a blank header.
func (h *cvHandlers) healBaseHeaderIfNeeded(ctx context.Context, userID int64) {
	base, ok, err := h.cvStore.BaseCV(ctx, userID)
	if err != nil || !ok {
		return
	}
	if _, err := h.healRecordHeader(ctx, userID, base); err != nil {
		log.Printf("cv: healing base header for user %d: %v", userID, err)
	}
}
