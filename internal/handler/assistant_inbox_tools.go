package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/inbox"
	"github.com/strelov1/freehire/internal/mailclassify"
)

// assistantInboxBodyMax caps a mail listing that carries bodies, far below the 50
// the HTTP surface allows an external harness.
//
// The two callers are bounded differently on purpose. A harness reads a page once
// and forgets it; a tool result is persisted in the transcript and replayed into
// the model's context on EVERY later turn of the session, so a page of fifty
// bodies is charged again to every question that follows. assistantResultCap is
// the backstop, not the answer — it truncates after the fact, and a truncated mail
// listing is one whose tail silently does not exist.
const assistantInboxBodyMax = 10

// assistantInboxPageMax caps a listing without bodies. Rows are small, but they
// are still replayed forever.
const assistantInboxPageMax = 40

// assistantInboxTools are the mail tools. They are registered for the general chat
// preset only: a tailoring session works one CV, an experience interview collects
// what the candidate has done, and a side panel talks about the page on screen.
//
// There is deliberately no tool here that opens ONE message by id. That endpoint
// marks the message read, and read_at means "a human saw this" — an agent sweeping
// the backlog through it would silently zero its owner's unread count. Bodies come
// from the listing, which marks nothing. And there is deliberately no tool that
// SENDS mail: message bodies are attacker-controlled text, and the surest answer to
// a prompt injection is that it has no outbound channel to reach.
func (h *assistantHandlers) assistantInboxTools() []assistant.Tool {
	return []assistant.Tool{
		h.inboxOverviewTool(),
		h.inboxSearchTool(),
		h.inboxTriageTool(),
		h.inboxResolveSuggestionTool(),
		h.inboxLinkTool(),
		h.inboxUnlinkTool(),
		h.inboxRecordApplicationTool(),
	}
}

// inboxOverviewTool reports the mailbox's shape and nothing of its content.
func (h *assistantHandlers) inboxOverviewTool() assistant.Tool {
	return assistant.Tool{
		Name: "inbox_overview",
		Description: "Count the user's application mail by label (interview_invitation, rejection, offer, …), " +
			"by link state, and by how much is unread or not yet judged. Call this FIRST when the question is " +
			"broad (\"what's happening with my applications?\") — it is one cheap call that tells you which " +
			"inbox_search to make, instead of reading messages until an answer appears. It returns no message " +
			"text at all.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}
			return svc.Overview(ctx, userID)
		},
	}
}

// assistantMailMessage is the model's view of one message: the fields it needs to
// decide and to act, and nothing it would pay to keep. Body is present only when
// the call asked for it.
type assistantMailMessage struct {
	ID           int64  `json:"id"`
	Source       string `json:"source"`
	From         string `json:"from"`
	Subject      string `json:"subject"`
	ReceivedAt   string `json:"received_at"`
	Read         bool   `json:"read"`
	Label        string `json:"label,omitempty"`
	LinkState    string `json:"link_state"`
	LinkedSlug   string `json:"linked_slug,omitempty"`
	LinkedTo     string `json:"linked_company,omitempty"`
	SuggestedFor string `json:"suggested_slug,omitempty"`
	Snippet      string `json:"snippet,omitempty"`
	Body         string `json:"body,omitempty"`
}

// mailMessageView projects a message for the model. The sender's address and
// display name are joined because both matter and neither is the employer: the
// display name is usually the ATS relay that sent it.
func mailMessageView(m inbox.Message, withBody bool) assistantMailMessage {
	from := m.FromName
	if from == "" {
		from = m.FromAddr
	} else if m.FromAddr != "" {
		from = fmt.Sprintf("%s <%s>", m.FromName, m.FromAddr)
	}
	out := assistantMailMessage{
		ID: m.ID, Source: m.Source, From: from, Subject: m.Subject,
		ReceivedAt: m.ReceivedAt.Format(time.RFC3339), Read: m.Read,
		Label: m.StatusSignal, LinkState: m.LinkState,
		LinkedSlug: m.LinkedSlug, LinkedTo: m.LinkedCompany, SuggestedFor: m.SuggestedSlug,
	}
	if withBody {
		out.Body = m.BodyText
	} else {
		out.Snippet = m.Snippet
	}
	return out
}

