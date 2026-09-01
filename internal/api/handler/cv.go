package handler

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tracerlink"
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
	// tracerMinter issues the tokens a traced render substitutes in; nil disables tracing.
	tracerMinter *tracerlink.Minter
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
func newCVHandlers(pool *pgxpool.Pool, queries *db.Queries, cvStore *cv.Store, assistantSessions *assistant.Store, cvRenderer cv.Renderer, tracerSalt, baseURL string, servedHosts []string, resumeStore *resume.Store, photoStore *headshot.Store, plans *plan.Store, match *matchHandlers, gate cvedit.EvidenceGate, jobs jobBoarder, letter coverLetterDeps, refuseListCap bool) *cvHandlers {
	h := &cvHandlers{
		letter:            letter,
		cvStore:           cvStore,
		assistantSessions: assistantSessions,
		cvRenderer:        cvRenderer,
		tracerSalt:        tracerSalt,
		tracerMinter: tracerlink.NewMinter(tracerlink.NewRepository(
			func(ctx context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error) {
				return queries.UpsertTracerLink(ctx, db.UpsertTracerLinkParams{
					CvID: cvID, UserID: userID, Token: token, SourcePath: sourcePath,
					DestinationUrl: destURL, DestinationHash: destHash,
				})
			}), servedHosts),
		tracerBaseURL: baseURL,
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

func (h *cvHandlers) register(api fiber.Router, mw middleware) {
	// CV builder + AI tailoring: open to every signed-in user (the plan meters the LLM spend).
	// Cookie-only, owner-scoped (a foreign id is a 404). The PDF endpoint 501s when no typst
	// binary is configured; the rest still works.
	api.Get("/cv-templates", mw.cookie, h.ListCVTemplates)
	api.Get("/cv-fonts", mw.cookie, h.ListCVFonts)
	// Listing takes a key: it is a read of the caller's own tailored copies, and it is where
	// a CLI learns the CV id every other keyed route here is addressed by. Creating a blank
	// CV stays cookie-only — authoring a whole document is the browser's.
	api.Get("/me/cvs", mw.key, h.ListCVs)
	api.Post("/me/cvs", mw.cookie, h.CreateCV)
	// Read + render accept a key too (keyAuth), so the tailoring agent's CLI can fetch a CV
	// and its PDF; mutations stay cookie-only (POST/PUT/DELETE — the browser owns authoring).
	api.Get("/me/cvs/:id", mw.key, h.GetCV)
	api.Put("/me/cvs/:id", mw.cookie, h.UpdateCV)
	// Change only the template (the gallery's one-field switch); cookie-only like other mutations.
	api.Put("/me/cvs/:id/template", mw.cookie, h.SetCVTemplate)
	// Cookie-only, and deliberately its own route rather than a field on the update above:
	// consent to track a third party is the candidate's to give, and it must not ride along
	// with a document save the tailoring agent can also make.
	api.Put("/me/cvs/:id/tracer-links", mw.cookie, h.SetCVTracerLinks)
	api.Get("/me/cvs/:id/tracer-links", mw.cookie, h.ListCVTracerLinks)
	api.Delete("/me/cvs/:id", mw.cookie, h.DeleteCV)
	api.Get("/me/cvs/:id/pdf", mw.key, h.RenderCVPDF)
	// Tailoring: the bootstrap takes a key as well as a cookie, so a CLI can start the cycle
	// it was already able to drive (edit + context/get/render all accept a key). It creates a
	// copy of the caller's own CV and spends their own plan allowance — both of which a full-scope
	// key already does through the fit analysis and the assistant — and never calls the LLM.
	api.Post("/me/cvs/tailor", mw.key, h.TailorCV)
	api.Post("/me/cvs/:id/tailor-session", mw.cookie, h.StartTailorSession)
	// Literal `/base/` before `:id` — otherwise Fiber treats "base" as a CV uuid.
	api.Post("/me/cvs/base/reset-from-resume", mw.cookie, h.ResetBaseCVFromResume)
	// Rebuild this tailored CV (and the base) from the current résumé seed. Cookie-only:
	// destructive whole-document replace; the browser is where the candidate confirms it.
	api.Post("/me/cvs/:id/reset-from-resume", mw.cookie, h.ResetCVFromResume)
	api.Patch("/me/cvs/:id", mw.key, h.PatchCV)
	api.Put("/me/cvs/:id/session", mw.key, h.SetCVSession)
	api.Get("/me/cvs/:id/tailor-context", mw.key, h.TailorContext)
	// Reading a letter takes a key like every other tailoring read: it calls no model and
	// spends nothing, so a scripted client may have it. Writing one is cookie-only because it
	// spends a daily allowance, and an allowance a key could drain is one the account holder
	// never agreed to spend. The assistant tool reaches the same write over a key-bearing
	// session, which is not a hole: a session is the account holder's own, and its turn is
	// metered before this one is.
	api.Get("/me/cvs/:id/cover-letter", mw.key, h.GetCVCoverLetter)
	api.Post("/me/cvs/:id/cover-letter", mw.cookie, h.DraftCVCoverLetter)
	// The history of what changed the CV, and the two ways to undo an entry: on its own, or
	// as the run it belonged to. Cookie-only for the same reason as every other mutation —
	// the browser is where the candidate is watching this happen, and the tailoring agent
	// must not be able to undo the work it is being measured on.
	api.Get("/me/cvs/:id/revisions", mw.cookie, h.ListCVRevisions)
	api.Post("/me/cvs/:id/revisions/:rid/undo", mw.cookie, h.UndoCVRevision)
	api.Post("/me/cvs/:id/revisions/batch/:bid/undo", mw.cookie, h.UndoCVRevisionBatch)
	// What tailoring did to the CV's ATS readiness. Cookie-only, and deliberately so: the
	// tailoring agent authenticates with a CLI credential, so this gate is what keeps the
	// score out of the reach of the thing being measured.
	api.Get("/me/cvs/:id/ats-delta", mw.cookie, h.GetCVATSDelta)
	// How well the tailored CV matches the vacancy it was written for. Cookie-only for the
	// same reason as the delta: the tailoring agent authenticates with a CLI credential, and
	// this gate is what keeps the score out of the reach of the thing being measured.
	api.Get("/me/cvs/:id/job-match", mw.cookie, h.GetCVJobMatch)
}

const maxCVTitleRunes = 200

type cvMetaResponse struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	TemplateID string    `json:"template_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type cvResponse struct {
	cvMetaResponse
	// AgentSessionID is the roy session bound to a tailored CV (empty when none) — the tailoring
	// workspace resumes it.
	AgentSessionID string      `json:"agent_session_id"`
	Document       cv.Document `json:"document"`
	// AutopilotReport is the last unattended run's account of itself, one entry per
	// requirement. The workspace panel renders it from this read rather than by parsing
	// the conversation, so it survives a reload. Empty when no run has happened.
	AutopilotReport []cv.AutopilotEntry `json:"autopilot_report,omitempty"`
	// TracerLinksEnabled is the candidate's consent for this CV's links to be traced. Reported so
	// the editor can show the toggle's actual state rather than assume it.
	TracerLinksEnabled bool `json:"tracer_links_enabled"`
}

// cvTailoredResponse is a tailored CV in the /my/cvs re-open list: metadata plus the vacancy
// slug and the bound agent session, so the client links each row to its tailoring workspace.
type cvTailoredResponse struct {
	cvMetaResponse
	JobSlug        string `json:"job_slug"`
	JobTitle       string `json:"job_title"`
	JobCompany     string `json:"job_company"`
	AgentSessionID string `json:"agent_session_id"`
}

type createCVRequest struct {
	Title string `json:"title"`
	// TemplateID selects the template; empty defaults to the classic-ats template.
	TemplateID string `json:"template_id"`
	// Seed pre-fills the new CV from the caller's stored résumé structure when available.
	Seed bool `json:"seed"`
}

type updateCVRequest struct {
	Title      string      `json:"title"`
	TemplateID string      `json:"template_id"`
	Document   cv.Document `json:"document"`
}

func metaResponse(m cv.Meta) cvMetaResponse {
	return cvMetaResponse{ID: m.ID.String(), Title: m.Title, TemplateID: m.TemplateID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func recordResponse(rec cv.Record) cvResponse {
	return cvResponse{
		cvMetaResponse:     metaResponse(rec.Meta),
		AgentSessionID:     rec.AgentSessionID,
		Document:           rec.Document,
		AutopilotReport:    rec.AutopilotReport,
		TracerLinksEnabled: rec.TracerLinksEnabled,
	}
}

// ListCVTemplates returns the registered CV templates and their display metadata (id, label,
// style, photo) so the UI can render the template gallery. Static registry data — no DB.
func (h *cvHandlers) ListCVTemplates(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": cv.Templates()})
}

// ListCVFonts returns the registered typefaces a CV may choose (id, label, the familiar face
// it matches, and the CSS stack the live preview renders with). Static registry data — no DB.
// The client reads it instead of hard-coding a list, which is what keeps a second copy of the
// registry from growing in the web app and drifting from the one the renderer obeys.
func (h *cvHandlers) ListCVFonts(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": cv.Fonts()})
}

// ListCVs returns the caller's TAILORED CVs (the re-open list), newest edit first, each with
// its vacancy slug and bound agent session so the client links back to the tailoring workspace.
func (h *cvHandlers) ListCVs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	items, err := h.cvStore.ListTailored(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]cvTailoredResponse, len(items))
	for i, m := range items {
		out[i] = cvTailoredResponse{
			cvMetaResponse: metaResponse(m.Meta),
			JobSlug:        m.JobSlug,
			JobTitle:       m.JobTitle,
			JobCompany:     m.JobCompany,
			AgentSessionID: m.AgentSessionID,
		}
	}
	return c.JSON(fiber.Map{"data": out})
}

// CreateCV creates a CV, optionally seeded from the caller's stored résumé structure.
func (h *cvHandlers) CreateCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createCVRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}

	// Seeding pulls the work history from the experience bank and the rest from the stored
	// structure — both in Postgres, independent of S3 object storage, so it is NOT gated on
	// résumé-storage being enabled. Nothing known degrades to an empty skeleton.
	doc := cv.EmptyDocument()
	if in.Seed {
		if st, ok, err := h.seedSource().Structured(c.Context(), userID); err == nil && ok {
			doc = cv.Seed(st)
		}
	}

	meta, err := h.cvStore.Create(c.Context(), userID, cvTitle(in.Title), tmplID, doc)
	if err != nil {
		return err
	}
	// Return the full record so the client can open the editor without a second fetch.
	rec, err := h.cvStore.Get(c.Context(), meta.ID, userID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": recordResponse(rec)})
}

// GetCV returns one owned CV: the full document on the owner's own session, and the
// document without its contact block to an agent reading with a key.
func (h *cvHandlers) GetCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	// An API-key caller is an agent running its own model over the CV, so it reads through
	// the redacting accessor — the same one the in-process assistant's cv_get tool uses, so
	// one implementation covers both and neither can drift. The owner's own cookie session
	// reads in full; the stored contacts are untouched either way and still render in the PDF.
	read := h.cvStore.Get
	if auth.ViaAPIKey(c) {
		read = h.cvStore.GetForModel
	}
	rec, err := read(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	// Owner cookie sessions: fill empty header contacts from résumé identity
	// (owned / current / provisional — see healRecordHeader). API-key reads stay
	// redacted and never trigger a heal write. List and PDF do not heal.
	if !auth.ViaAPIKey(c) && rec.IsTailored {
		healed, herr := h.healRecordHeader(c.Context(), userID, rec)
		if herr != nil {
			log.Printf("cv: healing tailored header %s: %v", id, herr)
		} else {
			rec = healed
		}
		h.healBaseHeaderIfNeeded(c.Context(), userID)
	}
	return c.JSON(fiber.Map{"data": recordResponse(rec)})
}

// UpdateCV replaces an owned CV's title, template, and document.
func (h *cvHandlers) UpdateCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in updateCVRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}
	// A whole-document save is an input format, not a second way to write the document: the
	// differ derives the operations, and from the editor's point of view this is
	// indistinguishable from an agent's batch. The actor is decided here, by the entry point
	// that authenticated the caller, and never read from the body.
	meta, _, err := h.editor.CommitDocument(c.Context(), id, userID,
		cvedit.ActorCandidate, cvedit.OriginEditor,
		cvedit.State{Title: cvTitle(in.Title), TemplateID: tmplID, Document: in.Document})
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}

type setCVTracerLinksRequest struct {
	Enabled bool `json:"enabled"`
}

// SetCVTracerLinks records the candidate's decision to have the links in this CV's PDF traced,
// or to stop. Off is the state of every CV that has not been through here.
//
// Enabling is refused when the deployment has no visitor salt: without one there is no honest way
// to tell one visitor from another, and accepting the consent while quietly recording less would
// answer a question the candidate did not ask. Disabling is never refused — withdrawing consent is
// not a favour the deployment grants.
func (h *cvHandlers) SetCVTracerLinks(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in setCVTracerLinksRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if in.Enabled && h.tracerSalt == "" {
		return fiber.NewError(fiber.StatusConflict,
			"link tracing is not configured on this deployment")
	}
	if err := h.cvStore.SetTracerLinks(c.Context(), id, userID, in.Enabled); err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"tracer_links_enabled": in.Enabled}})
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
func (h *cvHandlers) tracedHrefs(ctx context.Context, rec cv.Record, userID int64) cv.LinkHrefs {
	if !rec.TracerLinksEnabled || h.tracerMinter == nil || h.tracerBaseURL == "" {
		return cv.LinkHrefs{}
	}
	projectLinks := make([]string, len(rec.Document.Projects))
	for i, p := range rec.Document.Projects {
		projectLinks[i] = p.Link
	}
	minted := h.tracerMinter.Mint(ctx, rec.ID, userID, h.tracerBaseURL,
		rec.CompanySlug, rec.Document.Header.Links, projectLinks)
	return cv.LinkHrefs{Header: minted.Header, Projects: minted.Projects}
}

// tracerLinkResponse is one traced link as the owner reads it.
type tracerLinkResponse struct {
	SourcePath     string `json:"source_path"`
	DestinationURL string `json:"destination_url"`
	TracedURL      string `json:"traced_url"`
	Clicks         int64  `json:"clicks"`
	BotClicks      int64  `json:"bot_clicks"`
	// DistinctVisitors is omitted rather than reported as zero when the deployment has no salt:
	// every click then carries an empty hash, and counting those would say "one visitor" about any
	// number of people. Absent is the honest answer; zero would be a wrong one.
	DistinctVisitors *int64     `json:"distinct_visitors,omitempty"`
	LastClickAt      *time.Time `json:"last_click_at,omitempty"`
}

// ListCVTracerLinks reports what is known about each of an owned CV's traced links.
//
// Clicks the owner made themselves are excluded by the query — they are recorded so the history
// stays complete, not so they can be read back as somebody else's interest. Automated traffic is
// counted separately rather than dropped, so the UI's "include likely bots" switch needs no second
// request.
func (h *cvHandlers) ListCVTracerLinks(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	rows, err := h.queries.ListTracerLinkStats(c.Context(), db.ListTracerLinkStatsParams{CvID: id, UserID: userID})
	if err != nil {
		return err
	}
	out := make([]tracerLinkResponse, 0, len(rows))
	for _, r := range rows {
		item := tracerLinkResponse{
			SourcePath:     r.SourcePath,
			DestinationURL: r.DestinationUrl,
			TracedURL:      h.tracerBaseURL + "/cv/" + r.Token,
			Clicks:         r.Clicks,
			BotClicks:      r.BotClicks,
		}
		if h.tracerSalt != "" {
			n := r.DistinctVisitors
			item.DistinctVisitors = &n
		}
		if r.LastClickAt.Valid {
			t := r.LastClickAt.Time
			item.LastClickAt = &t
		}
		out = append(out, item)
	}
	return c.JSON(fiber.Map{"data": out})
}

// DeleteCV removes an owned CV.
func (h *cvHandlers) DeleteCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	if err := h.cvStore.Delete(c.Context(), id, userID); err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type setCVSessionRequest struct {
	SessionID string `json:"session_id"`
}

// SetCVSession binds a roy agent session to an owned CV so the tailoring workspace can re-open
// that exact session later. Cookie or API key; owner-scoped (a foreign/missing id is a 404).
func (h *cvHandlers) SetCVSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in setCVSessionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.cvStore.SetSession(c.Context(), id, userID, in.SessionID); err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type setCVTemplateRequest struct {
	TemplateID string `json:"template_id"`
}

// SetCVTemplate changes only the template of an owned CV (title and document untouched). The
// gallery uses this to switch templates without re-sending the whole document. Owner-scoped
// (a foreign/missing id is a 404); an unknown template_id is a 400.
func (h *cvHandlers) SetCVTemplate(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	var in setCVTemplateRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	tmplID, err := validCVTemplate(in.TemplateID)
	if err != nil {
		return err
	}
	// Through the editor like every other change: "switched to the Sidebar template" is a
	// legitimate line in the history, and one the candidate may want to undo.
	path, err := cvedit.ParsePath("template_id")
	if err != nil {
		return err
	}
	_, _, err = h.editor.Commit(c.Context(), id, userID, cvedit.Change{
		Actor:  cvedit.ActorCandidate,
		Origin: cvedit.OriginTemplate,
		Ops:    []cvedit.Op{{Kind: cvedit.OpSet, Path: path, Value: tmplID}},
	})
	if err != nil {
		return mapCVError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RenderCVPDF renders an owned CV to PDF and streams it. 501 when no renderer is
// configured (no typst binary); the CRUD surface still works in that state.
func (h *cvHandlers) RenderCVPDF(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if h.cvRenderer == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	rec, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	pdf, err := h.cvRenderer.Render(c.Context(), rec.Document, tmpl,
		headshotForTemplate(c.Context(), h.photos, userID, tmpl),
		h.tracedHrefs(c.Context(), rec, userID))
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="cv.pdf"`)
	return c.Send(pdf)
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

// cvPathID parses the :id route param as a CV's UUID. A malformed id cannot name
// any CV, so it is reported as missing rather than as a bad request — keeping
// "not a CV" and "not yours" the same answer, which is what stops the id from
// being probeable.
func cvPathID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, "cv not found")
	}
	return id, nil
}

