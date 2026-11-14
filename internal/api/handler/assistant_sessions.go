package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// sessionResponse is the wire shape of one conversation.
type sessionResponse struct {
	SearchContext *assistant.SearchContext `json:"search_context,omitempty"`
	ID            string                   `json:"id"`
	Preset        string                   `json:"preset"`
	Label         string                   `json:"label"`
	CVID          *string                  `json:"cv_id,omitempty"`
	JobID         *int64                   `json:"job_id,omitempty"`
}

// sessionView renders a session for the client. The id is a string because the
// client treats session ids as opaque and a CV stores its bound session id as text.
func sessionView(s assistant.Session) sessionResponse {
	return sessionResponse{
		ID:            s.ID.String(),
		SearchContext: s.SearchContext,
		Preset:        s.Preset,
		Label:         s.Label,
		CVID:          cvIDString(s.CVID),
		JobID:         s.JobID,
	}
}

// assistantSessionID parses the :id route param. A session id is a UUID, so
// anything else cannot name a session at all — report it as missing rather than as
// a bad request, keeping "not yours" and "not a session" one indistinguishable
// answer.
// cvIDString renders a tailoring session's CV binding for the wire. A CV id is a
// UUID the client treats as opaque, so it travels as a string like the session's
// own id.
func cvIDString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// parseAssistantSessionID is assistantSessionID for a raw id.
func parseAssistantSessionID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return id, nil
}

// CreateAssistantSession starts a new unbound conversation for the caller — a general
// chat, the experience interviewer with `?preset=profile`, or a browsing session held
// from the extension's side panel with `?preset=browse`.
//
// Only the presets that bind to NOTHING can be minted here. A tailoring session is
// created by the tailoring bootstrap, which knows the CV and the vacancy to bind; letting
// this endpoint name that preset would produce a session whose tools point at nothing.
func (h *assistantHandlers) CreateAssistantSession(ctx context.Context, req apigen.OptAssistantSessionRequest, params apigen.CreateAssistantSessionParams) (apigen.CreateAssistantSessionRes, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	preset, err := creatablePreset(params.Preset.Value)
	if err != nil {
		return nil, err
	}
	input := req.Value
	if input.SearchQuery.Set && !input.SearchQuery.Null {
		if preset != assistant.PresetChat {
			return nil, fiber.NewError(fiber.StatusBadRequest, "only general chat can bind a search")
		}
		return h.createShortlistSession(ctx, userID, input.SearchQuery.Value, input.RequestID.Value)
	}
	if input.RequestID.Value != "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "request_id requires search_query")
	}

	// The creatable presets that bind to something all bind to the same thing: an
	// application. The client may name the vacancy because that binding is one the caller
	// already owns, and the server checks it rather than taking their word for it.
	var jobID *int64
	var vacancy db.Job
	if bindsToApplication(preset) {
		job, err := h.applicationVacancy(ctx, userID, params.Job.Value)
		if err != nil {
			return nil, err
		}
		vacancy, jobID = job, &job.ID
	}
	// An application holds ONE apply conversation: its numbered questions and the answers
	// given to them must stay in one transcript, so opening it again resumes it.
	if preset == assistant.PresetApply {
		existing, ok, err := h.store.SessionForJob(ctx, userID, preset, *jobID)
		if err != nil {
			return nil, err
		}
		if ok {
			return assistantSessionOK(existing)
		}
	}
	sess, err := h.store.CreateSession(ctx, userID, preset, nil, jobID)
	if name, dup := pgerr.UniqueViolationConstraint(err); dup && name == "assistant_sessions_one_apply_per_job" {
		// Another open of the same application won the race to create it; that one is the
		// conversation, so return it rather than failing the second click.
		existing, ok, err := h.store.SessionForJob(ctx, userID, preset, *jobID)
		if err != nil {
			return nil, err
		}
		if ok {
			return assistantSessionOK(existing)
		}
	}
	if err != nil {
		return nil, err
	}
	// Name it after its vacancy now, while we hold it. A session is otherwise named from
	// its first user message — which for these presets is the server's own brief,
	// identical every time, so every such session in the rail would carry the same string
	// and none would say which interview it was.
	if label := applicationSessionLabel(preset, vacancy); label != "" {
		if err := h.store.LabelSession(ctx, sess.ID, sess.UserID, label); err != nil {
			log.Printf("assistant: could not name %s session %s: %v", preset, sess.ID, err)
		} else {
			sess.Label = label
		}
	}
	view, err := assistantSessionView(sess)
	if err != nil {
		return nil, err
	}
	return &apigen.CreateAssistantSessionCreated{Data: view}, nil
}

