package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	classifydict "github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// resumeHandlers serves the résumé/CV surfaces: skill extraction, stored-résumé management,
// the profile verdict and the CV ATS-readiness report. resume.Store is nil-safe: with no
// blob (S3 unconfigured) Enabled() is false and the upload/verdict paths parse in-request.
type resumeHandlers struct {
	resume *resume.Store
	// structuredExtractor derives the structured résumé in the background; with the LLM
	// unconfigured it no-ops and the profile shows no structured section.
	structuredExtractor *resumeextract.Extractor
	// facets backs the market-coverage computations (verdict + ATS report).
	facets facetCounter
	// userProfile loads the caller's profile (skills + role the verdict scores against).
	userProfile *userprofile.Service
	// llm binds the model client and the credential resolver, so the extraction and the
	// ATS review are both spent under the account that asked for them.
	llm llmBinding
	// atsAnalyzer runs the optional LLM review for the ATS report; a no-op when the LLM is unconfigured.
	atsAnalyzer *atscheck.Analyzer
	// atsCache reads/writes the per-user cached CV ATS review (backed by *db.Queries).
	atsCache atsReviewStore
	// bank receives the work history of every extracted CV and serves it back on the status
	// read. Nil when the queries are unavailable; both then no-op.
	bank experienceBank
}

func newResumeHandlers(resumeStore *resume.Store, structuredExtractor *resumeextract.Extractor, facets facetCounter, userProfile *userprofile.Service, atsAnalyzer *atscheck.Analyzer, queries *db.Queries) *resumeHandlers {
	return &resumeHandlers{
		resume:              resumeStore,
		structuredExtractor: structuredExtractor,
		facets:              facets,
		userProfile:         userProfile,
		atsAnalyzer:         atsAnalyzer,
		atsCache:            queries,
		bank:                newExperienceBank(queries, userProfile),
	}
}

// newExperienceBank builds the bank the upload path feeds, or nil (banking disabled) without
// queries. profileSkills folds an imported atom's skills into the owner's profile.
func newExperienceBank(queries *db.Queries, profileSkills experience.ProfileSkills) experienceBank {
	if queries == nil {
		return nil
	}
	return experience.NewStore(experience.NewQueriesRepository(queries)).SetProfileSkills(profileSkills)
}

func (h *resumeHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The verdict is a profile sub-resource, cookie-only like the profile (no profile → 404).
	api.Get("/me/profile/verdict", mw.cookie, contract)
	// ATS report: GET scores the stored CV (structure + role keyword-match); POST runs and caches
	// the optional LLM review.
	api.Get("/me/profile/ats-report", mw.cookie, contract)
	api.Post("/me/profile/ats-report", mw.cookie, mw.paidAI(unmeteredAI), contract)

	// Optional session. Signed in with S3 configured, it also stores the résumé (the single upload
	// point); otherwise it is stateless. Signed out (the AI search lets a visitor attach a CV) it
	// is always stateless and calls no model, bounded per address.
	//
	// Deliberately NOT behind mw.paidAI, like PUT /me/resume: the background structured extraction
	// is the model call every account gets (plan.WithFreeAI); without it the experience bank
	// stays empty for anyone who uploads before paying.
	api.Post("/me/resume/extract", mw.optional, h.ExtractResumeProfile)

	// Résumé storage, cookie-only; PUT stays on Fiber because the upload is multipart-or-JSON.
	// 501 from PUT/DELETE when S3 is unconfigured.
	api.Put("/me/resume", mw.cookie, h.PutResume)
	api.Get("/me/resume", mw.cookie, contract)
	api.Put("/me/resume/contacts", mw.cookie, contract)
	api.Post("/me/resume/contacts/replace-from-cv", mw.cookie, contract)
	api.Delete("/me/resume", mw.cookie, contract)
	// The retry runs the same free extraction again, so it is open to free as well.
	api.Post("/me/resume/retry-extract", mw.cookie, contract)

	// Stateless market-coverage of a caller-supplied skill list; cookie or key (the CLI uses a key).
	api.Post("/market/coverage", mw.key, contract)

	// The public CV roast: no session or key, since it is the landing page for cold search
	// traffic. Stores nothing and calls no model. IP-limited, as there is no caller to key by.
	api.Post("/cv/roast", h.RoastCV)

	// The CV-first onboarding's read (model parse, dictionary profile, ATS score), storing
	// nothing. The one model call a guest gets (plan.WithFreeAI), bounded per address by the
	// operations gateway; never behind mw.paidAI.
	api.Post("/onboarding/cv", mw.optional, h.ParseOnboardingCV)
}

