// Package resumeextract derives a typed, sanitized structured résumé from an uploaded
// CV via the LLM, for the read-only profile view and as pre-normalized fit input (see
// the resume-structured-profile change). It is a self-contained, typed prompt unit —
// the sibling of internal/matchanalysis and internal/enrich — kept free of storage concerns so
// the résumé Store stays free of LLM coupling. Sanitize (see structured.go) is both the
// persist guard and the prompt-injection guard for the untrusted CV text: every value is
// bounded and coerced to the contract before it is persisted or served, so the model can
// never introduce an out-of-bounds value.
package resumeextract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/strelov1/freehire/internal/llm"
)

// ErrDisabled is returned by Extract when the LLM is not configured (nil client), so a
// best-effort caller can skip persisting a structured résumé without treating it as a
// failure.
var ErrDisabled = errors.New("resumeextract: llm not configured")

// Extractor derives a Structured résumé over an llm.Client. A nil client (LLM
// unconfigured) makes Extract return ErrDisabled, mirroring matchanalysis.Analyzer's no-op.
type Extractor struct {
	client *llm.Client
}

// NewExtractor wraps an llm.Client; client may be nil (LLM unconfigured).
func NewExtractor(client *llm.Client) *Extractor { return &Extractor{client: client} }

// Enabled reports whether extraction can run (a non-nil client).
func (e *Extractor) Enabled() bool { return e != nil && e.client != nil }

// ModelID returns the underlying model id, so a caller can stamp the produced structure
// with the model that generated it.
func (e *Extractor) ModelID() string { return e.client.ModelID() }

// Extract sends the (bounded) CV text to the model in JSON mode and returns the parsed,
// sanitized structure. Returns ErrDisabled when unconfigured; a transport or parse
// failure is returned as an error (the caller degrades best-effort).
func (e *Extractor) Extract(ctx context.Context, cvText string) (Structured, error) {
	if !e.Enabled() {
		return Structured{}, ErrDisabled
	}
	raw, err := e.client.GenerateJSON(ctx, systemPrompt, userPrompt(cvText))
	if err != nil {
		return Structured{}, fmt.Errorf("resumeextract: generate: %w", err)
	}
	var s Structured
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Structured{}, fmt.Errorf("resumeextract: parse: %w", err)
	}
	s.Sanitize()
	return s, nil
}

// maxCVRunes bounds the CV text sent to the model — a long CV covers its substance well
// within this, and the cap keeps the call responsive (mirrors matchanalysis's input bounds).
const maxCVRunes = 12000

const systemPrompt = `You extract a structured résumé from raw CV text and return ONLY a JSON object.
Rules:
- Extract ONLY facts stated in the CV. Never invent or infer a field that is not present — omit it instead.
- Fields: full_name, headline (current role/title line), location, email, phone, summary (1-3 sentences),
  total_years (integer years of professional experience, best estimate; 0 if unclear),
  experience (array of {title, company, location, start, end, summary, highlights, stack}; keep dates as
    written, e.g. "2021-03" or "Present"; summary is the one-line company/role context; highlights is the
    array of achievement bullet points for that role, each a full sentence copied faithfully from the CV;
    stack is the array of technologies listed for that role, e.g. from a "Stack:" line),
  education (array of {degree, institution, year}), languages (array of strings), links (array of URLs),
  skills (array of strings — technologies/tools stated in the CV, properly cased, e.g. "Go", "PostgreSQL", "Kafka"),
  projects (array of {name, link, highlights} — personal/side projects with their bullet points).
- Omit any field or entry you cannot fill from the CV. Return {} if the text is not a résumé.`

func userPrompt(cvText string) string {
	return "CV text:\n" + clip(cvText, maxCVRunes)
}
