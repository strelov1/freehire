package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/apptimeline"
	"github.com/strelov1/freehire/internal/db"
)

// timelineHandlers serves the ledger's dated read — the caller's application events over
// a range, which the tracking calendar paints and an agent can ask for directly.
type timelineHandlers struct {
	timeline *apptimeline.Service
}

func newTimelineHandlers(queries *db.Queries) *timelineHandlers {
	return &timelineHandlers{timeline: apptimeline.New(queries)}
}

func (h *timelineHandlers) register(api fiber.Router, mw middleware) {
	// Its own namespace rather than /me/tracking/calendar, for the reason recorded above
	// /me/applications/:id: GET /me/tracking/:slug is already mounted — from gmail.go,
	// while its static siblings are registered in two other files — so a static segment
	// under that prefix resolves only on the order the Register* calls happen to run in.
	// Nothing enforces that order and the failure is quiet: the parameterised route would
	// answer for a job slug that does not exist.
	//
	// mw.key like the rest of /me: a cookie or a full-scope key, so a caller's own harness
	// can read its history.
	api.Get("/me/timeline", mw.key, h.Timeline)
}

// timelineEvent is one ledger event on the wire — a projection of apptimeline.Event, and
// separate from it so that a field added to the service type has to be published
// deliberately rather than by inheritance.
//
// The optional members are omitted rather than zeroed; apptimeline.Event says when each
// is legitimately absent.
type timelineEvent struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Signal string `json:"signal,omitempty"`
	Source string `json:"source"`
	// Observed is the server's verdict, not the reader's: appevent owns the rule that
	// says which sources carry a date somebody other than the candidate set.
	Observed bool `json:"observed"`
	// OccurredAt is an instant, not a day. Which day it falls on depends on the reader's
	// clock, so the grouping belongs in the browser that knows it.
	OccurredAt    time.Time `json:"occurred_at"`
	CompanySlug   string    `json:"company_slug"`
	RoleTitle     string    `json:"role_title,omitempty"`
	ApplicationID int64     `json:"application_id,omitempty"`
	JobSlug       string    `json:"job_slug,omitempty"`
	EmailID       int64     `json:"email_id,omitempty"`
	EmailSubject  string    `json:"email_subject,omitempty"`
}

// Timeline serves the caller's events between from and to inclusive, oldest first.
func (h *timelineHandlers) Timeline(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	from, err := timelineBound(c, "from")
	if err != nil {
		return err
	}
	to, err := timelineBound(c, "to")
	if err != nil {
		return err
	}

	events, err := h.timeline.Range(c.Context(), userID, from, to)
	if err != nil {
		if errors.Is(err, apptimeline.ErrInvalidRange) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}

	out := make([]timelineEvent, 0, len(events))
	for _, e := range events {
		out = append(out, timelineEvent{
			ID:            e.ID,
			Kind:          e.Kind,
			Signal:        e.Signal,
			Source:        e.Source,
			Observed:      e.Observed,
			OccurredAt:    e.OccurredAt,
			CompanySlug:   e.CompanySlug,
			RoleTitle:     e.RoleTitle,
			ApplicationID: e.ApplicationID,
			JobSlug:       e.JobSlug,
			EmailID:       e.EmailID,
			EmailSubject:  e.EmailSubject,
		})
	}
	return c.JSON(fiber.Map{
		"data": out,
		"meta": fiber.Map{"from": from, "to": to, "count": len(out)},
	})
}

// timelineBound parses one RFC3339 bound. Both are required and neither is defaulted: a
// missing bound silently widened to "the last month" would answer a question the caller
// did not ask, and the reader's month is not the server's to guess.
func timelineBound(c *fiber.Ctx, name string) (time.Time, error) {
	raw := c.Query(name)
	if raw == "" {
		return time.Time{}, fiber.NewError(fiber.StatusBadRequest, "both from and to are required, as RFC3339 instants")
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fiber.NewError(fiber.StatusBadRequest, name+" is not an RFC3339 instant")
	}
	return at, nil
}