// resumeTextRequest is the JSON body for the pasted-text path.
type resumeTextRequest struct {
	Text string `json:"text"`
}

// resumeUpload is a parsed résumé upload: the original bytes (to store), their content
// type, and the derived plain text (to extract skills / feed coherence).
type resumeUpload struct {
	Data        []byte
	ContentType string
	Text        string
}

// cvProfile is the profile a résumé yields through the deterministic dictionaries:
// canonical skills, the seniority grade, and every category the résumé spans (a person
// can be several — backend + ML). Each dictionary "never guesses", so an unresolved field
// is empty.
type cvProfile struct {
	Skills     []string
	Seniority  string
	Categories []string
}

// errResumeNoText is the message for a PDF that parsed but yielded no text (a scan or image);
// it names the cause so the user pastes text instead of assuming a size rejection.
const errResumeNoText = "couldn't read any text from this PDF — it looks like a scan or image. Upload a text-based PDF, or paste your résumé text instead."

// headlineRunes bounds the résumé "headline" fed to the title dictionaries: wide enough to reach
// the title past the name and contacts, tight enough to keep career history out.
const headlineRunes = 120

// resumeHeadline returns the top of a résumé (name, title, start of the summary) as one string
// with contact/metadata tokens dropped. The title dictionary runs over this, not the whole CV,
// so a past "Director" role can't over-promote the grade. Whitespace is collapsed first because
// PDF extraction often yields one token per line, which would stall a line-based scan.
func resumeHeadline(text string) string {
	var kept []string
	runes := 0
	for _, tok := range strings.Fields(text) {
		if looksLikeContactToken(tok) {
			continue
		}
		kept = append(kept, tok)
		if runes += len([]rune(tok)) + 1; runes >= headlineRunes {
			break
		}
	}
	return strings.Join(kept, " ")
}

// looksLikeContactToken reports whether a résumé token is contact/metadata noise rather than a
// title word: an email, a profile URL, or a token with no letters (a "|", a phone fragment).
func looksLikeContactToken(tok string) bool {
	lower := strings.ToLower(tok)
	if strings.Contains(lower, "@") {
		return true
	}
	for _, s := range []string{"http", "www.", "linkedin.com", "github.com", ".com/"} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	for _, r := range tok {
		if unicode.IsLetter(r) {
			return false
		}
	}
	return true // no letters → punctuation / phone digits / bare number
}

// resumeProfile derives a cvProfile from résumé text with the dictionaries only (no LLM):
// skilltag over the whole text, classify over the headline.
func resumeProfile(text string) cvProfile {
	skills := skilltag.Parse(text, skilltag.WithResumeAcronyms())
	if skills == nil {
		skills = []string{}
	}
	head := resumeHeadline(text)
	categories := classifydict.Categories(head)
	if categories == nil {
		categories = []string{}
	}
	return cvProfile{
		Skills:     skills,
		Seniority:  classifydict.Parse(head).Seniority,
		Categories: categories,
	}
}

// ExtractResumeProfile turns an uploaded résumé into skill slugs, a seniority grade and
// categories via the dictionaries (no LLM). It accepts a PDF (multipart field "file") or text
// (JSON {text}), dispatched by Content-Type. For a signed-in caller with S3 configured it also
// stores the résumé, best-effort (a hiccup must not fail extraction); a signed-out caller's
// bytes die with the request. Oversize bodies are rejected by the global BodyLimit (413).
func (h *resumeHandlers) ExtractResumeProfile(c *fiber.Ctx) error {
	userID, signedIn := auth.UserID(c)
	if !signedIn {
		if err := refuseEncodedBody(c); err != nil {
			return err
		}
	}

	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}

	prof := resumeProfile(up.Text)

	if signedIn && h.resume.Enabled() {
		// Best-effort: log (never the résumé bytes) and still return the profile.
		if _, err := h.storeCV(c.Context(), userID, up); err != nil {
			log.Printf("resume: store on extract failed for user %d: %v", userID, err)
		}
	}

	// skills and categories are always arrays (possibly empty) so the client can treat
	// them uniformly; seniority is omitted when unresolved so a client never sees a guess.
	data := fiber.Map{"skills": prof.Skills, "categories": prof.Categories}
	if prof.Seniority != "" {
		data["seniority"] = prof.Seniority
	}
	return c.JSON(fiber.Map{"data": data})
}

