package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/apptimeline"
	"github.com/strelov1/freehire/internal/platform/db"
)

// timelineHandlers serves the ledger's dated read: the caller's application events over a
// range, which the tracking calendar paints and an agent can ask for directly.
type timelineHandlers struct {
	timeline *apptimeline.Service
}

func newTimelineHandlers(queries *db.Queries) *timelineHandlers {
	return &timelineHandlers{timeline: apptimeline.New(queries)}
}

func (h *timelineHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Its own namespace, not a segment under /me/tracking: GET /me/tracking/:slug (gmail.go)
	// avoids shadowing its static siblings only through a hand-kept registration order, and
	// its failure is quiet (the parameterised route would answer for a nonexistent job slug).
	//
	// mw.key like the rest of /me: a cookie or a full-scope key, so a caller's own harness
	// can read its history.
	api.Get("/me/timeline", mw.key, contract)
	// The calendar's second layer, on its own path: /me/timeline's `data` is a list of events
	// read by the CLI and MCP, so a second list there would break that shape or hide meetings
	// in `meta`. The page asks for both ranges together, so the data is in hand before a day
	// is clicked.
	api.Get("/me/interviews", mw.key, contract)
}

// timelineWire projects the service's events onto the wire, separate from apptimeline.Event
// so a field added to the service type must be published deliberately. Shared by the range
// read and a single application's history so the two cannot disagree on fields.
//
// Optional members are omitted rather than zeroed (apptimeline.Event says when each is
// legitimately absent). OccurredAt is an instant, not a day: grouping by day belongs in the
// browser, which knows the reader's clock.
func timelineWire(events []apptimeline.Event) []apigen.TimelineEvent {
	out := make([]apigen.TimelineEvent, 0, len(events))
	for _, e := range events {
		out = append(out, apigen.TimelineEvent{
			ID:            e.ID,
			Kind:          e.Kind,
			Signal:        optString(e.Signal),
			Source:        e.Source,
			Observed:      e.Observed,
			OccurredAt:    e.OccurredAt,
			CompanySlug:   e.CompanySlug,
			RoleTitle:     optString(e.RoleTitle),
			ApplicationID: optInt64(e.ApplicationID),
			JobSlug:       optString(e.JobSlug),
			EmailID:       optInt64(e.EmailID),
			EmailSubject:  optString(e.EmailSubject),
		})
	}
	return out
}

// GetTimeline serves the caller's events between from and to inclusive, oldest first.
func (h *timelineHandlers) GetTimeline(ctx context.Context, params apigen.GetTimelineParams) (*apigen.GetTimelineOK, error) {
	userID, from, to, err := timelineRequest(ctx, params.From, params.To)
	if err != nil {
		return nil, err
	}

	events, err := h.timeline.Range(ctx, userID, from, to)
	if err != nil {
		if errors.Is(err, apptimeline.ErrInvalidRange) {
			return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return nil, err
	}

	out := timelineWire(events)
	return &apigen.GetTimelineOK{Data: out, Meta: apigen.TimelineMeta{From: from, To: to, Count: int64(len(out))}}, nil
}

// GetInterviews serves the caller's arranged meetings whose start falls in the range. A
// meeting is arranged and can move or be called off, unlike an event that happened, so it
// keeps a separate shape.
func (h *timelineHandlers) GetInterviews(ctx context.Context, params apigen.GetInterviewsParams) (*apigen.GetInterviewsOK, error) {
	userID, from, to, err := timelineRequest(ctx, params.From, params.To)
	if err != nil {
		return nil, err
	}

	found, err := h.timeline.Interviews(ctx, userID, from, to)
	if err != nil {
		if errors.Is(err, apptimeline.ErrInvalidRange) {
			return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return nil, err
	}

	out := scheduledInterviewsWire(found)
	return &apigen.GetInterviewsOK{Data: out, Meta: apigen.TimelineMeta{From: from, To: to, Count: int64(len(out))}}, nil
}

func scheduledInterviewsWire(found []apptimeline.Interview) []apigen.ScheduledInterview {
	out := make([]apigen.ScheduledInterview, 0, len(found))
	for _, iv := range found {
		out = append(out, apigen.ScheduledInterview{
			ID:            iv.ID,
			ApplicationID: iv.ApplicationID,
			StartsAt:      iv.StartsAt,
			// An all-day entry has no end; a zero time is left out, not sent as the year 1.
			EndsAt:      optTime(iv.EndsAt),
			Title:       optString(iv.Title),
			JoinURL:     optString(iv.JoinURL),
			Status:      iv.Status,
			CompanySlug: iv.CompanySlug,
			RoleTitle:   optString(iv.RoleTitle),
			JobSlug:     optString(iv.JobSlug),
		})
	}
	return out
}

func optInt64(n int64) apigen.OptInt64 {
	if n == 0 {
		return apigen.OptInt64{}
	}
	return apigen.NewOptInt64(n)
}

// optTime renders a zero time as absent rather than as the year 1.
func optTime(at time.Time) apigen.OptDateTime {
	if at.IsZero() {
		return apigen.OptDateTime{}
	}
	return apigen.NewOptDateTime(at)
}

// timelineRequest reads the caller and the range both calendar layers are asked for. Shared
// so a bound cannot be parsed differently by each layer on the same grid.
func timelineRequest(ctx context.Context, rawFrom, rawTo apigen.OptString) (userID int64, from, to time.Time, err error) {
	if userID, err = requireUserIDFromContext(ctx); err != nil {
		return 0, time.Time{}, time.Time{}, err
	}
	if from, err = timelineBound("from", rawFrom.Value); err != nil {
		return 0, time.Time{}, time.Time{}, err
	}
	if to, err = timelineBound("to", rawTo.Value); err != nil {
		return 0, time.Time{}, time.Time{}, err
	}
	return userID, from, to, nil
}

// timelineBound parses one RFC3339 bound. Both are required and neither is defaulted: a
// silently widened range would answer a question the caller did not ask.
func timelineBound(name, raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fiber.NewError(fiber.StatusBadRequest, "both from and to are required, as RFC3339 instants")
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fiber.NewError(fiber.StatusBadRequest, name+" is not an RFC3339 instant")
	}
	return at, nil
}
