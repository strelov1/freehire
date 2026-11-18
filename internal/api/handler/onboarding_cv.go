package handler

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// onboardingExtractTimeout bounds the in-request model parse. It stays under nginx's
// default 60 s proxy_read_timeout on /api/, so a slow model degrades the answer to the
// dictionary profile instead of the gateway cutting the request off.
const onboardingExtractTimeout = 45 * time.Second

// onboardingMaxCompanies is how many employers the onboarding summary names.
const onboardingMaxCompanies = 4

// onboardingCVResponse is what the CV-first onboarding reads from a CV. The structured
// half comes from the model and is empty (StructuredAvailable false) when the model is
// unconfigured, failed or ran out of time; the dictionary half and the ATS report are
// always there. No contact field is part of the shape, the name is cut to the first name
// (the page greets the visitor, it does not identify them), and the residence goes out
// only as the dictionary's country code — the address span it comes from can be a street
// address in any shape, so no form of its text is sent.
type onboardingCVResponse struct {
	StructuredAvailable bool             `json:"structured_available"`
	FirstName           string           `json:"first_name"`
	Headline            string           `json:"headline"`
	Country             string           `json:"country"`
	TotalYears          int              `json:"total_years"`
	Companies           []string         `json:"companies"`
	Skills              []string         `json:"skills"`
	Categories          []string         `json:"categories"`
	Seniority           string           `json:"seniority,omitempty"`
	Report              *atscheck.Report `json:"report"`
}

// ParseOnboardingCV reads a CV for the CV-first onboarding, signed in or not, and stores
// nothing: no bytes, no fields, no account. The visitor's browser keeps the file and
// hands it to the stored-CV upload once an account exists.
//
// It is the one place an ANONYMOUS visitor gets a model call, and it gets it free — a
// named plan.WithFreeAI exception (internal/ai/plan/AGENTS.md). A guest spends the
// service credential, never the binding: llmkey asked about account 0 would mint a
// credential for an account that does not exist (see search_intent.go). The per-IP
// bound on guests lives in the operations gateway, which owns every request budget.
func (h *resumeHandlers) ParseOnboardingCV(c *fiber.Ctx) error {
	userID, signedIn := auth.UserID(c)
	if !signedIn {
		if err := refuseEncodedBody(c); err != nil {
			return err
		}
	}
	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}

	profile := resumeProfile(up.Text)
	// The report is scored against the CV's own skills with no role market: the
	// onboarding asks for the role a step later, and the score must not wait on search.
	report := atscheck.Score(up.Text, profile.Skills, nil)
	out := onboardingCVResponse{
		Companies:  []string{},
		Skills:     profile.Skills,
		Categories: profile.Categories,
		Seniority:  profile.Seniority,
		Report:     &report,
	}
	if st, ok := h.onboardingStructured(c.UserContext(), userID, signedIn, up.Text); ok {
		out.StructuredAvailable = true
		out.FirstName = firstName(st.FullName)
		out.Headline = st.Headline
		out.TotalYears = st.TotalYears
		out.Companies = recentCompanies(st.Experience, onboardingMaxCompanies)
		if countries, _, _ := resume.DeriveGeography(st.Location); len(countries) > 0 {
			out.Country = countries[0]
		}
	}
	return c.JSON(fiber.Map{"data": out})
}

// onboardingStructured runs the model parse, reporting false (and logging, never the CV)
// on anything short of a structure: no extractor, a credential refusal, a model error
// or the deadline.
func (h *resumeHandlers) onboardingStructured(ctx context.Context, userID int64, signedIn bool, text string) (resumeextract.Structured, bool) {
	if h.structuredExtractor == nil || !h.structuredExtractor.Enabled() {
		return resumeextract.Structured{}, false
	}
	ctx, cancel := context.WithTimeout(plan.WithFreeAI(ctx), onboardingExtractTimeout)
	defer cancel()

	caller := llm.NewCaller("", nil, llm.Feature(tagCVOnboard))
	if signedIn {
		var err error
		if caller, err = h.llm.caller(ctx, userID, llm.Feature(tagCVOnboard)); err != nil {
			log.Printf("onboarding cv: user %d: %v", userID, err)
			return resumeextract.Structured{}, false
		}
	}
	st, err := h.structuredExtractor.As(caller).Extract(ctx, text)
	if err != nil {
		log.Printf("onboarding cv: user %d: %v", userID, err)
		return resumeextract.Structured{}, false
	}
	return st, true
}

// firstName is the first word of a full name.
func firstName(full string) string {
	if fields := strings.Fields(full); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// recentCompanies names up to n distinct employers in the order the CV lists them, which
// for a CV is most recent first.
func recentCompanies(exp []resumeextract.Experience, n int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, e := range exp {
		name := strings.TrimSpace(e.Company)
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
		if len(out) == n {
			break
		}
	}
	return out
}