// inboxSearchTool lists mail under the labels the classification worker assigned.
func (h *assistantHandlers) inboxSearchTool() assistant.Tool {
	return assistant.Tool{
		Name: "inbox_search",
		Description: "List the user's application mail, newest first, filtered by the label a background worker " +
			"already assigned, by link state, or by a search term. Each row carries the sender, subject, date, " +
			"label and which application it is attached to — enough to answer most questions WITHOUT bodies. " +
			"Ask for bodies only when the question is about what a message actually says; those pages are capped " +
			"low because everything returned here stays in this conversation and is re-read on every later turn. " +
			"Reading here never marks anything read.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{
					"type":        "string",
					"enum":        mailclassify.SignalValues,
					"description": "One classification label. Omit for every label.",
				},
				"link_state": map[string]any{
					"type": "string",
					"enum": inbox.LinkStates,
					"description": "'suggested' is the queue of matcher proposals awaiting the user's decision; " +
						"'unlinked' is mail with no application to attach to; 'linked' is already resolved.",
				},
				"unjudged":     map[string]any{"type": "boolean", "description": "Only mail nothing has classified yet."},
				"unread":       map[string]any{"type": "boolean", "description": "Only mail the user has not opened."},
				"query":        map[string]any{"type": "string", "description": "Free text matched against subject, sender and body."},
				"source":       map[string]any{"type": "string", "enum": inbox.Sources, "description": "One mail account. Omit for all."},
				"include_body": map[string]any{"type": "boolean", "description": fmt.Sprintf("Return each message's readable body. Caps the page at %d.", assistantInboxBodyMax)},
				"limit":        map[string]any{"type": "integer", "description": fmt.Sprintf("How many messages (default %d, max %d, or %d with bodies).", assistantInboxDefaultLimit, assistantInboxPageMax, assistantInboxBodyMax)},
				"offset":       map[string]any{"type": "integer", "description": "Skip this many messages (paging)."},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Label       string `json:"label"`
				LinkState   string `json:"link_state"`
				Unjudged    bool   `json:"unjudged"`
				Unread      bool   `json:"unread"`
				Query       string `json:"query"`
				Source      string `json:"source"`
				IncludeBody bool   `json:"include_body"`
				Limit       int    `json:"limit"`
				Offset      int    `json:"offset"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}

			ceiling := assistantInboxPageMax
			if in.IncludeBody {
				ceiling = assistantInboxBodyMax
			}
			limit := in.Limit
			if limit <= 0 {
				limit = assistantInboxDefaultLimit
			}
			limit = min(limit, ceiling)

			page, err := svc.Search(ctx, userID, inbox.Query{
				Source: in.Source, Status: in.Label, Link: in.LinkState, Q: in.Query,
				Unread: in.Unread, Unclassified: in.Unjudged, WithBody: in.IncludeBody,
				Limit: limit, Offset: max(in.Offset, 0),
			})
			if err != nil {
				return nil, mailToolError(err)
			}
			msgs := make([]assistantMailMessage, 0, len(page.Messages))
			for _, m := range page.Messages {
				msgs = append(msgs, mailMessageView(m, in.IncludeBody))
			}
			return map[string]any{
				"total": page.Total, "limit": limit, "offset": max(in.Offset, 0), "messages": msgs,
			}, nil
		},
	}
}

// assistantInboxDefaultLimit is one screenful of mail.
const assistantInboxDefaultLimit = 20

// inboxTriageTool records the model's verdict for one message.
func (h *assistantHandlers) inboxTriageTool() assistant.Tool {
	return assistant.Tool{
		Name: "inbox_triage",
		Description: "Record what one message is, and optionally which application it belongs to. Omitting the " +
			"slug classifies without touching the link — it never clears one; clearing is inbox_unlink. When the " +
			"label implies progress on a linked application, its stage moves forward (never backward).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "integer", "description": "The message id from inbox_search."},
				"label":      map[string]any{"type": "string", "enum": mailclassify.SignalValues, "description": "What the message is."},
				"slug":       map[string]any{"type": "string", "description": "public_slug of the application this message is about. Omit to leave the link as it is."},
				"confidence": map[string]any{"type": "number", "description": "Your own confidence, 0..1. Stored for the user to see; it gates nothing."},
			},
			"required": []string{"id", "label"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID         int64    `json:"id"`
				Label      string   `json:"label"`
				Slug       string   `json:"slug"`
				Confidence *float32 `json:"confidence"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}
			msg, err := svc.Triage(ctx, userID, in.ID, inbox.Verdict{
				Signal: in.Label, Slug: in.Slug, Confidence: in.Confidence,
			})
			if err != nil {
				return nil, mailToolError(err)
			}
			return mailMessageView(msg, false), nil
		},
	}
}

