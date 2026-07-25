package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/skilltag"
)

// matchTextRequest is a job posting scraped from an arbitrary page: its heading
// and visible text. No catalog job is required.
type matchTextRequest struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// textMatch derives skills from a posting's title+text with the same skilltag
// pass ingest uses for job.Skills, then scores their coverage by the profile.
// Deterministic, no LLM.
func textMatch(title, text string, profileSkills []string) jobmatch.JobMatch {
	skills := skilltag.Parse(title + "\n" + text)
	return jobmatch.Compute(skills, profileSkills)
}

// MatchText scores how well an arbitrary job posting (title + scraped text) is
// covered by the authenticated caller's profile skills — the same deterministic
// coverage as GET /jobs/:slug/match, but for a page that need not be in the
// freehire catalog. This is what lets the browser extension show a match on any
// job page. Cookie or API key; a caller without a profile is a 404.
func (a *API) MatchText(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in matchTextRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	profile, err := a.userProfile.Get(c.Context(), userID)
	if err != nil {
		return profileError(err)
	}
	return c.JSON(fiber.Map{"data": textMatch(in.Title, in.Text, profile.Skills)})
}
