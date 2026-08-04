package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jdresolve"
)

// jdResolveHandlers serves the entry point that turns an existing job's slug, an
// external URL, or pasted JD text into a job usable by the tailor workspace.
type jdResolveHandlers struct {
	resolver *jdresolve.Resolver
}

func newJDResolveHandlers(resolver *jdresolve.Resolver) *jdResolveHandlers {
	return &jdResolveHandlers{resolver: resolver}
}

func (h *jdResolveHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/me/jd/resolve", mw.cookie, jdURLLimiter(), h.Resolve)
}

// jdURLLimiterPerHour bounds how many outbound-fetching url requests one user may submit
// to this endpoint per hour — sized the same as contributionsPerHour since the underlying
// risk (an unbounded endpoint fetching a caller-supplied URL is an outbound-fetch amplifier
// and a timing oracle for public hosts) is identical. A DEDICATED limiter, not
// contributionLimiter itself: job_slug and text requests make no outbound fetch at all, so
// sharing /jobs/resolve's counter would throttle a tailoring session that never once
// touched the network, and vice versa.
const jdURLLimiterPerHour = contributionsPerHour

// jdURLLimiter throttles only the url branch: Next reads the already-buffered body (cheap,
// and it does not consume it — c.BodyParser in Resolve reads it again the normal way) and
// skips rate-limiting entirely when it names no url, so a job_slug or pasted-text request
// never spends this budget. A body that fails to parse here is not limited either; Resolve's
// own BodyParser call rejects it as a 400 right after, unmetered.
func jdURLLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        jdURLLimiterPerHour,
		Expiration: time.Hour,
		Next: func(c *fiber.Ctx) bool {
			var in jdResolveRequest
			if err := json.Unmarshal(c.Body(), &in); err != nil {
				return true
			}
			return strings.TrimSpace(in.URL) == ""
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := auth.UserID(c); ok {
				return "user:" + strconv.FormatInt(id, 10)
			}
			return "ip:" + c.IP()
		},
	})
}

// jdResolveRequest is exactly one of JobSlug/URL/Text. Title/Company are optional hints
// used only alongside Text (they improve classification/skill derivation but are not
// required — see internal/jobderive).
type jdResolveRequest struct {
	JobSlug string `json:"job_slug"`
	URL     string `json:"url"`
	Text    string `json:"text"`
	Title   string `json:"title"`
	Company string `json:"company"`
}

type jdResolveResponse struct {
	JobSlug string `json:"job_slug"`
}

// jdResolveError maps the jdresolve sentinels onto HTTP statuses. Anything else (e.g. a
// DB failure) falls through to the central RenderError as a 500.
func jdResolveError(err error) error {
	switch {
	case errors.Is(err, jdresolve.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, jdresolve.ErrUnreadableURL):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "could not read a vacancy from that url")
	default:
		return err
	}
}

// Resolve turns the request's job_slug/url/text into a job usable by the tailor
// workspace. Exactly one of the three is required; anything else is a 400 before any
// resolution is attempted.
func (h *jdResolveHandlers) Resolve(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in jdResolveRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	in.JobSlug = strings.TrimSpace(in.JobSlug)
	in.URL = strings.TrimSpace(in.URL)
	in.Text = strings.TrimSpace(in.Text)

	present := 0
	for _, v := range []string{in.JobSlug, in.URL, in.Text} {
		if v != "" {
			present++
		}
	}
	if present != 1 {
		return fiber.NewError(fiber.StatusBadRequest, "exactly one of job_slug, url, or text is required")
	}

	slug, err := h.resolver.Resolve(c.Context(), userID, jdresolve.Request{
		JobSlug: in.JobSlug,
		URL:     in.URL,
		Text:    in.Text,
		Title:   strings.TrimSpace(in.Title),
		Company: strings.TrimSpace(in.Company),
	})
	if err != nil {
		return jdResolveError(err)
	}
	return c.JSON(fiber.Map{"data": jdResolveResponse{JobSlug: slug}})
}
