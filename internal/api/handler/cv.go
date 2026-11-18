package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/db"
)

// CV-builder HTTP surface: per-user structured CVs (CRUD + seed) and on-demand PDF
// rendering. Whole-document authoring (create, replace, delete) and the scoring/undo
// surface are cookie-only (RequireAuth); everything the tailoring cycle itself needs —
// list, read, render, field-level patch, context, and the bootstrap that creates the
// vacancy-bound copy — also accepts an API key (RequireAuthOrKey), so a CLI can run the
// whole cycle rather than half of it. All routes are open to every signed-in user (the
// beta gate was lifted when tailoring went public; the plan meters the LLM spend). Every
// operation is owner-scoped — a foreign id is a 404, never a leak.

// cvHandlers serves the CV builder + AI tailoring routes. The renderer is nil when no
// typst binary is configured; the PDF endpoint then returns 501 while the rest of the
// builder still works. match links the tailoring flow to the fit-analysis helpers
// (blocker capping, plan standing) it reuses.
type cvHandlers struct {
	// cvStore owns the CV-builder use cases (per-user structured CVs, CRUD + seed).
	cvStore    *cv.Store
	cvRenderer cv.Renderer
	// tracerSalt keys the visitor hash of a traced click. Empty means the deployment cannot
	// identify visitors at all, which is why enabling tracing is refused without it.
	tracerSalt string
	// pdf renders an owned CV the way it is sent — shared with an on-site application.
	pdf ownedCVPDF
	// tracerBaseURL is the public origin a traced link points at, e.g. "https://freehire.me".
	tracerBaseURL string
	resume        *resume.Store
	// photos serves the headshot the photo-bearing templates print. Nil-safe: an
	// unconfigured bucket, like a member with no photo, means the placeholder.
	photos  *headshot.Store
	queries *db.Queries
	plans   *plan.Store
	// fit reads the cached fit analysis the tailoring context and the CV-vs-job score
	// ground on. Reads only, from this surface: the writes, the staleness rule and the
	// metering are the same service's and belong to the match surface.
	fit *fitanalysis.Service
	// jobReader serves the vacancy a tailoring context is about. Narrow on purpose: the
	// tailoring path reads one row by id and nothing else.
	jobReader jobReader
	match     *matchHandlers
	// assistantSessions mints the conversation a tailoring workspace runs in.
	assistantSessions *assistant.Store
	// seeder answers what a new CV starts from: the banked work history plus the sections
	// the stored structure still owns.
	seeder cv.Seeder
	// extractPDFText reads a rendered CV's text layer the way an ATS parser would. A field
	// rather than a direct call so a test can state the text layer without a poppler binary.
	extractPDFText func([]byte) (string, error)
	// editor is the only path that writes a stored CV. Every entry point — the editor's
	// autosave, the template picker, the CLI's patch, an agent tool, seeding a tailored
	// copy — commits through it, so no change happens without a revision recording it.
	editor *cvedit.Editor
	// jobs puts a vacancy on the Tracking Kanban when the caller starts (or reopens)
	// tailoring for it. Nil-safe for fixtures that never assert board membership.
	jobs jobBoarder
	// letters, letterChain and bank serve the cover-letter surface. Wired after construction
	// rather than through newCVHandlers, whose signature 78 integration tests already call —
	// widening it for three optional dependencies would edit every one of them to say nil.
	// All three are nil-safe: a fixture that never asks for a letter needs none of them.
	// letter holds the cover-letter surface's dependencies as one value: the store, the chain,
	// and the experience bank under the two narrow interfaces a letter needs of it.
	letter coverLetterDeps
	// llm binds a model call to the caller's own gateway credential, tagged by feature.
	llm llmBinding
}

// jobReader is the one vacancy read the tailoring context needs.
type jobReader interface {
	GetJob(ctx context.Context, id int64) (db.Job, error)
}

