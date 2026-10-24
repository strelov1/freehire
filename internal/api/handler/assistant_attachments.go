package handler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// attachmentExtractionsPerHour bounds how many documents one caller may have read per
// hour. Each one is a pdftotext subprocess, and nothing about a conversation needs more
// than a handful; the bound is against a script, not a person.
const attachmentExtractionsPerHour = 60

// errAttachmentNoText mirrors the résumé upload's own message for the same cause: the file
// parsed cleanly and carried no text layer.
const errAttachmentNoText = "couldn't read any text from this PDF — it looks like a scan or image. Attach a text-based PDF, or paste the text instead."

// assistantAttachmentResponse is what one extracted document returns: the name and text the
// client sends back with its next message, and whether the text was cut to fit.
type assistantAttachmentResponse struct {
	Name      string `json:"name"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// PostAssistantAttachment reads one PDF the candidate wants to attach to a message and
// returns its text. Nothing is stored — not the file and not the text: the client holds the
// result and sends it with the message, where it becomes part of that message's transcript
// row. A document attached here is therefore never a CV (the stored résumé has its own
// upload), and it reaches the model exactly as if the candidate had pasted it.
//
// Extraction is the résumé upload's own (resume.ExtractPDFText via h.extractPDFText): one
// PDF reader in the codebase, with its timeout and its output ceiling.
func (h *assistantHandlers) PostAssistantAttachment(c *fiber.Ctx) error {
	if _, err := requireUserID(c); err != nil {
		return err
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "missing file")
	}
	f, err := fh.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot read file")
	}
	defer f.Close()
	// The server's BodyLimit bounds this read.
	data, err := io.ReadAll(f)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot read file")
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "only PDF files can be attached")
	}
	text, err := h.extractPDFText(data)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PDF")
	}
	text, truncated := assistant.TruncateAttachmentText(text)
	if text == "" {
		return fiber.NewError(fiber.StatusUnprocessableEntity, errAttachmentNoText)
	}
	return c.JSON(fiber.Map{"data": assistantAttachmentResponse{
		Name:      assistant.AttachmentName(fh.Filename),
		Text:      text,
		Truncated: truncated,
	}})
}

// assistantTurnRequest is the body of a turn: the user's message and the documents
// attached to it, each as PostAssistantAttachment returned it.
type assistantTurnRequest struct {
	Text        string                 `json:"text"`
	Attachments []assistant.Attachment `json:"attachments"`
}

// errEmptyTurn is a message with neither text nor a document.
var errEmptyTurn = errors.New("text is required")

// parseTurnRequest validates a turn's body into the prompt and its documents. Text may be
// empty when a document is attached — "here is my CV" is often said by the file alone — but
// a message carrying neither is refused. The typed text keeps its own ceiling; each
// document is bounded by assistant.NewAttachment, which a client cannot raise by skipping
// the extraction endpoint and posting text directly.
func parseTurnRequest(in assistantTurnRequest, maxPrompt int) (string, []assistant.Attachment, error) {
	prompt := strings.TrimSpace(in.Text)
	if utf8.RuneCountInString(prompt) > maxPrompt {
		return "", nil, errors.New("message is too long")
	}
	if len(in.Attachments) > assistant.MaxAttachments {
		return "", nil, fmt.Errorf("at most %d attachments per message", assistant.MaxAttachments)
	}
	var docs []assistant.Attachment
	for _, a := range in.Attachments {
		doc, err := assistant.NewAttachment(a.Name, a.Text)
		switch {
		case errors.Is(err, assistant.ErrAttachmentEmpty):
			return "", nil, errors.New("an attachment has no text")
		case errors.Is(err, assistant.ErrAttachmentTooLong):
			return "", nil, errors.New("an attachment is too long")
		case err != nil:
			return "", nil, err
		}
		docs = append(docs, doc)
	}
	if prompt == "" && len(docs) == 0 {
		return "", nil, errEmptyTurn
	}
	return prompt, docs, nil
}
