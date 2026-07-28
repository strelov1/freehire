package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// stubProfiler stands in for the experience bank so the fit chain's candidate context can
// be tested without a database.
type stubProfiler struct {
	profile resumeextract.Professional
	err     error
	gotSt   resumeextract.Structured
	calls   int
}

func (s *stubProfiler) Professional(_ context.Context, _ int64, st resumeextract.Structured) (resumeextract.Professional, error) {
	s.calls++
	s.gotSt = st
	return s.profile, s.err
}

// fiberCtx makes a throwaway request context; candidateProfileJSON only needs c.Context().
func fiberCtx() *fiber.Ctx {
	return fiber.New().AcquireCtx(&fasthttp.RequestCtx{})
}

func TestCandidateProfileJSONComesFromTheBank(t *testing.T) {
	bank := &stubProfiler{profile: resumeextract.Professional{
		Headline: "Senior Backend Engineer",
		Experience: []resumeextract.Experience{
			{Company: "RingCentral", Title: "SWE", Highlights: []string{"Cut latency 20s to 1s"}},
		},
	}}
	h := &matchHandlers{bank: bank}

	got := h.candidateProfileJSON(fiberCtx(), 7)

	if got == "" {
		t.Fatal("candidate context is empty")
	}
	var decoded resumeextract.Professional
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Experience) != 1 || decoded.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked role", decoded.Experience)
	}
}

// The one state that stops the chain, and the reason it must NOT fall back: a candidate
// whose bank failed to seed gets no analysis, loudly, instead of an analysis quietly
// scored against a work history nothing owns.
func TestCandidateProfileJSONIsEmptyWhenTheBankIs(t *testing.T) {
	h := &matchHandlers{bank: &stubProfiler{profile: resumeextract.Professional{
		Headline:  "Senior Backend Engineer",
		Education: []resumeextract.Education{{Degree: "BSc"}},
	}}}

	if got := h.candidateProfileJSON(fiberCtx(), 7); got != "" {
		t.Errorf("candidate context = %q, want empty — an empty bank means no analysis", got)
	}
}

func TestCandidateProfileJSONSurvivesAFailingBank(t *testing.T) {
	h := &matchHandlers{bank: &stubProfiler{err: errors.New("database down")}}

	if got := h.candidateProfileJSON(fiberCtx(), 7); got != "" {
		t.Errorf("candidate context = %q, want empty on a bank error", got)
	}
}

func TestCandidateProfileJSONWithoutABankIsEmpty(t *testing.T) {
	h := &matchHandlers{}

	if got := h.candidateProfileJSON(fiberCtx(), 7); got != "" {
		t.Errorf("candidate context = %q, want empty when there is no bank", got)
	}
}

// Contacts must not reach the model. The composition keeps the whitelist, and this pins it
// at the boundary the fit chain actually crosses.
func TestCandidateProfileJSONCarriesNoContacts(t *testing.T) {
	h := &matchHandlers{bank: &stubProfiler{profile: resumeextract.Professional{
		Headline:   "Senior Backend Engineer",
		Experience: []resumeextract.Experience{{Company: "RingCentral"}},
	}}}

	got := h.candidateProfileJSON(fiberCtx(), 7)
	for _, key := range []string{"full_name", "email", "phone", "links"} {
		if strings.Contains(got, key) {
			t.Errorf("the candidate context carries a %q field", key)
		}
	}
}