// newResumeMeta builds the wire shape for résumé status. UploadedAt is RFC3339 or nil;
// StructurePending is true when a résumé is present but its structured stamp does not match
// the upload yet.
func newResumeMeta(enabled bool, m resume.Meta) resume.MetaResponse {
	out := resume.MetaResponse{Enabled: enabled, Present: m.Present}
	if m.UploadedAt != nil {
		s := m.UploadedAt.UTC().Format(time.RFC3339)
		out.UploadedAt = &s
	}
	return out
}

// PutResume stores (or replaces) the caller's résumé in object storage and records the
// pointer. 501 when storage is unconfigured. Cookie-only.
func (h *resumeHandlers) PutResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !h.resume.Enabled() {
		return fiber.NewError(fiber.StatusNotImplemented, "résumé storage is not available")
	}
	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}
	meta, err := h.storeCV(c.Context(), userID, up)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": newResumeMeta(true, meta)})
}

// storeCV is the CV upload use case every entry point shares (PutResume, the extract path and,
// as storeFirstCV, the MCP add_cv tool): store the bytes, then derive the structured résumé in
// the background, stamped with this upload's time so it is served only while it describes the
// current CV. The derivation marks its own model call free (plan.WithFreeAI).
func (h *resumeHandlers) storeCV(ctx context.Context, userID int64, up resumeUpload) (resume.Meta, error) {
	return h.storeAndDerive(ctx, userID, up, h.resume.Put)
}

// storeFirstCV is storeCV for an account with no CV yet, and resume.ErrAlreadyStored for one
// that has one — decided under the per-user lock every upload takes.
func (h *resumeHandlers) storeFirstCV(ctx context.Context, userID int64, up resumeUpload) (resume.Meta, error) {
	return h.storeAndDerive(ctx, userID, up, h.resume.PutIfAbsent)
}

func (h *resumeHandlers) storeAndDerive(ctx context.Context, userID int64, up resumeUpload,
	put func(context.Context, int64, string, []byte) (resume.Meta, error)) (resume.Meta, error) {
	meta, err := put(ctx, userID, up.ContentType, up.Data)
	if err != nil {
		return resume.Meta{}, err
	}
	h.deriveResumeArtifacts(userID, up.Text, meta.UploadedAt)
	return meta, nil
}

// deriveResumeArtifacts kicks the detached, best-effort derivation of the structured résumé.
// RetryResumeExtract re-runs it over the bytes already stored.
//
// Intentionally does NOT write cvs rows: seeding a base or tailored CV is the candidate's
// explicit Reset from résumé (or TailorCV's stale-base refresh); upload never mutates cvs.
func (h *resumeHandlers) deriveResumeArtifacts(userID int64, text string, uploadedAt *time.Time) {
	go h.extractStructuredResume(userID, text, uploadedAt)
}

// extractStructuredResume derives the structured résumé from the just-uploaded CV and persists
// it, stamped with uploadedAt (captured up front so the stamp matches the CV actually read;
// Store.Structured serves only on a stamp match). Best-effort: an unconfigured LLM, a missing
// upload time, or any error is logged (never the CV text) and swallowed. Runs on its own
// timeout context because the request's is gone.
func (h *resumeHandlers) extractStructuredResume(userID int64, text string, uploadedAt *time.Time) {
	success := false
	defer func() { observability.RecordCVExtraction(success) }()
	if h.structuredExtractor == nil || !h.structuredExtractor.Enabled() {
		log.Printf("resume structured: user %d: skipped — extractor disabled (LLM unset)", userID)
		if uploadedAt != nil {
			_ = h.resume.MarkExtractFailed(context.Background(), userID, "extractor unavailable", *uploadedAt)
		}
		return
	}
	if uploadedAt == nil {
		log.Printf("resume structured: user %d: skipped — missing upload stamp", userID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), resumeExtractLLMTimeout+30*time.Second)
	defer cancel()
	ctx = plan.WithFreeAI(ctx)
	caller, err := h.llm.caller(ctx, userID, llm.Feature(tagCVExtract))
	if err != nil {
		log.Printf("resume structured: user %d: %v", userID, err)
		_ = h.resume.MarkExtractFailed(ctx, userID, "extract failed", *uploadedAt)
		return
	}
	extractor := h.structuredExtractor.As(caller)
	st, err := extractor.Extract(ctx, text)
	if err != nil {
		log.Printf("resume structured: user %d: %v", userID, err)
		_ = h.resume.MarkExtractFailed(ctx, userID, "extract failed", *uploadedAt)
		return
	}
	if err := h.resume.SetStructured(ctx, userID, st, extractor.ModelID(), *uploadedAt); err != nil {
		log.Printf("resume structured persist: user %d: %v", userID, err)
		_ = h.resume.MarkExtractFailed(ctx, userID, "persist failed", *uploadedAt)
		return
	}
	success = true
	h.importExperience(ctx, userID, st, uploadedAt.UTC().Format(time.RFC3339))
}

