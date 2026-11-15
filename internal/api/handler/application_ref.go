package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// applicationRef is one row of the tracking listing, addressed the way the listing served it
// (internal/application/jobtracking/repository.go): an application whose posting was deleted is
// named by the application itself, every other row by its posting's public slug. The write
// endpoints accept both. Exactly one field is set.
type applicationRef struct {
	// AppID is applications.id when the row named an application directly.
	AppID int64
	// Slug is the posting's public slug otherwise.
	Slug string
}

// applicationRefPrefix marks the application form. See ListInteractions, which mints it.
const applicationRefPrefix = "a"

// parseApplicationRef reads a row id into the thing it names. An empty id is refused as
// not-found, not as a bad request: "not an id" and "not visible to you" must be one answer.
//
// Anything that is not the application form is handed on as a slug, including "a" followed by
// digits too large for an int64; that lookup answers 404 anyway.
func parseApplicationRef(raw string) (applicationRef, error) {
	if raw == "" {
		return applicationRef{}, fiber.NewError(fiber.StatusNotFound, "application not found")
	}
	if rest, ok := strings.CutPrefix(raw, applicationRefPrefix); ok && isDigits(rest) {
		if id, err := strconv.ParseInt(rest, 10, 64); err == nil {
			return applicationRef{AppID: id}, nil
		}
	}
	return applicationRef{Slug: raw}, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
