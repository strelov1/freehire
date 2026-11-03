package handler

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// ReseedCV rebuilds one owned CV's content from the current seed (experience bank +
// structured extract — the same source first-time tailor uses, and the bank first
// whenever it has rows; see cv_seed.go) and keeps its id (and, for a tailored copy, its
// agent session). It touches ONLY the CV it was asked about: a tailored copy is refreshed
// without writing any base, and a base is refreshed without touching its copies. Pointed at a
// base it is the candidate's explicit "update from my CV", so it replaces hand edits — the
// confirmation is the client's. Cookie-only: destructive whole-document replace.
//
// Upload does not do this. Upload refreshes the seed source; this is the explicit apply.
func (h *cvHandlers) ReseedCV(c *fiber.Ctx) error {
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

	st, ok, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok || !hasSeedBody(st) {
		// ok alone (from StructureForSeed/seedable) is satisfied by identity fields
		// alone — candidate-owned contacts with no résumé ever uploaded and no bank
		// rows. This is a destructive whole-document replace of an EXISTING CV, unlike
		// first-time tailor bootstrap: identity alone is not reason enough to wipe it.
		return fiber.NewError(fiber.StatusConflict, "add a résumé before reseeding")
	}
	seeded := cv.Seed(st)

	if !rec.IsTailored {
		// An explicit refresh of this base. Presentation is preserved by applySeedContent.
		if err := h.applySeedToBase(c.Context(), userID, rec, seeded); err != nil {
			return mapCVError(err)
		}
		out, err := h.cvStore.Get(c.Context(), id, userID)
		if err != nil {
			return mapCVError(err)
		}
		return c.JSON(fiber.Map{"data": recordResponse(out)})
	}

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

	_, _, err = h.editor.CommitDocument(c.Context(), id, userID,
		cvedit.ActorCandidate, cvedit.OriginImport, next)
	if err != nil {
		return mapCVError(err)
	}

	out, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": recordResponse(out)})
}

// ReseedBaseCV rebuilds the owner's DEFAULT base CV from the current seed (experience bank +
// structured extract). Cookie-only. Does not touch tailored copies — those stay on
// History → Reset (or the tailor-workspace refresh prompt). It is an explicit action, so it
// replaces hand edits; POST /me/cvs/:id/reseed does the same for one named base.
func (h *cvHandlers) ReseedBaseCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	st, ok, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok || !hasSeedBody(st) {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before reseeding")
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
	return mapCVError(h.applySeedToBase(c.Context(), userID, base, seeded))
}

// applySeedToBase writes seeded content onto an existing base, keeping its title, template and
// presentation.
func (h *cvHandlers) applySeedToBase(ctx context.Context, userID int64, base cv.Record, seeded cv.Document) error {
	_, _, err := h.editor.CommitDocument(ctx, base.ID, userID,
		cvedit.ActorCandidate, cvedit.OriginImport,
		applySeedContent(cvedit.State{
			Title:      base.Title,
			TemplateID: base.TemplateID,
			Document:   base.Document,
		}, seeded))
	return err
}

// reseedBaseIfStaleVsUpload refreshes a base from bankedSeeder when it predates the caller's
// current résumé upload; baseID names the base about to be tailored from, uuid.Nil the default.
// No-op when there is no such base, no upload stamp, the base was edited at/after the upload,
// the seed is unusable — or the candidate has edited the base by hand: an upload must never
// silently erase that. (The explicit "update from my CV" is ReseedCV on the base.)
// Current structure: full reseed. Pending extract: header heal only (see AGENTS.md).
func (h *cvHandlers) reseedBaseIfStaleVsUpload(c *fiber.Ctx, userID int64, baseID uuid.UUID) error {
	if h.resume == nil {
		return nil
	}
	uploadedAt, err := h.resume.UploadedAt(c.Context(), userID)
	if err != nil || uploadedAt == nil {
		return err
	}
	base, ok, err := h.cvStore.BaseOrDefault(c.Context(), userID, baseID)
	if err != nil || !ok {
		return err
	}
	if base.HandEdited || !base.UpdatedAt.Before(*uploadedAt) {
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
	return h.applySeedToBase(c.Context(), userID, base, cv.Seed(st))
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
