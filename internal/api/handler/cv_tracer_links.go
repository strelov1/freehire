package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
)

// SetCVTracerLinks records the candidate's decision to have the links in this CV's PDF traced,
// or to stop. Off is the state of every CV that has not been through here.
//
// Enabling is refused when the deployment has no visitor salt: without one there is no honest way
// to tell one visitor from another, and accepting the consent while quietly recording less would
// answer a question the candidate did not ask. Disabling is never refused — withdrawing consent is
// not a favour the deployment grants.
func (h *cvHandlers) SetCVTracerLinks(ctx context.Context, req *apigen.CvTracerLinksSetting, params apigen.SetCVTracerLinksParams) (*apigen.SetCVTracerLinksOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	enabled := req.Enabled.Value
	if enabled && h.tracerSalt == "" {
		return nil, fiber.NewError(fiber.StatusConflict,
			"link tracing is not configured on this deployment")
	}
	if err := h.cvStore.SetTracerLinks(ctx, id, userID, enabled); err != nil {
		return nil, mapCVError(err)
	}
	return &apigen.SetCVTracerLinksOK{Data: apigen.CvTracerLinksState{TracerLinksEnabled: enabled}}, nil
}

// ListCVTracerLinks reports what is known about each of an owned CV's traced links.
//
// Clicks the owner made themselves are excluded by the query — they are recorded so the history
// stays complete, not so they can be read back as somebody else's interest. Automated traffic is
// counted separately rather than dropped, so the UI's "include likely bots" switch needs no second
// request.
func (h *cvHandlers) ListCVTracerLinks(ctx context.Context, params apigen.ListCVTracerLinksParams) (*apigen.ListCVTracerLinksOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := cvID(params.ID)
	if err != nil {
		return nil, err
	}
	rows, err := h.queries.ListTracerLinkStats(ctx, db.ListTracerLinkStatsParams{CvID: id, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]apigen.CvTracerLink, 0, len(rows))
	for _, r := range rows {
		item := apigen.CvTracerLink{
			SourcePath:     r.SourcePath,
			DestinationURL: r.DestinationUrl,
			TracedURL:      h.tracerBaseURL + "/cv/" + r.Token,
			Clicks:         r.Clicks,
			BotClicks:      r.BotClicks,
		}
		// DistinctVisitors is omitted rather than reported as zero when the deployment has no
		// salt: every click then carries an empty hash, and counting those would say "one
		// visitor" about any number of people. Absent is the honest answer; zero a wrong one.
		if h.tracerSalt != "" {
			item.DistinctVisitors = apigen.NewOptInt64(r.DistinctVisitors)
		}
		if r.LastClickAt.Valid {
			item.LastClickAt = apigen.NewOptDateTime(r.LastClickAt.Time)
		}
		out = append(out, item)
	}
	return &apigen.ListCVTracerLinksOK{Data: out}, nil
}