// ListCVRevisions serves the history of what changed this CV, newest first.
//
// The feed carries the addresses each revision touched but not the operations themselves:
// the preview needs the places to underline, and the inverses hold the candidate's own
// earlier text, which has no business travelling out for a list view.
func (h *cvHandlers) ListCVRevisions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	// Owner-scoped read first, so a foreign CV is a 404 rather than an empty feed — the
	// same answer "no such CV" and "not yours" have everywhere else here.
	if _, err := h.cvStore.Get(c.Context(), id, userID); err != nil {
		return mapCVError(err)
	}
	revisions, err := h.editor.History(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": cvedit.Views(revisions)})
}

// UndoCVRevision reverses one entry, leaving later edits in place. The undo is itself
// recorded, so the feed keeps describing what actually happened to the document.
func (h *cvHandlers) UndoCVRevision(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	revisionID, err := uuid.Parse(c.Params("rid"))
	if err != nil {
		// A malformed id names no revision, so it is reported as missing rather than as a
		// bad request — keeping "no such revision" and "not yours" the same answer.
		return fiber.NewError(fiber.StatusNotFound, "revision not found")
	}
	meta, undo, err := h.editor.Revert(c.Context(), id, userID, revisionID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"cv": metaResponse(meta), "revision": undo.View()}})
}

// UndoCVRevisionBatch reverses every standing edit of one agent turn, newest first.
//
// The batch id comes from the feed rather than from a column on the CV: the run's own
// revisions carry it, which is what let the pre-run snapshot be retired along with the edge
// two concurrent runs used to create.
func (h *cvHandlers) UndoCVRevisionBatch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}
	batchID, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "run not found")
	}
	meta, err := h.editor.RevertBatch(c.Context(), id, userID, batchID)
	if err != nil {
		return mapCVError(err)
	}
	// The run report goes with the run: it describes what the run made of each requirement,
	// and those edits have just been reversed. Best-effort — the document is already back,
	// and failing the request now would tell the candidate nothing was undone.
	if err := h.cvStore.SetAutopilotReport(c.Context(), id, userID, nil); err != nil {
		log.Printf("cv: clearing the run report after a batch revert: %v", err)
	}
	return c.JSON(fiber.Map{"data": metaResponse(meta)})
}
