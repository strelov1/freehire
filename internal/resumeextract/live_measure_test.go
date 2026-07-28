//go:build llmlive

// Package-level measurement, not a unit test: it dials the configured model and costs
// real tokens, so it is behind the llmlive tag and never runs in CI.
//
//	LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=… go test -tags=llmlive ./internal/resumeextract/ -run Measure -v
//
// It answers the one question the migration turns on: does constraining the response
// to a schema extract LESS from a real CV than the unconstrained prompt did?
package resumeextract

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/strelov1/freehire/internal/llm"
)

// pdfText shells out rather than importing internal/resume, which imports this
// package — the extraction it would reuse is one exec call wide.
func pdfText(t *testing.T, path string) string {
	t.Helper()

	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not on PATH")
	}

	var out bytes.Buffer
	cmd := exec.Command("pdftotext", "-layout", path, "-")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("pdftotext %s: %v", path, err)
	}

	return out.String()
}

var fixtures = map[string]string{
	"handler/resume_sample": "../handler/testdata/resume_sample.pdf",
	"resume/canva_cid":      "../resume/testdata/canva_cid_identityh.pdf",
}

func liveClient(t *testing.T) *llm.Client {
	t.Helper()

	s := llm.Settings{
		BaseURL: os.Getenv("LLM_BASE_URL"),
		APIKey:  os.Getenv("LLM_API_KEY"),
		Model:   os.Getenv("LLM_MODEL"),
	}
	if !s.Enabled() {
		t.Skip("LLM_BASE_URL/LLM_API_KEY/LLM_MODEL not set")
	}

	c, flush, err := llm.NewClient(s, "measure")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(flush)

	return c
}

// populated counts the fields the extraction actually filled, which is the quantity a
// constrained response could silently reduce.
func populated(s Structured) map[string]int {
	counts := map[string]int{
		"experience":     len(s.Experience),
		"education":      len(s.Education),
		"skills":         len(s.Skills),
		"languages":      len(s.Languages),
		"projects":       len(s.Projects),
		"certifications": len(s.Certifications),
	}

	highlights := 0
	for _, e := range s.Experience {
		highlights += len(e.Highlights)
	}
	counts["highlights"] = highlights

	for name, filled := range map[string]bool{
		"headline":    s.Headline != "",
		"summary":     s.Summary != "",
		"location":    s.Location != "",
		"total_years": s.TotalYears > 0,
	} {
		if filled {
			counts[name] = 1
		} else {
			counts[name] = 0
		}
	}

	return counts
}

func extractOnce(t *testing.T, c *llm.Client, cvText string, opts ...llm.GenOption) Structured {
	t.Helper()

	raw, err := c.GenerateJSON(context.Background(), systemPrompt, userPrompt(cvText), opts...)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var s Structured
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	s.Sanitize()

	return s
}

func TestMeasure_SchemaVersusPlainOnRealCVs(t *testing.T) {
	c := liveClient(t)

	schema, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	for name, path := range fixtures {
		t.Run(name, func(t *testing.T) {
			text := pdfText(t, path)

			plain := populated(extractOnce(t, c, text))
			constrained := populated(extractOnce(t, c, text, llm.WithSchema(schemaName, schema)))

			for _, field := range []string{
				"headline", "summary", "location", "total_years",
				"experience", "highlights", "education", "skills",
				"languages", "projects", "certifications",
			} {
				before, after := plain[field], constrained[field]
				mark := "  "
				switch {
				case after < before:
					mark = "!!"
				case after > before:
					mark = "++"
				}
				t.Logf("%s %-15s plain=%-3d schema=%-3d", mark, field, before, after)
			}
		})
	}
}

// TestMeasure_DumpConstrainedRaw prints what the constrained call actually returns for
// the fields the comparison flagged, so a drop can be explained rather than accepted.
func TestMeasure_DumpConstrainedRaw(t *testing.T) {
	c := liveClient(t)

	schema, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	text := pdfText(t, fixtures["resume/canva_cid"])

	raw, err := c.GenerateJSON(context.Background(), systemPrompt, userPrompt(text),
		llm.WithSchema(schemaName, schema))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, field := range []string{"location", "headline", "total_years"} {
		t.Logf("%-12s => %#v", field, probe[field])
	}
	if skills, ok := probe["skills"].([]any); ok {
		t.Logf("skills(%d) => %v", len(skills), skills)
	}
}
