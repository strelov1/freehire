package handler

import (
	"bytes"
	"context"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ledongthuc/pdf"

	"github.com/strelov1/freehire/internal/resume"
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

// ExtractResumeSkills turns an uploaded resume into canonical skill slugs via the
// deterministic skilltag dictionary. It accepts a PDF (multipart/form-data field
// "file") or plain text (application/json {text}), dispatched by Content-Type. When S3
// storage is configured it also stores the résumé once — the single upload point, so the
// verdict's coherence can reuse it without a second upload; storing is best-effort here
// (a hiccup must not fail skill extraction, this endpoint's contract). When storage is
// unconfigured the résumé is parsed and discarded (only the slugs are returned). Behind
// RequireAuth (cookie-only). Oversize bodies are rejected by the server's global
// BodyLimit (413) before this handler runs.
func (a *API) ExtractResumeSkills(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "resume is empty")
	}

	// Résumé path: enable the résumé-scoped acronym tier (e.g. RAG), which stays off
	// for job parsing so it never tags job facets ("RAG status").
	skills := skilltag.Parse(up.Text, skilltag.WithResumeAcronyms())
	if skills == nil {
		skills = []string{}
	}

	if a.resume.Enabled() {
		if _, err := a.resume.Put(c.Context(), userID, up.ContentType, up.Data); err != nil {
			// Best-effort: log (never the résumé bytes) and still return the skills.
			log.Printf("resume: store on extract failed for user %d: %v", userID, err)
		} else {
			// This is the résumé-upload path the app actually uses, so it is where the CV
			// gets embedded for /my/recommendations (background, best-effort — see embedResume).
			go a.embedResume(userID, up.Text)
		}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"skills": skills}})
}

// resumeMetaResponse is the wire shape for résumé status: whether storage is enabled at
// all, whether the caller has a résumé stored, and when it was uploaded (RFC3339, nil
// when absent).
type resumeMetaResponse struct {
	Enabled    bool    `json:"enabled"`
	Present    bool    `json:"present"`
	UploadedAt *string `json:"uploaded_at"`
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
		return fiber.NewError(fiber.StatusBadRequest, "resume is empty")
	}
	meta, err := a.resume.Put(c.Context(), userID, up.ContentType, up.Data)
	if err != nil {
		return err
	}
	// Embed in the background: it must not block the upload response. Embedding is a
	// Meilisearch round-trip that is seconds normally but MINUTES while a full semantic
	// rebuild is monopolizing the engine — long enough to time out the proxy/upload.
	go a.embedResume(userID, up.Text)
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
	vec, model, err := a.search.EmbedText(ctx, strconv.FormatInt(userID, 10), text)
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
	return c.JSON(fiber.Map{"data": newResumeMeta(true, meta)})
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
	// feed the PDF parser. The server's 1MB BodyLimit bounds this.
	data, err := io.ReadAll(f)
	if err != nil {
		return resumeUpload{}, fiber.NewError(fiber.StatusBadRequest, "cannot read resume file")
	}
	text, err := pdfText(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return resumeUpload{}, err
	}
	return resumeUpload{Data: data, ContentType: "application/pdf", Text: text}, nil
}

// pdfText extracts the plain text from a PDF. An undecodable or non-PDF input is a 400
// (not a 500): it is bad client input, not a server fault. bytes.Reader satisfies
// io.ReaderAt, so the buffered upload parses without a second copy.
//
// ledongthuc/pdf can panic (not just error) on a malformed content stream, so a deferred
// recover maps that to the same 400 — a corrupt upload must not surface as a server error.
func pdfText(r io.ReaderAt, size int64) (text string, err error) {
	defer func() {
		if p := recover(); p != nil {
			text, err = "", fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
		}
	}()

	rd, err := pdf.NewReader(r, size)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
	}
	tr, err := rd.GetPlainText()
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tr); err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
	}
	return buf.String(), nil
}