// jobBoarder places a vacancy on the caller's Tracking board. The Kanban only shows
// staged (or applied) rows — a bare bookmark lives under Activity → Saved — so this is
// save + stage=preparing when no stage exists yet. applied_at stays unset: preparing a
// CV is not submitting an application, and silence must not start. preparing has its
// own Kanban column and auto-promotes to applied the moment a real apply signal fires
// (see MarkJobApplied) — it is never mistaken for a submitted application, only for a
// candidate's own manual, undated "Applied" drag, the ambiguity this stage exists to
// resolve (see internal/application/userjob/AGENTS.md).
type jobBoarder interface {
	EnsureOnBoard(ctx context.Context, userID, jobID int64) error
}

// trackingBoarder adapts jobtracking's repository to jobBoarder.
type trackingBoarder struct {
	repo interface {
		SaveJob(ctx context.Context, userID, jobID int64) (jobtracking.Interaction, error)
		TrackJob(ctx context.Context, userID, jobID int64, stage, notes *string, source string) (jobtracking.Interaction, error)
	}
}

func (s trackingBoarder) EnsureOnBoard(ctx context.Context, userID, jobID int64) error {
	row, err := s.repo.SaveJob(ctx, userID, jobID)
	if err != nil {
		return err
	}
	if row.Stage != nil && *row.Stage != "" {
		return nil
	}
	stage := "preparing"
	_, err = s.repo.TrackJob(ctx, userID, jobID, &stage, nil, appevent.SourceSystem)
	return err
}

// refuseListCap is normally true (Commit refuses over-cap edits). Pass false only when
// an operator has turned on CV_EDIT_ALLOW_BULLET_TRUNCATION.
func newCVHandlers(pool *pgxpool.Pool, queries *db.Queries, cvStore *cv.Store, assistantSessions *assistant.Store, cvRenderer cv.Renderer, pdf ownedCVPDF, tracerSalt, baseURL string, resumeStore *resume.Store, photoStore *headshot.Store, plans *plan.Store, match *matchHandlers, gate cvedit.EvidenceGate, jobs jobBoarder, letter coverLetterDeps, refuseListCap bool) *cvHandlers {
	h := &cvHandlers{
		letter:            letter,
		cvStore:           cvStore,
		assistantSessions: assistantSessions,
		cvRenderer:        cvRenderer,
		tracerSalt:        tracerSalt,
		pdf:               pdf,
		tracerBaseURL:     baseURL,
		// The editor is the only thing that writes a stored CV, and the gate is what
		// refuses an agent's uncited claim. It is a constructor argument rather than
		// something attached afterwards: PATCH /me/cvs/:id edits as the agent for any
		// API-key caller, so the wall cannot depend on which other features the assembly
		// built.
		editor:         cvedit.NewEditor(cvedit.NewRepository(pool, queries), gate).SetRefuseListCap(refuseListCap),
		jobReader:      queries,
		resume:         resumeStore,
		photos:         photoStore,
		seeder:         bankedSeeder{resume: resumeStore, bank: newWorkHistoryReader(queries)},
		queries:        queries,
		plans:          plans,
		fit:            match.fit,
		match:          match,
		jobs:           jobs,
		extractPDFText: resume.ExtractPDFText,
	}
	return h
}

// seedSource returns what a new CV starts from, and is never nil.
//
// A handler assembled without a seeder still seeds from the stored structure. "There is
// nothing to seed from" and "the seeder was not wired" are different statements:
// collapsing them hands a client that asked for a pre-filled CV a blank one, and passing
// the nil interface down to cv.Store.Tailor panics on the first tailoring bootstrap.
func (h *cvHandlers) seedSource() cv.Seeder {
	if h.seeder != nil {
		return h.seeder
	}
	return bankedSeeder{resume: h.resume}
}

