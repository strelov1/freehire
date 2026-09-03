package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/linkedinprofile"
	"github.com/strelov1/freehire/internal/dict/location"
)

// linkedInReader is the handler's view of the profile reader: one call, three outcomes.
// Declared here rather than taken from the package so a test can answer without a network,
// and so nothing about fetching leaks into the handler's own concerns.
type linkedInReader interface {
	Fetch(ctx context.Context, input string) (linkedinprofile.Profile, error)
}

type linkedInHandlers struct {
	reader linkedInReader
}

func newLinkedInHandlers() *linkedInHandlers {
	return &linkedInHandlers{reader: linkedinprofile.NewClient()}
}

func (h *linkedInHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only, like the CV extraction it sits beside: this is a step in the signed-in
	// user's own onboarding, not something an API key has business triggering.
	//
	// It shares mw.outboundFetch with the other routes that make the server fetch a
	// caller-supplied URL, rather than carrying a limiter of its own. That is the point of
	// that middleware: a per-route limiter would hand the same user a fresh budget on every
	// such endpoint, which is precisely the budget that is supposed to be shared. Mounted
	// after mw.cookie so the key resolves to the user rather than falling back to the
	// address.
	api.Post("/me/linkedin/import", mw.cookie, mw.outboundFetch, h.ImportLinkedInProfile)
}

type linkedInImportRequest struct {
	URL string `json:"url"`
}

// ImportLinkedInProfile reads the caller's public LinkedIn profile and returns the same
// facets a CV would yield, plus the location and the display fields the wizard shows back.
//
// It stores NOTHING. In particular it does not mark the account as having a CV: CV presence
// is the onboarding page's own redirect gate, and an import that quietly satisfied that gate
// would stop prompting a user who still has no CV. The values reach the server only through
// the wizard's existing single profile save.
//
// Facets come from resumeProfile — the same helper /me/resume/extract runs — so a headline
// and a CV carrying the same words can never resolve differently. There is no second
// vocabulary here and there must not be one.
// Authentication is the route's, not this function's: register mounts it behind mw.cookie,
// which rejects an anonymous caller before any of this runs. There is no user id to read
// here — the import writes nothing that would belong to one.
func (h *linkedInHandlers) ImportLinkedInProfile(c *fiber.Ctx) error {
	// A body that does not parse and a body carrying no link are the same situation to the
	// user — there is nothing to follow — so they get the same answer, and neither costs an
	// outbound request.
	var req linkedInImportRequest
	if err := c.BodyParser(&req); err != nil {
		req.URL = ""
	}
	input := strings.TrimSpace(req.URL)
	if input == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Paste your LinkedIn profile link.")
	}

	profile, err := h.reader.Fetch(c.Context(), input)
	if err != nil {
		return linkedInImportError(err)
	}

	prof := resumeProfile(profile.Headline)

	// skills and categories are always arrays (possibly empty) so the client can treat them
	// uniformly; seniority is omitted when unresolved so a client never sees a guess. This
	// mirrors /me/resume/extract exactly, because the client merges both into one set.
	data := fiber.Map{"skills": prof.Skills, "categories": prof.Categories}
	if prof.Seniority != "" {
		data["seniority"] = prof.Seniority
	}
	if geo := derivedGeo(profile.Location); geo != nil {
		data["location"] = geo
	}
	putIfSet(data, "name", profile.Name)
	putIfSet(data, "headline", profile.Headline)
	putIfSet(data, "company", profile.Company)

	return c.JSON(fiber.Map{"data": data})
}

// derivedGeo resolves a stated locality through the geography dictionary, or returns nil
// when it resolves to nothing — an address the dictionary does not know yields no location
// rather than a guess, and an empty object would read to a client as "we looked and the
// answer is nowhere".
func derivedGeo(stated string) fiber.Map {
	if strings.TrimSpace(stated) == "" {
		return nil
	}
	geo := location.Parse(stated)
	if len(geo.Countries) == 0 && len(geo.Regions) == 0 && len(geo.Cities) == 0 {
		return nil
	}
	return fiber.Map{
		"countries": orEmpty(geo.Countries),
		"regions":   orEmpty(geo.Regions),
		"cities":    orEmpty(geo.Cities),
	}
}

// linkedInImportError renders the reader's three outcomes as three statuses. They are told
// apart because they are three different situations for the user: one they can fix by
// pasting a different link, one that is nothing to do with them, and one where the profile
// simply is not public enough to read.
//
// Anything else is returned unwrapped, so RenderError treats it as the unexpected error it
// is — 500, and reported. Folding an unrecognised error into the 502 would tell the user
// LinkedIn did not answer when in fact we had a bug, and would keep that bug off the error
// tracker for as long as it lived.
func linkedInImportError(err error) error {
	switch {
	case errors.Is(err, linkedinprofile.ErrNotAProfileURL):
		return fiber.NewError(fiber.StatusBadRequest,
			"That doesn't look like a LinkedIn profile link. It should look like linkedin.com/in/your-name.")
	case errors.Is(err, linkedinprofile.ErrNoProfile):
		return fiber.NewError(fiber.StatusUnprocessableEntity,
			"We couldn't read that profile. Upload your CV instead, or fill the next steps in yourself.")
	case errors.Is(err, linkedinprofile.ErrFetch):
		return fiber.NewError(fiber.StatusBadGateway,
			"LinkedIn didn't answer just now. Try again in a moment, or upload your CV instead.")
	default:
		return err
	}
}

// putIfSet omits an empty display field rather than sending "". The client renders these
// back to the user as "here is what we recognised", and an empty string there would read as
// "we recognised nothing" for a field LinkedIn simply withheld.
func putIfSet(m fiber.Map, key, value string) {
	if value != "" {
		m[key] = value
	}
}
