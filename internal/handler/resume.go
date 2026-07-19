package handler

import (
	"context"
	"io"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"

	classifydict "github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/skilltag"
)

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

// errResumeNoText is returned when a résumé file parsed cleanly but yielded no text —
// a scanned or image-only PDF with no selectable text layer. The message names the real
// cause (not a bare "empty") so the user knows to upload a text-based PDF or paste the
// text, rather than assuming the file was rejected for its size.
const errResumeNoText = "couldn't read any text from this PDF — it looks like a scan or image. Upload a text-based PDF, or paste your résumé text instead."

// headlineRunes bounds the résumé "headline" fed to the title dictionaries: wide enough to
// clear the name + contact preamble and reach the title (and a few summary words), tight
// enough that the career-history section below can't reach in and over-promote the grade.
const headlineRunes = 120

// resumeHeadline returns the top of a résumé as a single flowing string — the name, title,
// and the start of the summary — with contact/metadata tokens (email, phone, profile URL,
// bare numbers, punctuation) dropped. The title dictionary (classify) runs over this, not
// the whole CV, so a grade or role word buried in the career history
// ("reported to the Head of Eng", a past Director role) can't over-promote the current
// role. It collapses all whitespace first because PDF text extraction is unreliable about
// line breaks — a two-column or table header often comes out one token per line — so a
// line-based scan would stall on the name and never reach the title.
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

// looksLikeContactToken reports whether a whitespace-delimited résumé token is
// contact/metadata noise rather than a title word: an email, a profile URL, or a token
// with no letters at all (a "|" separator, a phone fragment, a bare number). Dropping these
// keeps the headline landing on the actual role, not the candidate's inbox.
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

// resumeProfile derives a cvProfile from résumé text using only the existing
// dictionaries — skilltag for skills over the whole text (skills appear anywhere), and
// classify over the headline (the current title + summary top) for the seniority grade
// and the categories the résumé spans. No LLM.
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

// ExtractResumeProfile turns an uploaded résumé into a structured profile — canonical
// skill slugs plus the dictionary-resolved seniority grade and the categories it spans —
// all via the deterministic dictionaries (no LLM). It accepts a PDF (multipart/form-data field
// "file") or plain text (application/json {text}), dispatched by Content-Type. When S3
// storage is configured it also stores the résumé once — the single upload point, so the
// verdict's coherence can reuse it without a second upload; storing is best-effort here
// (a hiccup must not fail extraction, this endpoint's contract). When storage is
// unconfigured the résumé is parsed and discarded (only the derived fields are returned).
// Behind RequireAuth (cookie-only). Oversize bodies are rejected by the server's global
// BodyLimit (413) before this handler runs; the web client also guards the size up front.
func (a *API) ExtractResumeProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}

	prof := resumeProfile(up.Text)

	if a.resume.Enabled() {
		if meta, err := a.resume.Put(c.Context(), userID, up.ContentType, up.Data); err != nil {
			// Best-effort: log (never the résumé bytes) and still return the profile.
			log.Printf("resume: store on extract failed for user %d: %v", userID, err)
		} else {
			// This is the résumé-upload path the app actually uses, so it is where the CV
			// gets embedded for /my/recommendations and structured for the profile.
			a.deriveResumeArtifacts(userID, up.Text, meta.UploadedAt)
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

// resumeMetaResponse is the wire shape for résumé status: whether storage is enabled at
// all, whether the caller has a résumé stored, and when it was uploaded (RFC3339, nil
// when absent). Structured carries the read-only structured résumé for the profile view,
// null when the caller has none current (no résumé, unconfigured LLM, not yet extracted,
// or stale relative to the current CV).
type resumeMetaResponse struct {
	Enabled    bool                      `json:"enabled"`
	Present    bool                      `json:"present"`
	UploadedAt *string                   `json:"uploaded_at"`
	Structured *resumeextract.Structured `json:"structured"`
}

func newResumeMeta(enabled bool, m resume.Meta) resumeMetaResponse {
	out := resumeMetaResponse{Enabled: enabled, Present: m.Present}
	if m.UploadedAt != nil {
		s := m.UploadedAt.UTC().Format(time.RFC3339)
		out.UploadedAt = &s
	}
	return out
}

// PutResume stores (or replaces) the caller's résumé in object storage and records the
// pointer, returning the résumé metadata. 501 when storage is unconfigured (the SPA then
// falls back to per-request upload on the verdict page). Cookie-only.
func (a *API) PutResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !a.resume.Enabled() {
		return fiber.NewError(fiber.StatusNotImplemented, "résumé storage is not available")
	}
	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}
	meta, err := a.resume.Put(c.Context(), userID, up.ContentType, up.Data)
	if err != nil {
		return err
	}
	// Embed in the background: it must not block the upload response. Embedding is a
	// Meilisearch round-trip that is seconds normally but MINUTES while a full semantic
	// rebuild is monopolizing the engine — long enough to time out the proxy/upload.
	// Derive the CV embedding and the structured résumé in the background (best-effort,
	// off the response path). The structure is stamped with this upload's time so it is
	// served only while it describes the current CV.
	a.deriveResumeArtifacts(userID, up.Text, meta.UploadedAt)
	return c.JSON(fiber.Map{"data": newResumeMeta(true, meta)})
}