func (h *cvHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// CV builder + AI tailoring: open to every signed-in user (the plan meters the LLM spend).
	// Cookie-only, owner-scoped (a foreign id is a 404). The PDF endpoint 501s when no typst
	// binary is configured; the rest still works.
	api.Get("/cv-templates", mw.cookie, contract)
	api.Get("/cv-fonts", mw.cookie, contract)
	// The candidate's personal defaults for a NEW CV's template/typography/margins. Cookie-only:
	// a personal setting, not something the in-app assistant needs to touch. A flat path
	// (not nested under /me/cvs/:id) — this is not a CV, it never collides with the :id routes.
	api.Get("/me/cv-appearance-defaults", mw.cookie, contract)
	api.Put("/me/cv-appearance-defaults", mw.cookie, contract)
	// Listing takes a key: it is a read of the caller's own tailored copies, and it is where
	// a CLI learns the CV id every other keyed route here is addressed by. Creating a blank
	// CV stays cookie-only — authoring a whole document is the browser's.
	api.Get("/me/cvs", mw.key, contract)
	// The base (non-tailored) CVs, with the default flagged. Literal segment, registered before
	// `/me/cvs/:id` so Fiber does not read "bases" as a CV uuid.
	api.Get("/me/cvs/bases", mw.key, contract)
	api.Post("/me/cvs", mw.cookie, contract)
	// Read + render accept a key too (keyAuth), so the tailoring agent's CLI can fetch a CV
	// and its PDF; mutations stay cookie-only (POST/PUT/DELETE — the browser owns authoring).
	api.Get("/me/cvs/:id", mw.key, contract)
	api.Put("/me/cvs/:id", mw.cookie, contract)
	// Change only the template (the gallery's one-field switch); cookie-only like other mutations.
	api.Put("/me/cvs/:id/template", mw.cookie, contract)
	// Cookie-only, and deliberately its own route rather than a field on the update above:
	// consent to track a third party is the candidate's to give, and it must not ride along
	// with a document save the tailoring agent can also make.
	api.Put("/me/cvs/:id/tracer-links", mw.cookie, contract)
	api.Get("/me/cvs/:id/tracer-links", mw.cookie, contract)
	api.Delete("/me/cvs/:id", mw.cookie, contract)
	// Copy a base CV (stored document, template, appearance) as "<title> (copy)".
	api.Post("/me/cvs/:id/duplicate", mw.cookie, contract)
	// The agent conversation for a BASE CV (no vacancy). Not behind paidAI: see StartBaseSession.
	api.Post("/me/cvs/:id/base-session", mw.cookie, contract)
	api.Get("/me/cvs/:id/pdf", mw.key, h.RenderCVPDF)
	// Tailoring: the bootstrap takes a key as well as a cookie, so a CLI can start the cycle
	// it was already able to drive (edit + context/get/render all accept a key). It creates a
	// copy of the caller's own CV and spends their own plan allowance — both of which a full-scope
	// key already does through the fit analysis and the assistant — and never calls the LLM.
	api.Post("/me/cvs/tailor", mw.key, mw.paidAI(plan.FeatureTailor), contract)
	api.Post("/me/cvs/:id/tailor-session", mw.cookie, mw.paidAI(plan.FeatureTailor), contract)
	// Literal `/base/` before `:id` — otherwise Fiber treats "base" as a CV uuid.
	api.Post("/me/cvs/base/reseed", mw.cookie, contract)
	// Rebuild this tailored CV (and the base) from the current seed. Cookie-only:
	// destructive whole-document replace; the browser is where the candidate confirms it.
	api.Post("/me/cvs/:id/reseed", mw.cookie, contract)
	api.Patch("/me/cvs/:id", mw.key, contract)
	api.Put("/me/cvs/:id/session", mw.key, contract)
	api.Get("/me/cvs/:id/tailor-context", mw.key, contract)
	// Reading a letter takes a key like every other tailoring read: it calls no model and
	// spends nothing, so a scripted client may have it. Writing one is cookie-only because it
	// spends a daily allowance, and an allowance a key could drain is one the account holder
	// never agreed to spend. The assistant tool reaches the same write over a key-bearing
	// session, which is not a hole: a session is the account holder's own, and its turn is
	// metered before this one is.
	api.Get("/me/cvs/:id/cover-letter/evidence", mw.key, contract)
	api.Put("/me/cvs/:id/cover-letter/evidence", mw.cookie, contract)
	api.Get("/me/cvs/:id/cover-letter", mw.key, contract)
	api.Post("/me/cvs/:id/cover-letter", mw.cookie, mw.paidAI(plan.FeatureCoverLetter), contract)
	// The streaming twin of that POST, and what the workspace actually calls. The chain is
	// three model calls in series and takes minutes; a proxy closes a silent response at
	// sixty seconds, so the one-shot POST reached production only as a 504. The one-shot
	// stays for scripted callers, who control their own timeout.
	//
	// POST, not GET, even though it streams: it spends a daily allowance and writes storage,
	// and the read/write split this feature rests on ("reading a draft never calls a model")
	// would not survive a drafting GET. The client reads the body with fetch rather than
	// EventSource — EventSource can only GET, and it also hides the status code, which is how
	// a 402 first reached the workspace disguised as a dropped connection.
	api.Post("/me/cvs/:id/cover-letter/stream", mw.cookie, mw.paidAI(plan.FeatureCoverLetter), h.StreamCVCoverLetter)
	// The history of what changed the CV, and the two ways to undo an entry: on its own, or
	// as the run it belonged to. Cookie-only for the same reason as every other mutation —
	// the browser is where the candidate is watching this happen, and the tailoring agent
	// must not be able to undo the work it is being measured on.
	api.Get("/me/cvs/:id/revisions", mw.cookie, contract)
	api.Post("/me/cvs/:id/revisions/:rid/undo", mw.cookie, contract)
	api.Post("/me/cvs/:id/revisions/batch/:bid/undo", mw.cookie, contract)
	// What tailoring did to the CV's ATS readiness. Cookie-only, and deliberately so: the
	// tailoring agent authenticates with a CLI credential, so this gate is what keeps the
	// score out of the reach of the thing being measured.
	api.Get("/me/cvs/:id/ats-delta", mw.cookie, contract)
	// How well the tailored CV matches the vacancy it was written for. Cookie-only for the
	// same reason as the delta: the tailoring agent authenticates with a CLI credential, and
	// this gate is what keeps the score out of the reach of the thing being measured.
	api.Get("/me/cvs/:id/job-match", mw.cookie, contract)
}

