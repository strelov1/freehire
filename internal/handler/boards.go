package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// boardResponse is the public wire shape of a shared board: only its display fields. It
// deliberately omits every owner-identifying column (user id, email) so publishing a board
// exposes no account PII. author_label is empty when the board is anonymous.
type boardResponse struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	AuthorLabel string `json:"author_label"`
}

// toBoardResponse maps a public-board row to its wire shape.
func toBoardResponse(b db.GetPublicBoardBySlugRow) boardResponse {
	return boardResponse{
		Name:        b.Name,
		Query:       b.Query,
		AuthorLabel: b.AuthorLabel.String,
	}
}

// GetBoard serves a shared board by its public slug — unauthenticated, no owner-scoping.
// An unknown or unshared slug is a 404 (mapped from the saved-search not-found sentinel).
func (a *API) GetBoard(c *fiber.Ctx) error {
	board, err := a.savedSearch.GetPublicBoard(c.Context(), c.Params("slug"))
	if err != nil {
		return savedSearchError(err)
	}
	return c.JSON(fiber.Map{"data": toBoardResponse(board)})
}
