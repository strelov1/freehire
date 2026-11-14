package resumeextract

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// recordingModel captures the user prompt so a test can assert on what reaches the model.
type recordingModel struct {
	resp   string
	prompt string
	calls  int
}

func (m *recordingModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	m.calls++
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if t, ok := part.(llms.TextContent); ok {
				m.prompt += t.Text
			}
		}
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.resp}}}, nil
}
func (*recordingModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

func TestExtract_ContactsWithoutDetector(t *testing.T) {
	cv := "João da Silva\nLisbon, Portugal\njoao@example.com +351 912 345 678 https://github.com/joaosilva\nBackend Engineer, Acme, 05/2016 - 06/2024"
	m := &recordingModel{resp: `{"full_name":"João da Silva","email":"joao@example.com","phone":"+351 912 345 678","location":"Lisbon, Portugal","links":["https://github.com/joaosilva"],"experience":[{"title":"Backend Engineer","company":"Acme","start":{"year":2016,"month":5},"end":{"year":2024,"month":6}}]}`}
	got, err := NewExtractor(llm.NewWithModel(m)).Extract(context.Background(), cv)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got.FullName != "João da Silva" || got.Email != "joao@example.com" || got.Phone != "+351 912 345 678" || got.Location != "Lisbon, Portugal" {
		t.Fatalf("contacts not recovered from original CV: %+v", got)
	}
	if len(got.Links) != 1 || got.Links[0] != "https://github.com/joaosilva" {
		t.Fatalf("links = %v", got.Links)
	}
	if !strings.Contains(m.prompt, cv) {
		t.Fatal("model did not receive the original CV with contacts and dates intact")
	}
	if m.calls != 1 {
		t.Fatalf("model calls = %d, want one for a valid extraction", m.calls)
	}
	if len(got.Experience) != 1 || got.Experience[0].Start == nil || got.Experience[0].End == nil || got.Experience[0].Start.Year != 2016 || got.Experience[0].Start.Month != 5 || got.Experience[0].End.Year != 2024 || got.Experience[0].End.Month != 6 {
		t.Fatalf("dates = %+v", got.Experience)
	}
}

func TestExtract_MissingContactsStayEmpty(t *testing.T) {
	m := &recordingModel{resp: `{"full_name":null,"email":null,"phone":null,"location":null,"links":null,"summary":"Backend engineer."}`}
	got, err := NewExtractor(llm.NewWithModel(m)).Extract(context.Background(), "Backend engineer.")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got.FullName != "" || got.Email != "" || got.Phone != "" || got.Location != "" || len(got.Links) != 0 {
		t.Fatalf("missing contacts must stay empty: %+v", got)
	}
	if got.Summary != "Backend engineer." {
		t.Fatalf("summary = %q", got.Summary)
	}
}

func TestExtract_BoundsOriginalCV(t *testing.T) {
	m := &recordingModel{resp: `{}`}
	cv := strings.Repeat("Ж", maxCVRunes) + "TRAILING_TEXT_OUTSIDE_LIMIT"
	if _, err := NewExtractor(llm.NewWithModel(m)).Extract(context.Background(), cv); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if strings.Contains(m.prompt, "TRAILING_TEXT_OUTSIDE_LIMIT") || !strings.Contains(m.prompt, strings.Repeat("Ж", maxCVRunes)) {
		t.Fatal("CV must be bounded by Unicode characters before the model call")
	}
}

func TestExtract_KeepsContactsOutOfProfessionalFacts(t *testing.T) {
	m := &recordingModel{resp: `{
		"full_name":" Ann ","email":" ann@example.com ","phone":" +1 415 555 0123 ",
		"links":[" https://github.com/ann "],
		"summary":"Ann is a backend engineer.",
		"experience":[{"company":"Acme","title":"Engineer","highlights":[
			"Improved annual reporting.",
			"Published the SDK at https://github.com/ann/sdk",
			"Contact ann@example.com for references.",
			"Reach me at +1 415 555 0123."
		]}]
	}`}
	cv := "Ann ann@example.com +1 415 555 0123 https://github.com/ann\nEngineer at Acme."
	got, err := NewExtractor(llm.NewWithModel(m)).Extract(context.Background(), cv)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got.FullName != "Ann" || got.Email != "ann@example.com" || len(got.Links) != 1 {
		t.Fatalf("contact extraction was damaged: %+v", got)
	}
	if got.Summary != "" {
		t.Errorf("identity-bearing summary survived: %q", got.Summary)
	}
	if len(got.Experience) != 1 || len(got.Experience[0].Highlights) != 1 || got.Experience[0].Highlights[0] != "Improved annual reporting." {
		t.Fatalf("contact-bearing facts survived or unrelated words were removed: %+v", got.Experience)
	}
	professional, err := json.Marshal(got.Professional())
	if err != nil {
		t.Fatal(err)
	}
	for _, contact := range []string{"ann@example.com", "+1 415 555 0123", "github.com/ann"} {
		if strings.Contains(string(professional), contact) {
			t.Errorf("professional projection contains contact %q", contact)
		}
	}
}
