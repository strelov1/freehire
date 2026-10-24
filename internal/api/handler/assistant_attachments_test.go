package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// attachmentApp mounts the extraction route behind a stand-in sign-in, with the PDF reader
// replaced so the test needs no pdftotext.
func attachmentApp(extract func([]byte) (string, error)) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h := &assistantHandlers{extractPDFText: extract}
	app.Post("/assistant/attachments", signedIn(7), h.PostAssistantAttachment)
	return app
}

func postAttachment(t *testing.T, app *fiber.App, filename string, body []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/assistant/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestAttachmentExtractsAPDFsTextWithoutStoringIt(t *testing.T) {
	app := attachmentApp(func(data []byte) (string, error) {
		if !bytes.HasPrefix(data, []byte("%PDF")) {
			t.Errorf("extractor got %q, want the uploaded bytes", data)
		}
		return "  Senior Go engineer, Berlin  \n", nil
	})
	status, out := postAttachment(t, app, `C:\docs\vacancy.pdf`, []byte("%PDF-1.7 ..."))
	if status != http.StatusOK {
		t.Fatalf("status = %d (%v), want 200", status, out)
	}
	data, _ := out["data"].(map[string]any)
	if data["name"] != "vacancy.pdf" || data["text"] != "Senior Go engineer, Berlin" || data["truncated"] != false {
		t.Errorf("data = %v, want the sanitised name, trimmed text, not truncated", data)
	}
}

func TestAttachmentTruncatesALongDocumentAndSaysSo(t *testing.T) {
	app := attachmentApp(func([]byte) (string, error) {
		return strings.Repeat("x", assistant.MaxAttachmentRunes+500), nil
	})
	status, out := postAttachment(t, app, "book.pdf", []byte("%PDF-1.4"))
	if status != http.StatusOK {
		t.Fatalf("status = %d (%v), want 200", status, out)
	}
	data, _ := out["data"].(map[string]any)
	if data["truncated"] != true || len(data["text"].(string)) != assistant.MaxAttachmentRunes {
		t.Errorf("truncated = %v, len = %d; want the text cut to the ceiling and flagged", data["truncated"], len(data["text"].(string)))
	}
}

func TestAttachmentRefusals(t *testing.T) {
	cases := []struct {
		name    string
		body    []byte
		extract func([]byte) (string, error)
		want    int
	}{
		{"not a PDF", []byte("PK\x03\x04 a docx"), func([]byte) (string, error) { return "never", nil }, http.StatusUnsupportedMediaType},
		{"corrupt PDF", []byte("%PDF-broken"), func([]byte) (string, error) { return "", errors.New("bad xref") }, http.StatusBadRequest},
		{"scan with no text layer", []byte("%PDF-1.4"), func([]byte) (string, error) { return " \n ", nil }, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, out := postAttachment(t, attachmentApp(tc.extract), "f.pdf", tc.body)
			if status != tc.want {
				t.Errorf("status = %d (%v), want %d", status, out, tc.want)
			}
		})
	}
}

func TestParseTurnRequest(t *testing.T) {
	doc := assistant.Attachment{Name: "cv.pdf", Text: "Jane Doe"}

	t.Run("text alone", func(t *testing.T) {
		prompt, docs, err := parseTurnRequest(assistantTurnRequest{Text: "  hi  "}, 100)
		if err != nil || prompt != "hi" || docs != nil {
			t.Errorf("got (%q, %v, %v), want the trimmed text and no documents", prompt, docs, err)
		}
	})
	t.Run("a document alone is a message", func(t *testing.T) {
		prompt, docs, err := parseTurnRequest(assistantTurnRequest{Attachments: []assistant.Attachment{doc}}, 100)
		if err != nil || prompt != "" || len(docs) != 1 || docs[0].Name != "cv.pdf" {
			t.Errorf("got (%q, %v, %v), want an empty prompt carrying the document", prompt, docs, err)
		}
	})
	t.Run("the name is sanitised server-side too", func(t *testing.T) {
		_, docs, err := parseTurnRequest(assistantTurnRequest{Attachments: []assistant.Attachment{{Name: `a"><b.pdf`, Text: "x"}}}, 100)
		if err != nil || docs[0].Name != "ab.pdf" {
			t.Errorf("got (%v, %v), want the name stripped of delimiter characters", docs, err)
		}
	})

	refused := map[string]assistantTurnRequest{
		"nothing at all":        {Text: "   "},
		"text past the ceiling": {Text: strings.Repeat("a", 101)},
		"too many documents":    {Text: "x", Attachments: []assistant.Attachment{doc, doc, doc, doc}},
		"an empty document":     {Text: "x", Attachments: []assistant.Attachment{{Name: "a.pdf", Text: " "}}},
		// A client cannot raise the ceiling by skipping the extraction endpoint.
		"an oversized document": {Text: "x", Attachments: []assistant.Attachment{{Name: "a.pdf", Text: strings.Repeat("a", assistant.MaxAttachmentRunes+1)}}},
	}
	for name, in := range refused {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseTurnRequest(in, 100); err == nil {
				t.Error("accepted, want a refusal")
			}
		})
	}
}
