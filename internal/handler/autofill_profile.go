package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/cv"
)

// autofillProfile is the canonical set of fields the browser extension writes
// into application forms — assembled server-side so the extension stays dumb.
type autofillProfile struct {
	FullName  string `json:"full_name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Location  string `json:"location"`
	LinkedIn  string `json:"linkedin"`
	GitHub    string `json:"github"`
	Portfolio string `json:"portfolio"`
}

// buildAutofillProfile projects a CV contact header (plus the account email as a
// fallback) into the canonical autofill fields, splitting the name and sorting
// links into linkedin / github / portfolio.
func buildAutofillProfile(h cv.Header, accountEmail string) autofillProfile {
	first, last := splitName(h.FullName)
	email := h.Email
	if email == "" {
		email = accountEmail
	}
	p := autofillProfile{
		FullName:  h.FullName,
		FirstName: first,
		LastName:  last,
		Email:     email,
		Phone:     h.Phone,
		Location:  h.Location,
	}
	for _, link := range h.Links {
		switch l := strings.ToLower(link); {
		case strings.Contains(l, "linkedin.com") && p.LinkedIn == "":
			p.LinkedIn = link
		case strings.Contains(l, "github.com") && p.GitHub == "":
			p.GitHub = link
		case p.Portfolio == "":
			p.Portfolio = link
		}
	}
	return p
}

func splitName(full string) (first, last string) {
	parts := strings.Fields(full)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

// autofillProfile assembles a user's canonical autofill fields from their most
// recent CV's contact header plus their account email. Shared by the endpoint the
// extension reads and the agent that fills forms with it.
func (a *API) autofillProfile(ctx context.Context, userID int64) (autofillProfile, error) {
	var accountEmail string
	_ = a.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&accountEmail)

	var header cv.Header
	var raw []byte
	err := a.pool.QueryRow(ctx,
		`SELECT data FROM cvs WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 1`, userID).Scan(&raw)
	if err == nil {
		var doc cv.Document
		if json.Unmarshal(raw, &doc) == nil {
			header = doc.Header
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return autofillProfile{}, err
	}

	return buildAutofillProfile(header, accountEmail), nil
}

// AutofillProfile returns the caller's canonical autofill fields. keyAuth so the
// browser extension (Bearer) can read it. All-empty (bar email) with no CV.
func (a *API) AutofillProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	profile, err := a.autofillProfile(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": profile})
}