const maxCVTitleRunes = 200

// cvMetaOf is a CV without its document, as the contract answers it.
func cvMetaOf(m cv.Meta) apigen.CvMeta {
	return apigen.CvMeta{ID: m.ID.String(), Title: m.Title, TemplateID: m.TemplateID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// cvRecordOf is one CV with its document. The document and the run report belong to the
// CV package and are shared with the assistant's tools, so they travel as the JSON their
// types marshal to.
func cvRecordOf(rec cv.Record) (apigen.CvRecord, error) {
	doc, err := json.Marshal(rec.Document)
	if err != nil {
		return apigen.CvRecord{}, err
	}
	out := apigen.CvRecord{
		ID: rec.ID.String(), Title: rec.Title, TemplateID: rec.TemplateID,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
		AgentSessionID:     rec.AgentSessionID,
		Document:           doc,
		TracerLinksEnabled: rec.TracerLinksEnabled,
	}
	if len(rec.AutopilotReport) > 0 {
		if out.AutopilotReport, err = json.Marshal(rec.AutopilotReport); err != nil {
			return apigen.CvRecord{}, err
		}
	}
	return out, nil
}

// ListCVs returns the caller's TAILORED CVs (the re-open list), newest edit first, each with
// its vacancy slug and bound agent session so the client links back to the tailoring workspace.
func (h *cvHandlers) ListCVs(ctx context.Context) (*apigen.ListCVsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.cvStore.ListTailored(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.CvTailored, len(items))
	for i, m := range items {
		meta := cvMetaOf(m.Meta)
		out[i] = apigen.CvTailored{
			ID: meta.ID, Title: meta.Title, TemplateID: meta.TemplateID, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
			JobSlug:        m.JobSlug,
			JobTitle:       m.JobTitle,
			JobCompany:     m.JobCompany,
			AgentSessionID: m.AgentSessionID,
		}
	}
	return &apigen.ListCVsOK{Data: out}, nil
}

// validCVTemplate rejects an unknown template_id (400) and resolves an empty one to the
// default; it returns the id to persist.
func validCVTemplate(id string) (string, error) {
	tmpl, err := cv.ResolveTemplate(id)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "unknown template")
	}
	return tmpl.ID, nil
}

