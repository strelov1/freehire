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
	"github.com/strelov1/freehire/internal/pii"
)

// ErrDisabled is returned by Extract when the LLM is not configured (nil client), so a
// best-effort caller can skip persisting a structured résumé without treating it as a
// failure.
var ErrDisabled = errors.New("resumeextract: llm not configured")

// Extractor derives a Structured résumé over an llm.Client. It needs both a client and a
// PII detector: the client parses the semantic fields from the REDACTED CV, and the detector
// both de-identifies the CV and supplies the contact fields. Either missing makes Extract
// return ErrDisabled (fail-closed — no CV reaches the LLM), mirroring the no-op degradation.
type Extractor struct {
	client   *llm.Client
	detector pii.Detector
}

// NewExtractor wraps an llm.Client and the PII detector; either may be nil, which disables
// extraction (ErrDisabled).
// As returns an extractor that runs on a different client, so one extraction can be spent
// under the caller's own gateway credential. The PII detector travels with it: redaction
// is not the credential's business and must hold whoever is paying. Nil-safe both ways.
func (e *Extractor) As(client *llm.Client) *Extractor {
	if e == nil || client == nil {
		return e
	}
	clone := *e
	clone.client = client

	return &clone
}

func NewExtractor(client *llm.Client, detector pii.Detector) *Extractor {
	return &Extractor{client: client, detector: detector}
}

// Enabled reports whether extraction can run: both the client and the detector are present.
// Without the detector the CV cannot be de-identified, so extraction is disabled rather than
// run in the clear.
func (e *Extractor) Enabled() bool { return e != nil && e.client != nil && e.detector != nil }

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
	// De-identify the CV before it reaches the LLM. Fail-closed: a detector error means no
	// CV is sent (the caller degrades best-effort, persisting no structured résumé).
	red, err := pii.Build(ctx, cvText, pii.Contacts{}, e.detector)
	if err != nil {
		return Structured{}, fmt.Errorf("resumeextract: pii: %w", err)
	}
	schema, err := requestSchema()
	if err != nil {
		return Structured{}, err
	}
	raw, err := e.client.GenerateJSON(ctx, systemPrompt, userPrompt(red.Redact(cvText)),
		llm.WithSchema(schemaName, schema))
	if err != nil {
		return Structured{}, fmt.Errorf("resumeextract: generate: %w", err)
	}
	var s Structured
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Structured{}, fmt.Errorf("resumeextract: parse: %w", err)
	}
	// Contact fields come from deterministic detection, NOT the model — it only ever saw the
	// redacted CV, so it cannot (and must not) supply them. Location is included: the
	// candidate's residence is typically an ADDRESS span masked before the model runs.
	c := red.Contacts()
	s.FullName, s.Email, s.Phone, s.Location, s.Links = c.FullName, c.Email, c.Phone, c.Location, c.Links
	s.Sanitize()
	return s, nil
}

// maxCVRunes bounds the CV text sent to the model — a long CV covers its substance well
// within this, and the cap keeps the call responsive (mirrors matchanalysis's input bounds).
const maxCVRunes = 12000

// systemPrompt pairs with the request schema (schema.go): the schema fixes the shape,
// this fixes the meaning. Every field is required by the schema, so "I cannot fill
// this" is expressed as null — never as an absent key, which the schema forbids and a
// model with no legal way to decline answers by inventing.
const systemPrompt = `You extract a structured résumé from raw CV text and return ONLY a JSON object.
Rules:
- Extract ONLY facts stated in the CV. Never invent or infer a field that is not present — return null for it.
- The CV has been de-identified: contact details (name, email, phone, residence, links) are handled
  separately and appear as [REDACTED_...] placeholders. Never copy a placeholder into any field.
- Fields: headline (current role/title line), summary (1-3 sentences),
  total_years (years of professional experience as a plain number written as a string, e.g. "6";
    best estimate; "0" if unclear),
  experience (array of {title, company, location, start, end, summary, highlights, stack}; keep dates as
    written, e.g. "2021-03" or "Present"; summary is the one-line company/role context; highlights is the
    array of achievement bullet points for that role, each a full sentence copied faithfully from the CV;
    stack is the array of technologies listed for that role, e.g. from a "Stack:" line;
    location here is the employer's office for that role, not the candidate's residence),
  education (array of {degree, institution, year}), languages (array of strings),
  skills (array of strings — technologies/tools stated in the CV, properly cased, e.g. "Go", "PostgreSQL", "Kafka"),
  certifications (array of strings — professional certifications/licenses the CV states the person holds, e.g. "AWS Certified Solutions Architect", "CISSP", "PMP"),
  projects (array of {name, link, highlights} — personal/side projects with their bullet points).
- Return null for any field you cannot fill from the CV, and null for every field if the text is not a résumé.`

func userPrompt(cvText string) string {
	return "CV text:\n" + clip(cvText, maxCVRunes)
}