// inboxResolveSuggestionTool answers a matcher proposal.
func (h *assistantHandlers) inboxResolveSuggestionTool() assistant.Tool {
	return assistant.Tool{
		Name: "inbox_resolve_suggestion",
		Description: "Answer a pending match suggestion on one message: confirm it into a real link, or reject it " +
			"and leave the message unlinked. Only an unambiguous automatic match links mail on its own, so " +
			"everything else waits here — find them with inbox_search link_state='suggested'.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":       map[string]any{"type": "integer", "description": "The message id."},
				"decision": map[string]any{"type": "string", "enum": []string{"confirm", "reject"}, "description": "confirm accepts the suggested application; reject dismisses it."},
			},
			"required": []string{"id", "decision"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID       int64  `json:"id"`
				Decision string `json:"decision"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Decision != "confirm" && in.Decision != "reject" {
				return nil, fmt.Errorf("unknown decision %q — valid values are: confirm, reject", in.Decision)
			}
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}
			msg, err := svc.ResolveSuggestion(ctx, userID, in.ID, in.Decision == "confirm")
			if err != nil {
				return nil, mailToolError(err)
			}
			return mailMessageView(msg, false), nil
		},
	}
}

// inboxLinkTool attaches a message to an application.
func (h *assistantHandlers) inboxLinkTool() assistant.Tool {
	return assistant.Tool{
		Name: "inbox_link",
		Description: "Attach one message to one of the user's tracked applications, without re-classifying it. " +
			"Use inbox_record_application instead when the application was never recorded.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "integer", "description": "The message id."},
				"slug": map[string]any{"type": "string", "description": "public_slug of the application."},
			},
			"required": []string{"id", "slug"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID   int64  `json:"id"`
				Slug string `json:"slug"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}
			msg, err := svc.Link(ctx, userID, in.ID, in.Slug)
			if err != nil {
				return nil, mailToolError(err)
			}
			return mailMessageView(msg, false), nil
		},
	}
}

// inboxUnlinkTool detaches a message from its application.
func (h *assistantHandlers) inboxUnlinkTool() assistant.Tool {
	return assistant.Tool{
		Name:        "inbox_unlink",
		Description: "Detach one message from the application it is linked to, leaving its label intact.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "The message id."}},
			"required":   []string{"id"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID int64 `json:"id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}
			msg, err := svc.Unlink(ctx, userID, in.ID)
			if err != nil {
				return nil, mailToolError(err)
			}
			return mailMessageView(msg, false), nil
		},
	}
}

// inboxRecordApplicationTool is the way out of the unlinked queue.
func (h *assistantHandlers) inboxRecordApplicationTool() assistant.Tool {
	return assistant.Tool{
		Name: "inbox_record_application",
		Description: "Record a tracked application from one message and link the message to it, in one call. This " +
			"is for mail about an application the user never recorded — plain linking needs an application to " +
			"point at, and there is none. The application is dated by the message, not by today. A message still " +
			"carrying a pending suggestion is refused: resolve that first.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "integer", "description": "The message id."},
				"slug": map[string]any{"type": "string", "description": "public_slug of the vacancy the message is about."},
			},
			"required": []string{"id", "slug"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID   int64  `json:"id"`
				Slug string `json:"slug"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			svc, err := h.mailService()
			if err != nil {
				return nil, err
			}
			msg, err := svc.RecordApplication(ctx, userID, in.ID, in.Slug)
			if err != nil {
				return nil, mailToolError(err)
			}
			return mailMessageView(msg, false), nil
		},
	}
}

// mailService reports the mail use cases, or an error the model can read when the
// surface was not wired up.
func (h *assistantHandlers) mailService() (*inbox.Service, error) {
	if h.mail == nil || h.mail.inbox == nil {
		return nil, errors.New("mail is not available")
	}
	return h.mail.inbox, nil
}

// mailToolError turns a service error into a message the model can act on within
// the same turn. The service already names an invalid value and its vocabulary;
// what is added here is the shape a model reads best — an unaddressable message is
// said to be missing, not merely "not found".
func mailToolError(err error) error {
	var invalid *inbox.InvalidError
	switch {
	case errors.As(err, &invalid):
		return invalid
	case errors.Is(err, inbox.ErrPendingSuggestion):
		return err
	case errors.Is(err, inbox.ErrSlugRequired):
		return err
	case inbox.IsNotFound(err):
		return errors.New("no such message, or the slug names no vacancy — take message ids from inbox_search " +
			"and slugs from search_jobs or my_jobs")
	default:
		return err
	}
}
