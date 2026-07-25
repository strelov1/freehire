package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// FindJob resolves an external job posting to a freehire catalog slug by exact,
// case-insensitive company + title. It lets the browser extension recognise that
// the page it's on is a job already in the catalog and switch from the text
// match to the rich curated card. Public; returns {"data": null} on no match.
//
// NOTE: raw pgx query for prototype speed — promote to a sqlc query before merge.
func (a *API) FindJob(c *fiber.Ctx) error {
	company := strings.TrimSpace(c.Query("company"))
	title := strings.TrimSpace(c.Query("title"))
	if company == "" || title == "" {
		return c.JSON(fiber.Map{"data": nil})
	}
	// The catalog title must appear within the page title (real page titles carry
	// noise like "Remote … Job at Stripe"), scoped to an exact company match.
	// Prefer the longest (most specific) catalog title.
	var slug string
	err := a.pool.QueryRow(c.Context(),
		`SELECT public_slug FROM jobs
		 WHERE company ILIKE $1 AND $2 ILIKE '%' || title || '%'
		   AND public_slug IS NOT NULL AND length(title) >= 6
		 ORDER BY length(title) DESC, posted_at DESC NULLS LAST
		 LIMIT 1`, company, title).Scan(&slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(fiber.Map{"data": nil})
		}
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"public_slug": slug}})
}