// wireParseStatus is the parse status a client sees, and whether a structure is still owed.
// A failure is reported as pending-structure (the retry may fill it).
func wireParseStatus(es resume.ExtractStatus) (status string, pending bool) {
	return es.Status, es.Status == resume.ExtractStatusPending || es.Status == resume.ExtractStatusFailed
}

// resumeDataOK answers GetResume with the value as the Go type marshals it.
func resumeDataOK(v any) (*apigen.GetResumeOK, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &apigen.GetResumeOK{Data: raw}, nil
}

// GetResume reports whether the caller has a stored résumé (and when). Always 200:
// unconfigured storage or no résumé is a normal state. Cookie-only.
// Identity composition follows internal/candidate/resume/AGENTS.md: owned contacts as a block,
// current structure for semantic sections, provisional contacts while pending.
func (h *resumeHandlers) GetResume(ctx context.Context) (*apigen.GetResumeOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if !h.resume.Enabled() {
		return resumeDataOK(newResumeMeta(false, resume.Meta{}))
	}
	meta, err := h.resume.Status(ctx, userID)
	if err != nil {
		return nil, err
	}
	resp := newResumeMeta(true, meta)

	row, err := h.resume.StructuredRow(ctx, userID)
	if err != nil {
		log.Printf("resume structured row: user %d: %v", userID, err)
	} else {
		es := resume.ResolveExtractStatus(row)
		resp.ParseStatus, resp.StructurePending = wireParseStatus(es)
		resp.ParseDetail = es.Detail
	}

	if owned, err := h.resume.CandidateOwned(ctx, userID); err != nil {
		log.Printf("resume owned: user %d: %v", userID, err)
	} else if !owned.Empty() {
		contacts := owned
		resp.Contacts = &contacts
	}

	var st resumeextract.Structured
	if pr, err := h.resume.ProfileReadForUser(ctx, userID); err != nil {
		log.Printf("resume structured read: user %d: %v", userID, err)
	} else {
		if pr.Pending {
			resp.StructurePending = true
		}
		// pr.Structure already carries only identity fields when !pr.Current.
		st = pr.Structure
		st.Experience = nil
		// Owned overrides win over the latest extract, field by field. Identity and body are gated
		// separately: blank owned identity fields (only a summary was edited) must NOT blank a
		// real name/email from the extract.
		if resp.Contacts != nil {
			if !resp.Contacts.IdentityEmpty() {
				st.FullName = resp.Contacts.FullName
				st.Email = resp.Contacts.Email
				st.Phone = resp.Contacts.Phone
				st.Location = resp.Contacts.Location
				st.Links = append([]string(nil), resp.Contacts.Links...)
			}
			resp.Contacts.ApplyBody(&st)
		}
	}
	if h.bank != nil {
		history, err := h.bank.SeedHistory(ctx, userID)
		if err != nil {
			log.Printf("resume seed history: user %d: %v", userID, err)
		} else {
			st.Experience = history.Experience
			st.Projects = history.Projects
		}
	}
	if resumeStructureWorthServing(st) {
		resp.Structured = &st
	}
	return resumeDataOK(resp)
}