// assistantSessionView is sessionView for the contract's assistant operations.
func assistantSessionView(s assistant.Session) (apigen.AssistantSession, error) {
	v := sessionView(s)
	out := apigen.AssistantSession{ID: v.ID, Preset: v.Preset, Label: v.Label}
	optNil(v.CVID, out.CvID.SetTo)
	optNil(v.JobID, out.JobID.SetTo)
	if v.SearchContext != nil {
		raw, err := json.Marshal(v.SearchContext)
		if err != nil {
			return apigen.AssistantSession{}, err
		}
		out.SearchContext = raw
	}
	return out, nil
}

// assistantSessionOK answers 200 with a session that already existed.
func assistantSessionOK(s assistant.Session) (apigen.CreateAssistantSessionRes, error) {
	view, err := assistantSessionView(s)
	if err != nil {
		return nil, err
	}
	return &apigen.CreateAssistantSessionOK{Data: view}, nil
}

// creatablePreset resolves the preset a client asked for. Naming the value it rejected
// and the ones it accepts matters: this endpoint serves the web app and the extension
// both, and a bare 400 leaves a client author guessing.
//
// Tailoring is the one preset that cannot be minted here, because its binding is a CV
// that does not exist yet — the tailoring bootstrap creates both together. A rehearsal
// and a debrief bind to an application the caller already has, so naming one is safe.
//
// The accepted list is a slice rather than a switch's case list because the refusal has
// to recite it, and a preset added to one and forgotten in the other is a 400 that lies
// about what the client may ask for.
var creatablePresets = []string{
	assistant.PresetChat,
	assistant.PresetProfile,
	assistant.PresetBrowse,
	assistant.PresetInterview,
	assistant.PresetDebrief,
	assistant.PresetApply,
}

func creatablePreset(asked string) (string, error) {
	if asked == "" {
		return assistant.PresetChat, nil
	}
	if slices.Contains(creatablePresets, asked) {
		return asked, nil
	}
	return "", fiber.NewError(fiber.StatusBadRequest,
		fmt.Sprintf("preset %q cannot be created here; use one of %q — a tailoring session is created from its CV",
			asked, creatablePresets))
}

// bindsToApplication answers whether a preset is held against one application. Both
// such presets carry a vacancy and no CV, and both are minted by naming a slug the
// caller has applied to.
//
// The stage is deliberately not part of the answer. A debrief belongs after an
// interview, but a candidate who sat one and never moved their application's stage is
// exactly who it is for; where the button appears is the client's judgement, and
// refusing them here would be a bug wearing a rule's clothes.
func bindsToApplication(preset string) bool {
	return preset == assistant.PresetInterview || preset == assistant.PresetDebrief || preset == assistant.PresetApply
}

// applicationSessionLabel names such a session in the rail: what it is, the role, and
// the company when the posting carries one. Empty for a vacancy with no title, which
// leaves the ordinary naming-from-the-first-message in place rather than writing a label
// that says nothing — and empty for any preset that is not bound to an application.
func applicationSessionLabel(preset string, job db.Job) string {
	var prefix string
	switch preset {
	case assistant.PresetInterview:
		prefix = "Interview: "
	case assistant.PresetDebrief:
		prefix = "Debrief: "
	case assistant.PresetApply:
		prefix = "Application: "
	default:
		return ""
	}
	title := strings.TrimSpace(job.Title)
	if title == "" {
		return ""
	}
	label := prefix + title
	if company := strings.TrimSpace(job.Company); company != "" {
		label += " · " + company
	}
	return label
}

