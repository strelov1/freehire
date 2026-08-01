package handler

import (
	"context"
	"errors"
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

// fiberCtx makes a throwaway request context; candidateProfile only needs c.Context().
func fiberCtx() *fiber.Ctx {
	return fiber.New().AcquireCtx(&fasthttp.RequestCtx{})
}

func TestCandidateProfileComesFromTheBank(t *testing.T) {
	bank := &stubProfiler{profile: resumeextract.Professional{
		Headline: "Senior Backend Engineer",
		Experience: []resumeextract.Experience{
			{Company: "RingCentral", Title: "SWE", Highlights: []string{"Cut latency 20s to 1s"}},
		},
	}}
	h := &matchHandlers{bank: bank}

	got := h.candidateProfile(fiberCtx(), 7)

	if len(got.Experience) != 1 || got.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked role", got.Experience)
	}
}

// The one state that stops the chain, and the reason it must NOT fall back: a candidate
// whose bank failed to seed gets no analysis, loudly, instead of an analysis quietly
// scored against a work history nothing owns. The producer reports the empty history rather
// than an empty string; matchanalysis is what refuses to run on it.
func TestCandidateProfileHasNoExperienceWhenTheBankIsEmpty(t *testing.T) {
	h := &matchHandlers{bank: &stubProfiler{profile: resumeextract.Professional{
		Headline:  "Senior Backend Engineer",
		Education: []resumeextract.Education{{Degree: "BSc"}},
	}}}

	if got := h.candidateProfile(fiberCtx(), 7); len(got.Experience) != 0 {
		t.Errorf("experience = %+v, want none — an empty bank means no analysis", got.Experience)
	}
}

func TestCandidateProfileSurvivesAFailingBank(t *testing.T) {
	h := &matchHandlers{bank: &stubProfiler{err: errors.New("database down")}}

	if got := h.candidateProfile(fiberCtx(), 7); len(got.Experience) != 0 {
		t.Errorf("experience = %+v, want none on a bank error", got.Experience)
	}
}

// A handler with neither a bank nor the queries to build one has nothing to say. But the
// nil FIELD alone must not mean that: a handler holding queries reaches the bank anyway,
// because "not wired" and "this candidate has no experience" are different statements and
// collapsing them would deny someone their fit analysis over an assembly detail.
func TestCandidateProfileWithNothingToReadFrom(t *testing.T) {
	h := &matchHandlers{}

	if got := h.candidateProfile(fiberCtx(), 7); len(got.Experience) != 0 {
		t.Errorf("experience = %+v, want none when there is no bank and no queries", got.Experience)
	}
	if h.candidateBank() != nil {
		t.Error("a handler with no queries produced a bank")
	}
}