// PutResumeContacts replaces the candidate's owned overrides without a CV upload. The route
// name predates resume.Owned growing past identity: the body also takes
// headline/summary/languages/certifications.
func (h *resumeHandlers) PutResumeContacts(ctx context.Context, req jx.Raw) (*apigen.PutResumeContactsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var in resume.Owned
	if err := json.Unmarshal(req, &in); err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid contacts body")
	}
	out, err := h.resume.SetCandidateOwned(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &apigen.PutResumeContactsOK{Data: raw}, nil
}

// ReplaceResumeContactsFromCV overwrites every owned override from the current structured
// résumé — a full reset, not just identity fields.
func (h *resumeHandlers) ReplaceResumeContactsFromCv(ctx context.Context) (*apigen.ReplaceResumeContactsFromCvOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	st, ok, err := h.resume.Structured(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fiber.NewError(fiber.StatusConflict, "no current structured résumé to copy contacts from")
	}
	out, err := h.resume.ReplaceOwnedFromStructured(ctx, userID, st)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &apigen.ReplaceResumeContactsFromCvOK{Data: raw}, nil
}

// resumeStructureWorthServing reports whether the composed structure has anything the
// profile tab should render (contacts and/or banked experience / semantic sections).
func resumeStructureWorthServing(st resumeextract.Structured) bool {
	if st.FullName != "" || st.Email != "" || st.Phone != "" || st.Location != "" || len(st.Links) > 0 {
		return true
	}
	if len(st.Experience) > 0 || len(st.Education) > 0 || len(st.Skills) > 0 ||
		len(st.Languages) > 0 || len(st.Projects) > 0 || len(st.Certifications) > 0 {
		return true
	}
	return st.Summary != "" || st.Headline != "" || st.TotalYears > 0
}

// DeleteResume removes the caller's stored résumé (object + pointer). 501 when storage is
// unconfigured. Cookie-only.
func (h *resumeHandlers) DeleteResume(ctx context.Context) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	if !h.resume.Enabled() {
		return fiber.NewError(fiber.StatusNotImplemented, "résumé storage is not available")
	}
	return h.resume.Delete(ctx, userID)
}

// RetryResumeExtract re-derives the structured résumé from the already-stored upload. It reads
// text and stamp in one pointer read (resume.Store.TextAndUploadedAt) so they cannot mismatch.
//
// Marks the extract pending BEFORE the background derivation starts; otherwise a client polling
// GET /me/resume would see the PREVIOUS attempt's "failed" status and mistake it for this
// retry's outcome. Cookie-only.
func (h *resumeHandlers) RetryResumeExtract(ctx context.Context) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	if !h.resume.Enabled() {
		return fiber.NewError(fiber.StatusNotImplemented, "résumé storage is not available")
	}
	text, uploadedAt, err := h.resume.TextAndUploadedAt(ctx, userID)
	if errors.Is(err, resume.ErrNotStored) {
		return fiber.NewError(fiber.StatusConflict, "no résumé stored to retry")
	}
	if err != nil {
		return err
	}
	if err := h.resume.MarkExtractPending(ctx, userID, uploadedAt); err != nil {
		return err
	}
	h.deriveResumeArtifacts(userID, text, &uploadedAt)
	return nil
}

// readResumeUpload reads a résumé from the request into its original bytes, content type,
// and derived plain text: JSON {text} for the paste path, otherwise the "file" part
// parsed as a PDF.
func readResumeUpload(c *fiber.Ctx) (resumeUpload, error) {
	if strings.HasPrefix(c.Get(fiber.HeaderContentType), fiber.MIMEApplicationJSON) {
		var in resumeTextRequest
		if err := c.BodyParser(&in); err != nil {
			return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		return resumeUpload{
			Data:        []byte(in.Text),
			ContentType: "text/plain; charset=utf-8",
			Text:        in.Text,
		}, nil
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "missing resume file")
	}
	f, err := fh.Open()
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "cannot read resume file")
	}
	defer f.Close()
	// Buffered because the raw bytes go to storage and the PDF parser reads them too; bounded by BodyLimit.
	data, err := io.ReadAll(f)
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "cannot read resume file")
	}
	// Bad client input is a 400, not a 500; ExtractPDFText recovers parser panics into errors.
	text, err := resume.ExtractPDFText(data)
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
	}
	return resumeUpload{Data: data, ContentType: "application/pdf", Text: text}, nil
}
