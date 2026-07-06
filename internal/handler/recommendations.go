package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// Recommendations returns open jobs ranked by semantic similarity to the caller's
// persisted CV embedding — the /my/recommendations feed. Behind RequireAuthOrKey;
// response is the standard list envelope of job views. It degrades to a successful
// EMPTY list (never an error) when the caller has no usable CV vector: none stored, or
// one produced by a superseded embedder (stale, so not comparable to the current jobs),
// or the search backend / semantic index is unavailable. A profile edit needing a
// re-embed happens on the user's next CV upload.
func (a *API) Recommendations(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, offset := pageParams(c)
	if offset+limit > maxSearchWindow {
		return fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}

	empty := func() error { return listResponse(c, []jobview.Job{}, 0, limit, offset) }
	if a.search == nil {
		return empty()
	}

	vec, model, err := a.resume.Embedding(c.Context(), userID)
	if err != nil {
		return err
	}
	if len(vec) == 0 || model != search.CurrentEmbedderModel() {
		return empty()
	}

	res, err := a.search.RecommendByVector(c.Context(), vec, limit, offset)
	if err != nil {
		return err
	}
	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}
	return listResponse(c, views, res.Total, limit, offset)
}