// applicationVacancy resolves the `job_id` such a session is asked for, and refuses one
// the caller has no application against.
//
// The application row IS the authorisation: user_jobs holds one per (user, vacancy), so
// its absence answers "not yours" and "no such thing" with the same 404 — the same way a
// session the caller does not own is reported as missing.
func (h *assistantHandlers) applicationVacancy(ctx context.Context, userID int64, rawSlug string) (db.Job, error) {
	slug := strings.TrimSpace(rawSlug)
	if slug == "" {
		return db.Job{}, fiber.NewError(fiber.StatusBadRequest,
			"this conversation needs the `job` slug of an application you hold")
	}
	if h.stages == nil {
		return db.Job{}, fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// The vacancy is named by its public slug, like everywhere else on this API — the
	// numeric id is ours and never leaves the backend.
	job, err := h.stages.GetJobBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Job{}, fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		return db.Job{}, err
	}
	if _, err := h.stages.GetUserJobStage(ctx,
		db.GetUserJobStageParams{UserID: userID, JobID: job.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Job{}, fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		return db.Job{}, err
	}
	return job, nil
}

// ListAssistantSessions returns the caller's chat conversations, newest activity
// first. Tailoring conversations are not chats — each belongs to a CV — so they
// never appear here.
func (h *assistantHandlers) ListAssistantSessions(ctx context.Context) (*apigen.AssistantSessionList, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := h.store.ChatSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.AssistantSession, 0, len(sessions))
	for _, s := range sessions {
		view, err := assistantSessionView(s)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return &apigen.AssistantSessionList{Data: out, Meta: apigen.AssistantSessionListMeta{Total: int64(len(out))}}, nil
}

// GetAssistantSession returns one owned conversation with its full transcript, so
// the client can repaint it through the same reducer live events fold through.
func (h *assistantHandlers) GetAssistantSession(ctx context.Context, params apigen.GetAssistantSessionParams) (*apigen.GetAssistantSessionOK, error) {
	sess, err := h.ownedSessionByID(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	messages, err := h.store.Transcript(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	// The transcript is stored JSON the model also reads: pass it through as written.
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	view, err := assistantSessionView(sess)
	if err != nil {
		return nil, err
	}
	return &apigen.GetAssistantSessionOK{Data: apigen.AssistantSessionDetail{Session: view, Messages: raw}}, nil
}

// DeleteAssistantSession removes an owned conversation and its transcript.
func (h *assistantHandlers) DeleteAssistantSession(ctx context.Context, params apigen.DeleteAssistantSessionParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := parseAssistantSessionID(params.ID)
	if err != nil {
		return err
	}
	sess, err := h.store.Session(ctx, id, userID)
	if err != nil {
		return mapAssistantError(err)
	}
	// A support session is never deleted by its owner: its handover stamp is what bounds
	// how many times an account may email the team.
	if sess.Preset == assistant.PresetSupport {
		return errSessionNotFound
	}
	return mapAssistantError(h.store.DeleteSession(ctx, id, userID))
}

// CancelAssistantTurn stops the session's running turn.
//
// It exists because stopping used to be a side effect of the transport: the client aborted its
// read, the next write failed, and the turn was cancelled. That made a deliberate stop
// indistinguishable from a phone freezing its tab, and the turn paid for the confusion. Now the
// two are different things, and this is the one that means stop.
//
// A session with nothing running is not an error. A client cancels a turn whose end it cannot
// see; requiring it to first prove the turn is alive would ask it for the one fact it does not
// have. The ownership check comes first either way, so an id cannot be probed for existence.
func (h *assistantHandlers) CancelAssistantTurn(ctx context.Context, params apigen.CancelAssistantTurnParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := parseAssistantSessionID(params.ID)
	if err != nil {
		return err
	}
	sess, err := h.store.Session(ctx, id, userID)
	if err != nil {
		return mapAssistantError(err)
	}
	if sess.Preset == assistant.PresetSupport {
		return errSessionNotFound
	}
	h.turns.cancel(id)
	return nil
}
