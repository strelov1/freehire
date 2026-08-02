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
	// Its own namespace rather than a segment under /me/tracking. GET /me/tracking/:slug
	// is mounted from gmail.go and only avoids shadowing its static siblings because the
	// registration order is deliberate — both sides say so (see the comment there and the
	// one above inboxH.register). That arrangement is maintained by hand and by nothing
	// else, and its failure is quiet: the parameterised route would answer for a job slug
	// that does not exist. Every static segment added under that prefix is another thing
	// the arrangement has to keep carrying, so this one is not added to it.
	//
	// mw.key like the rest of /me: a cookie or a full-scope key, so a caller's own harness
	// can read its history.
	api.Get("/me/timeline", mw.key, h.Timeline)
	// The calendar's second layer, on its own path rather than folded into the timeline.
	// /me/timeline is published with `data` as a list of events and is read by the CLI
	// and MCP surfaces; a second list inside it would either break that shape or hide
	// the meetings in `meta`. The day panel's no-fetch rule does not require ONE request
	// — it requires the data to be in hand before a day is clicked, and the page asks
	// for both ranges together.
	api.Get("/me/interviews", mw.key, h.Interviews)
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

// scheduledInterview is one arranged meeting on the wire.
//
// Separate from timelineEvent because it is a different kind of fact: an event happened
// and cannot change, a meeting is arranged and can move or be called off. Rendering them
// with one shape would invite the view to draw them with one meaning.
type scheduledInterview struct {
	ID            int64     `json:"id"`
	ApplicationID int64     `json:"application_id"`
	StartsAt      time.Time `json:"starts_at"`
	// A pointer, not a time.Time with omitempty: encoding/json has no notion of an empty
	// struct, so the tag is inert there and a meeting with no end serialises as year 1.
	// An all-day entry legitimately has no end, and a reader that trusted the field would
	// draw it two millennia long.
	EndsAt  *time.Time `json:"ends_at,omitempty"`
	Title   string     `json:"title,omitempty"`
	JoinURL string     `json:"join_url,omitempty"`
	// Status is suggested | confirmed | cancelled — the matcher's certainty, and then
	// the organiser's. A cancelled meeting is served, not withheld.
	Status      string `json:"status"`
	CompanySlug string `json:"company_slug"`
	RoleTitle   string `json:"role_title,omitempty"`
	JobSlug     string `json:"job_slug,omitempty"`
}

// Interviews serves the caller's arranged meetings whose start falls in the range.
func (h *timelineHandlers) Interviews(c *fiber.Ctx) error {
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

	found, err := h.timeline.Interviews(c.Context(), userID, from, to)
	if err != nil {
		if errors.Is(err, apptimeline.ErrInvalidRange) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}

	out := make([]scheduledInterview, 0, len(found))
	for _, iv := range found {
		out = append(out, scheduledInterview{
			ID:            iv.ID,
			ApplicationID: iv.ApplicationID,
			StartsAt:      iv.StartsAt,
			EndsAt:        optionalTime(iv.EndsAt),
			Title:         iv.Title,
			JoinURL:       iv.JoinURL,
			Status:        iv.Status,
			CompanySlug:   iv.CompanySlug,
			RoleTitle:     iv.RoleTitle,
			JobSlug:       iv.JobSlug,
		})
	}
	return c.JSON(fiber.Map{
		"data": out,
		"meta": fiber.Map{"from": from, "to": to, "count": len(out)},
	})
}

// optionalTime renders a zero time as absent rather than as the year 1.
func optionalTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
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
