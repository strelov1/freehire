package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// The CV routes answer through the contract (typespec/cv.tsp), whose generated types hold
// the document and its parts as raw JSON. These are the typed shapes the tests read the
// answers back into, with the same json tags the routes always sent.

type cvMetaResponse struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	TemplateID string    `json:"template_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type cvResponse struct {
	cvMetaResponse
	AgentSessionID     string              `json:"agent_session_id"`
	Document           cv.Document         `json:"document"`
	AutopilotReport    []cv.AutopilotEntry `json:"autopilot_report,omitempty"`
	TracerLinksEnabled bool                `json:"tracer_links_enabled"`
}

type cvBaseResponse struct {
	cvMetaResponse
	IsDefault  bool `json:"is_default"`
	HandEdited bool `json:"hand_edited"`
}

type createCVRequest struct {
	Title      string `json:"title"`
	TemplateID string `json:"template_id"`
	Seed       bool   `json:"seed"`
}

type updateCVRequest struct {
	Title      string      `json:"title"`
	TemplateID string      `json:"template_id"`
	Document   cv.Document `json:"document"`
}

type setCVTemplateRequest struct {
	TemplateID string `json:"template_id"`
}

type setCVAppearanceDefaultsRequest struct {
	TemplateID string     `json:"template_id"`
	Style      cv.Style   `json:"style"`
	Margins    cv.Margins `json:"margins"`
}

type tailorCVRequest struct {
	JobSlug  string `json:"job_slug"`
	BaseCVID string `json:"base_cv_id,omitempty"`
}

type tailorCVResponse struct {
	TailorCVID       string                  `json:"tailor_cv_id"`
	BaseCVID         string                  `json:"base_cv_id"`
	Analysis         *matchanalysis.Analysis `json:"analysis"`
	SessionID        string                  `json:"session_id"`
	ColdStartRunning bool                    `json:"cold_start_running"`
}

type tailorSessionResponse struct {
	TailorCVID string `json:"tailor_cv_id"`
	BaseCVID   string `json:"base_cv_id"`
	SessionID  string `json:"session_id"`
}

type baseSessionResponse struct {
	BaseCVID  string `json:"base_cv_id"`
	SessionID string `json:"session_id"`
}

type atsDeltaResponse struct {
	Available bool            `json:"available"`
	Reason    string          `json:"reason,omitempty"`
	BaseCVID  string          `json:"base_cv_id,omitempty"`
	Delta     *atscheck.Delta `json:"delta,omitempty"`
}

type cvJobMatchResponse struct {
	Available bool           `json:"available"`
	Reason    string         `json:"reason,omitempty"`
	Score     *cvmatch.Score `json:"score,omitempty"`
}

// cvContract is the contract handler the CV routes end in, for a test that mounts one.
func cvContract(h *cvHandlers) fiber.Handler {
	return newContract(contractHandlers{cvHandlers: h})
}
