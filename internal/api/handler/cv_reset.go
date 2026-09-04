package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// ResetCVFromResume rebuilds a tailored CV's content from the current résumé seed
// (experience bank + structured extract — the same source first-time tailor uses), keeps
// the same CV id and agent session, and refreshes the base CV from that same seed so ATS
// delta and future bootstraps stay aligned. Cookie-only: destructive whole-document replace.
//
// Upload does not do this. Upload refreshes the seed source; this is the explicit apply.
func (h *cvHandlers) ResetCVFromResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}

	// Ownership before seed: a foreign id must 404 even when the caller has no résumé
	// (otherwise "add a résumé" would leak that the CV exists).
	rec, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	if !rec.IsTailored {
		return fiber.NewError(fiber.StatusConflict, "reset from résumé is only for a tailored CV")
	}

	st, ok, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok || !hasSeedBody(st) {
		// ok alone (from StructureForSeed/seedable) is satisfied by identity fields
		// alone — candidate-owned contacts with no résumé ever uploaded and no bank
		// rows. This is a destructive whole-document replace of an EXISTING CV, unlike
		// first-time tailor bootstrap: identity alone is not reason enough to wipe it.
		return fiber.NewError(fiber.StatusConflict, "add a résumé before resetting from it")
	}
	seeded := cv.Seed(st)

	// Align only the tailored copy to the vacancy. The base stays on the résumé's own
	// spellings — a vacancy's wording belongs to the copy aimed at it, not to the
	// candidate's seed.
	//
	// Alignment runs on the MERGED state, not on the seed: applySeedContent keeps the
	// summary and skills already on the page when the seed carries none, and aligning
	// beforehand would leave exactly those kept sections in the old wording.
	next := applySeedContent(cvedit.State{
		Title:      rec.Title,
		TemplateID: rec.TemplateID,
		Document:   rec.Document,
	}, seeded)
	if rec.JobID != 0 {
		if job, jerr := h.queries.GetJob(c.Context(), rec.JobID); jerr == nil {
			next.Document, _ = cv.Align(next.Document, skilltag.PreferredFromText(job.Description))
		} else {
			log.Printf("cv: loading job %d for reset surface-align: %v", rec.JobID, jerr)
		}
	}

	// The requested target commits first: if the tailored copy is what the seed cannot
	// satisfy (e.g. a role over the bullet cap refuses the whole-document write), nothing
	// is written at all. Refreshing the base second means a failure there after the target
	// already succeeded leaves the base merely stale — the same state a plain page reload
	// would find it in before this call — rather than a request that "failed" while having
	// silently rewritten a CV the caller did not ask to touch.
	_, _, err = h.editor.CommitDocument(c.Context(), id, userID,
		cvedit.ActorCandidate, cvedit.OriginImport, next)
	if err != nil {
		return mapCVError(err)
	}

	if err := h.reseedBaseFromSeed(c, userID, seeded); err != nil {
		return err
	}

	out, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": recordResponse(out)})
}

// ResetBaseCVFromResume rebuilds the owner's base CV from the current résumé seed
// (experience bank + structured extract). Cookie-only. Does not touch tailored copies —
// those stay on History → Reset (or the tailor-workspace refresh prompt).
func (h *cvHandlers) ResetBaseCVFromResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	st, ok, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok || !hasSeedBody(st) {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before resetting from it")
	}
	if err := h.reseedBaseFromSeed(c, userID, cv.Seed(st)); err != nil {
		return err
	}
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil {
		return mapCVError(err)
	}
	if !ok {
		return fiber.NewError(fiber.StatusInternalServerError, "base CV was not created")
	}
	return c.JSON(fiber.Map{"data": recordResponse(base)})
}

// reseedBaseFromSeed refreshes the current base CV from seeded content, or creates one when
// the user has none. Presentation on an existing base is preserved.
func (h *cvHandlers) reseedBaseFromSeed(c *fiber.Ctx, userID int64, seeded cv.Document) error {
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok {
		defaults, _, err := h.cvStore.GetAppearanceDefaults(c.Context(), userID)
		if err != nil {
			return err
		}
		seeded.Style = defaults.Style
		seeded.Margins = defaults.Margins
		meta, err := h.cvStore.Create(c.Context(), userID, "My CV", defaults.TemplateID, seeded)
		if err != nil {
			return err
		}
		// Best-effort history milestone — same pattern as TailorCV opening a tailored copy.
		if _, err := h.editor.Seed(c.Context(), meta.ID, userID, "Created from your résumé"); err != nil {
			log.Printf("cv: seeding the revision history for base %s: %v", meta.ID, err)
		}
		return nil
	}
	_, _, err = h.editor.CommitDocument(c.Context(), base.ID, userID,
		cvedit.ActorCandidate, cvedit.OriginImport,
		applySeedContent(cvedit.State{
			Title:      base.Title,
			TemplateID: base.TemplateID,
			Document:   base.Document,
		}, seeded))
	return mapCVError(err)
}

// reseedBaseIfStaleVsUpload refreshes the base from bankedSeeder when it predates the
// caller's current résumé upload. No-op when there is no base, no upload stamp, the base
// was edited at/after the upload, or the seed is unusable.
// Current structure: full reseed. Pending extract: header heal only (see AGENTS.md).
func (h *cvHandlers) reseedBaseIfStaleVsUpload(c *fiber.Ctx, userID int64) error {
	if h.resume == nil {
		return nil
	}
	uploadedAt, err := h.resume.UploadedAt(c.Context(), userID)
	if err != nil || uploadedAt == nil {
		return err
	}
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil || !ok {
		return err
	}
	if !base.UpdatedAt.Before(*uploadedAt) {
		return nil
	}
	st, usable, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil || !usable {
		return err
	}
	// A current structure refreshes the whole base from the seed. Provisional-only
	// identity (pending extract) only heals the header — a full Seed would blank
	// summary/education the candidate already has on the base. Same for a seed that
	// is technically current/usable but carries identity alone: not enough reason to
	// wipe an existing base's body.
	if _, current, cerr := h.resume.Structured(c.Context(), userID); cerr != nil {
		return cerr
	} else if !current {
		_, err := h.healRecordHeader(c.Context(), userID, base)
		return err
	}
	if !hasSeedBody(st) {
		_, err := h.healRecordHeader(c.Context(), userID, base)
		return err
	}
	return h.reseedBaseFromSeed(c, userID, cv.Seed(st))
}

// hasSeedBody reports whether a composed seed carries body content — experience,
// education, skills, and so on — rather than identity alone. seedable() (cv_seed.go)
// treats FullName alone as enough reason to seed a brand-new, empty CV; a destructive
// whole-document replace of an EXISTING CV needs a higher bar.
func hasSeedBody(st resumeextract.Structured) bool {
	return len(st.Experience) > 0 || len(st.Education) > 0 || len(st.Skills) > 0 ||
		len(st.Languages) > 0 || len(st.Projects) > 0 || len(st.Certifications) > 0 ||
		st.Summary != "" || st.Headline != ""
}
