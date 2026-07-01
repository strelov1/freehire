package handler

import (
	"bytes"
	"io"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ledongthuc/pdf"

	"github.com/strelov1/freehire/internal/skilltag"
)

// resumeTextRequest is the JSON body for the pasted-text path.
type resumeTextRequest struct {
	Text string `json:"text"`
}

// ExtractResumeSkills turns an uploaded resume into canonical skill slugs via the
// deterministic skilltag dictionary. It accepts a PDF (multipart/form-data field
// "file") or plain text (application/json {text}), dispatched by Content-Type. The
// resume is never persisted or logged — only the resulting slugs are returned, so a
// caller can merge them into a search profile. Behind RequireAuth (cookie-only), like
// the profiles feature it feeds. Oversize bodies are rejected by the server's global
// BodyLimit (413) before this handler runs.
func (a *API) ExtractResumeSkills(c *fiber.Ctx) error {
	if _, err := requireUserID(c); err != nil {
		return err
	}

	text, err := resumeText(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "resume is empty")
	}

	skills := skilltag.Parse(text)
	if skills == nil {
		skills = []string{}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"skills": skills}})
}

// resumeText reads the resume text from the request: JSON {text} for the paste path,
// otherwise the "file" part parsed as a PDF.
func resumeText(c *fiber.Ctx) (string, error) {
	if strings.HasPrefix(c.Get(fiber.HeaderContentType), fiber.MIMEApplicationJSON) {
		var in resumeTextRequest
		if err := c.BodyParser(&in); err != nil {
			return "", fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		return in.Text, nil
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "missing resume file")
	}
	f, err := fh.Open()
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "cannot read resume file")
	}
	defer f.Close()
	return pdfText(f, fh.Size)
}

// pdfText extracts the plain text from a PDF. An undecodable or non-PDF input is a 400
// (not a 500): it is bad client input, not a server fault. multipart.File satisfies
// io.ReaderAt, so the upload streams straight in without buffering the whole file.
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