// embedResume computes and persists the user's CV embedding through the same embedder
// as jobs (so it shares their vector space), best-effort: any failure — no search
// backend, embed error, or persist error — is logged and swallowed so it never breaks
// the upload. On an embed failure the prior vector is cleared so the new CV is never
// matched by a stale one. The scratch id is the user id. It runs on its own timeout
// context (not the request's, which is already gone once the upload responded).
func (a *API) embedResume(userID int64, text string) {
	if a.search == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	vec, model, err := a.search.EmbedText(ctx, text)
	if err != nil {
		log.Printf("resume embed: user %d: %v", userID, err)
		if err := a.resume.SetEmbedding(ctx, userID, nil, ""); err != nil {
			log.Printf("resume embed clear: user %d: %v", userID, err)
		}
		return
	}
	if err := a.resume.SetEmbedding(ctx, userID, vec, model); err != nil {
		log.Printf("resume embed persist: user %d: %v", userID, err)
	}
}

// deriveResumeArtifacts kicks the background best-effort derivations a fresh upload feeds:
// the CV embedding (/my/recommendations) and the structured résumé (profile view + fit
// context). Both run detached on their own timeout contexts. Defined once so the two
// upload paths (PutResume, ExtractResumeProfile) can't drift out of sync.
func (a *API) deriveResumeArtifacts(userID int64, text string, uploadedAt *time.Time) {
	go a.embedResume(userID, text)
	go a.extractStructuredResume(userID, text, uploadedAt)
}

// extractStructuredResume derives the read-only structured résumé from the just-uploaded
// CV and persists it, stamped with uploadedAt (the résumé upload time it was derived
// from, captured up front so the stamp matches the CV actually read — Store.Structured
// serves only on a stamp match). Background + best-effort: an unconfigured LLM, a missing
// upload time, or any extraction/persist error is logged (never the CV text/bytes) and
// swallowed, so the upload and the deterministic extractors are untouched. Runs on its
// own timeout context (the request's is gone once the upload responded).
func (a *API) extractStructuredResume(userID int64, text string, uploadedAt *time.Time) {
	if !a.structuredExtractor.Enabled() || uploadedAt == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), resumeExtractLLMTimeout+30*time.Second)
	defer cancel()
	st, err := a.structuredExtractor.Extract(ctx, text)
	if err != nil {
		log.Printf("resume structured: user %d: %v", userID, err)
		return
	}
	if err := a.resume.SetStructured(ctx, userID, st, a.structuredExtractor.ModelID(), *uploadedAt); err != nil {
		log.Printf("resume structured persist: user %d: %v", userID, err)
	}
}

// GetResume reports whether the caller has a stored résumé (and when). Always 200:
// unconfigured storage or no résumé is a normal state the SPA renders (it decides between
// "re-run coherence" and a single upload prompt). Cookie-only.
func (a *API) GetResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !a.resume.Enabled() {
		return c.JSON(fiber.Map{"data": newResumeMeta(false, resume.Meta{})})
	}
	meta, err := a.resume.Status(c.Context(), userID)
	if err != nil {
		return err
	}
	resp := newResumeMeta(true, meta)
	// Attach the read-only structured résumé when a current one exists (best-effort: a
	// read hiccup or stale/absent structure simply leaves it null, never failing status).
	if st, ok, err := a.resume.Structured(c.Context(), userID); err != nil {
		log.Printf("resume structured read: user %d: %v", userID, err)
	} else if ok {
		resp.Structured = &st
	}
	return c.JSON(fiber.Map{"data": resp})
}

// DeleteResume removes the caller's stored résumé (object + pointer). 501 when storage is
// unconfigured. Cookie-only.
func (a *API) DeleteResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if !a.resume.Enabled() {
		return fiber.NewError(fiber.StatusNotImplemented, "résumé storage is not available")
	}
	if err := a.resume.Delete(c.Context(), userID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
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
	// Buffer the upload: the raw bytes go to storage and the same bytes (as a ReaderAt)
	// feed the PDF parser. The server's 8MB BodyLimit bounds this.
	data, err := io.ReadAll(f)
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "cannot read resume file")
	}
	// An undecodable or non-PDF input is a 400 (not a 500): it is bad client input,
	// not a server fault. ExtractPDFText's deferred recover already turns the parser's
	// panic on a malformed stream into an error, so every failure lands here.
	text, err := resume.ExtractPDFText(data)
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
	}
	return resumeUpload{Data: data, ContentType: "application/pdf", Text: text}, nil
}
