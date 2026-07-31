package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// applicationRef is one row of the tracking listing, addressed the way the listing
// served it. The listing mints two forms (internal/jobtracking/repository.go): an
// application whose posting cmd/prune removed is named by the application itself,
// because it has no slug to borrow; every other row is named by its posting's public
// slug. The write endpoints accept both, because the interface can only send back
// what the listing gave it.
//
// Exactly one field is set.
type applicationRef struct {
	// AppID is applications.id when the row named an application directly.
	AppID int64
	// Slug is the posting's public slug otherwise.
	Slug string
}

// applicationRefPrefix marks the application form. See ListInteractions, which mints it.
const applicationRefPrefix = "a"

// parseApplicationRef reads a row id into the thing it names. An empty id names
// nothing and is refused as not-found rather than as a bad request: "not an id" and
// "not visible to you" must be one answer, the rule the opaque-id swap set for CVs.
//
// Everything that is not the application form is handed on as a slug, including "a"
// followed by digits too large for an int64. The lookup there answers 404, which is
// the answer that branch would give anyway — no second error path for the same result.
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