// cvTitle trims, bounds, and defaults the CV title.
func cvTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Untitled CV"
	}
	if r := []rune(title); len(r) > maxCVTitleRunes {
		return strings.TrimSpace(string(r[:maxCVTitleRunes]))
	}
	return title
}

// mapCVError translates cv-domain errors into HTTP errors (ErrNotFound → 404, unknown
// template → 400); any other error propagates as a 500.
func mapCVError(err error) error {
	switch {
	case errors.Is(err, cv.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, cv.ErrNotBase):
		return fiber.NewError(fiber.StatusConflict, "this is a tailored CV, not a base CV")
	case errors.Is(err, cv.ErrUnknownTemplate):
		return fiber.NewError(fiber.StatusBadRequest, "unknown template")
	case errors.Is(err, cvedit.ErrInvalidOp):
		// Surface the specific reason (unknown path, wrong type, out-of-range index) rather
		// than a generic 422: the reason IS the remedy, and a caller that cannot see it can
		// only retry the same mistake.
		return fiber.NewError(fiber.StatusUnprocessableEntity,
			strings.TrimPrefix(err.Error(), cvedit.ErrInvalidOp.Error()+": "))
	case errors.Is(err, cvedit.ErrForbiddenPath), errors.Is(err, cvedit.ErrEvidenceRequired):
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	case errors.Is(err, cvedit.ErrProjectLooksLikeJob):
		// Model-facing: the error already names the project and the fix (experience[]).
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, cvedit.ErrListCap):
		// Candidate-facing: no internal prefixes, clear that nothing was deleted.
		if msg := cvedit.UserListCapMessage(err); msg != "" {
			return fiber.NewError(fiber.StatusConflict, msg)
		}
		return fiber.NewError(fiber.StatusConflict,
			"this role already has the maximum number of bullet points; nothing was changed")
	case errors.Is(err, cvedit.ErrNothingToUndo):
		return fiber.NewError(fiber.StatusConflict, "there is nothing to undo")
	case errors.Is(err, cvedit.ErrCannotUndo):
		// A fact about the document as it stands, not a malformed request: the place this
		// edit changed is gone, so its inverse has nowhere to land.
		return fiber.NewError(fiber.StatusConflict,
			"this edit can no longer be undone — the part of the CV it changed is gone")
	default:
		return err
	}
}

// cvPathID parses the :id route param as a CV's UUID (see cvID).
func cvPathID(c *fiber.Ctx) (uuid.UUID, error) {
	return cvID(c.Params("id"))
}

// cvID parses a CV's UUID. A malformed id cannot name any CV, so it is reported as
// missing rather than as a bad request — keeping "not a CV" and "not yours" the same
// answer, which is what stops the id from being probeable.
func cvID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, "cv not found")
	}
	return id, nil
}

// cv is the refusal as the CV contract answers it: the body write402 sends, with the
// allowance as the JSON its view marshals to.
func (r planRefusal) cv() (*apigen.CvPlanRefusal, error) {
	allowance, err := json.Marshal(r.allowance)
	if err != nil {
		return nil, err
	}
	out := &apigen.CvPlanRefusal{Error: r.message, Allowance: allowance}
	if r.offerUpgrade {
		out.UpgradeURL = apigen.NewOptString(upgradePath)
	}
	return out, nil
}
