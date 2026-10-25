package handler

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The auto-apply and screening-answer tools of the signed-in MCP server
// (openspec/changes/auto-apply-over-mcp): an agent connected for a person can start an
// application, follow it, answer the question it stopped on, and approve it — the whole
// background flow, with no browser in the loop.
//
// Every tool calls the same method the corresponding HTTP route does, so the two doors cannot
// disagree about what is allowed. The tools that ACT on an attempt (start, review, retry,
// cancel) additionally require the account's agents_may_apply switch, which only a browser
// session can turn on; reading never does.

// autoApplyMCPTools is the set the account server adds to the in-app assistant's discovery
// and tracking tools. Not offered to the in-app assistant: the browser there already has the
// job page's button and the drawer's review.
func (h *assistantHandlers) autoApplyMCPTools() []assistant.Tool {
	return []assistant.Tool{
		h.autoApplyStartTool(),
		h.autoApplyListTool(),
		h.autoApplyGetTool(),
		h.autoApplyReviewTool(),
		h.autoApplyRetryTool(),
		h.autoApplyCancelTool(),
		h.answerBankListTool(),
		h.answerBankSaveTool(),
		h.answerBankDeleteTool(),
		h.screeningAnswersGetTool(),
		h.screeningAnswersSetTool(),
	}
}

// agentsMayApplyOff is what an acting tool answers while the switch is off. It names where the
// person turns it on, because the agent can only relay that — it cannot flip it.
const agentsMayApplyOff = "auto-apply by connected agents is turned off for this account. " +
	"Ask the person to turn on \"Let connected agents apply for me\" on freehire, under " +
	"Account → API keys (/my/api-keys), then try again"

// requireAgentsMayApply refuses unless the account switched agents_may_apply on.
func (h *assistantHandlers) requireAgentsMayApply(ctx context.Context, userID int64) error {
	if agentsMayApplyWaived(ctx) {
		return nil
	}
	allowed, err := h.queries.GetAgentsMayApply(ctx, userID)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.New(agentsMayApplyOff)
	}
	return nil
}

// toolError turns what the shared methods return into a sentence an agent can act on. A
// fiber.Error already is one; pgx.ErrNoRows from a slug lookup is the one bare error they
// return that means something to the caller.
func toolError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("no job with this slug")
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return errors.New(fe.Message)
	}
	return err
}

var jobSlugProperty = map[string]any{"type": "string", "description": "The vacancy's slug, as search_jobs and my_jobs return it."}

var jobSlugSchema = map[string]any{
	"type":                 "object",
	"properties":           map[string]any{"job_slug": jobSlugProperty},
	"required":             []string{"job_slug"},
	"additionalProperties": false,
}

type jobSlugArgs struct {
	JobSlug string `json:"job_slug"`
}

// slugTool builds a tool whose only argument is a job slug. gated decides whether it needs
// the account switch.
func (h *assistantHandlers) slugTool(name, title, description string, effect assistant.Effect, gated bool,
	run func(ctx context.Context, userID int64, slug string) (any, error)) assistant.Tool {
	return assistant.Tool{
		Name: name, Title: title, Description: description, Effect: effect, Schema: jobSlugSchema,
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in jobSlugArgs
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.JobSlug == "" {
				return nil, errors.New("job_slug is required")
			}
			if gated {
				if err := h.requireAgentsMayApply(ctx, userID); err != nil {
					return nil, err
				}
			}
			out, err := run(ctx, userID, in.JobSlug)
			if err != nil {
				return nil, toolError(err)
			}
			return out, nil
		},
	}
}

func (h *assistantHandlers) autoApplyStartTool() assistant.Tool {
	return h.slugTool("auto_apply_start", "Start an automatic application",
		"Start applying to a vacancy automatically on the person's behalf. freehire tailors their CV "+
			"to the vacancy, checks the employer's application form against their saved answers, and "+
			"waits for a review (auto_apply_review) before submitting. Spends one of today's auto-apply "+
			"allowance. Only vacancies on supported applicant-tracking systems can be started. Follow "+
			"progress with auto_apply_get.",
		assistant.EffectAdds, true,
		func(ctx context.Context, userID int64, slug string) (any, error) {
			if err := h.startAutoApply(ctx, userID, slug); err != nil {
				return nil, err
			}
			return h.autoApplyView(ctx, userID, slug)
		})
}

func (h *assistantHandlers) autoApplyGetTool() assistant.Tool {
	return h.slugTool("auto_apply_get", "Check an automatic application",
		"Read where one automatic application stands: its status, the questions it stopped on, the "+
			"answers still missing before submission, its full history, and what to do next. Call it "+
			"again to follow progress — history only grows.",
		assistant.EffectReads, false, h.autoApplyView)
}

func (h *assistantHandlers) autoApplyRetryTool() assistant.Tool {
	return h.slugTool("auto_apply_retry", "Retry an automatic application",
		"Restart an automatic application that stopped — on an unanswered question, after repeated "+
			"errors, or because the CV could not be tailored — or refresh one awaiting review so newly "+
			"saved answers are reflected. Answer the open questions with answer_bank_save or "+
			"screening_answers_set first. Refused for a declined attempt and for one that may already "+
			"have reached the employer.",
		assistant.EffectOverwrites, true,
		func(ctx context.Context, userID int64, slug string) (any, error) {
			if err := h.retryAutoApply(ctx, userID, slug); err != nil {
				return nil, err
			}
			return h.autoApplyView(ctx, userID, slug)
		})
}

func (h *assistantHandlers) autoApplyCancelTool() assistant.Tool {
	return h.slugTool("auto_apply_cancel", "Cancel an automatic application",
		"Withdraw an automatic application that has not been submitted. Refused while it is being "+
			"submitted. Its history is kept.",
		assistant.EffectOverwrites, true,
		func(ctx context.Context, userID int64, slug string) (any, error) {
			if err := h.cancelAutoApply(ctx, userID, slug); err != nil {
				return nil, err
			}
			return map[string]any{"cancelled": true}, nil
		})
}

func (h *assistantHandlers) autoApplyReviewTool() assistant.Tool {
	return assistant.Tool{
		Name:  "auto_apply_review",
		Title: "Approve or decline an automatic application",
		Description: "Record the review of an automatic application whose status is pending_review. " +
			"\"approved\" lets freehire submit it to the employer with the tailored CV and the answers " +
			"auto_apply_get shows; \"declined\" stops it for good. One decision per attempt.",
		Effect: assistant.EffectOverwrites,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_slug": jobSlugProperty,
				"decision": map[string]any{"type": "string", "enum": []string{autoApplyReviewApproved, autoApplyReviewDeclined}},
			},
			"required":             []string{"job_slug", "decision"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				JobSlug  string `json:"job_slug"`
				Decision string `json:"decision"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := h.requireAgentsMayApply(ctx, userID); err != nil {
				return nil, err
			}
			job, row, err := h.autoApplyEntryForSlug(ctx, userID, in.JobSlug)
			if err != nil {
				return nil, toolError(err)
			}
			entry := autoApplyEntry{ID: row.ID, UserID: userID, JobID: job.ID, TailoredCvID: row.TailoredCvID, ReviewDecision: row.ReviewDecision}
			if err := h.decideAutoApplyReview(ctx, entry, in.Decision); err != nil {
				return nil, toolError(err)
			}
			return h.autoApplyView(ctx, userID, in.JobSlug)
		},
	}
}

// The statuses of a pair whose queue row is gone, so only the history can say what became of
// it. Not among autoapply's seven, which all describe a live attempt.
const (
	autoApplyStatusSubmitted = "submitted"
	autoApplyStatusCancelled = "cancelled"
)

// autoApplyAttemptView is one attempt as an agent reads it.
type autoApplyAttemptView struct {
	JobSlug string `json:"job_slug"`
	Status  string `json:"status"`
	// NextStep says what the agent can do about this status, in a sentence.
	NextStep string `json:"next_step"`
	// MayHaveReachedEmployer marks a failure where the employer may already hold the
	// application; such an attempt is never retried.
	MayHaveReachedEmployer bool `json:"may_have_reached_employer,omitempty"`
	// OpenQuestions are the form fields a submission attempt stopped on (status blocked).
	OpenQuestions []autoapply.UnmappedField `json:"open_questions,omitempty"`
	// PendingAnswers are the questions the review preview could not answer yet (status
	// pending_review); will_draft_at_submission ones are written from the CV when it submits.
	PendingAnswers []autoapply.PreviewPending `json:"pending_answers,omitempty"`
	// Answers are what will be sent, as the preview resolved them (status pending_review).
	Answers []autoapply.PreviewField `json:"answers,omitempty"`
	History []autoApplyEventView     `json:"history"`
}

type autoApplyEventView struct {
	At     time.Time       `json:"at"`
	Event  string          `json:"event"`
	Detail json.RawMessage `json:"detail,omitempty"`
}

// autoApplyView assembles the caller's attempt on the job behind slug: the live attempt if
// there is one, otherwise the history of a submitted one.
func (h *assistantHandlers) autoApplyView(ctx context.Context, userID int64, slug string) (any, error) {
	job, err := h.queries.GetJobBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	events, err := h.queries.ListAutoApplyEvents(ctx, db.ListAutoApplyEventsParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return nil, err
	}
	view := autoApplyAttemptView{JobSlug: slug, History: make([]autoApplyEventView, 0, len(events))}
	for _, e := range slices.Backward(events) {
		ev := autoApplyEventView{At: e.CreatedAt.Time.UTC(), Event: e.Kind}
		if string(e.Detail) != "{}" {
			ev.Detail = e.Detail
		}
		view.History = append(view.History, ev)
	}

	info, err := autoApplyReviewInfoForJob(ctx, h.queries, userID, job.ID)
	if err != nil {
		return nil, err
	}
	switch {
	case info != nil:
		view.Status = string(info.Status)
		view.OpenQuestions = info.Unmapped
		if info.ResolvedPreview != nil {
			view.PendingAnswers = info.ResolvedPreview.Pending
			view.Answers = info.ResolvedPreview.Fields
		}
		view.MayHaveReachedEmployer = info.MayHaveReachedEmployer
	case len(view.History) > 0 && slices.Contains([]string{autoApplyStatusSubmitted, autoApplyStatusCancelled}, view.History[len(view.History)-1].Event):
		view.Status = view.History[len(view.History)-1].Event
	default:
		return nil, errors.New(autoApplyNoAttemptMessage)
	}
	view.NextStep = autoApplyNextStep(view.Status, view.MayHaveReachedEmployer)
	return view, nil
}

// autoApplyNextStep tells an agent what it can do about an attempt in this status.
func autoApplyNextStep(status string, mayHaveReachedEmployer bool) string {
	switch autoapply.Status(status) {
	case autoapply.StatusTailoring:
		return "Wait: the CV is being tailored and the application form checked. Check again in a few minutes."
	case autoapply.StatusPendingReview:
		return "Review the answers below. If pending_answers lists questions the person can answer, save them with answer_bank_save and call auto_apply_retry to refresh; then call auto_apply_review."
	case autoapply.StatusApproved:
		return "Wait: it is queued for submission."
	case autoapply.StatusBlocked:
		return "Ask the person every open question at once, as one numbered list, so they can answer \"1 …, 3 …\" in one reply. " +
			"Save each answer in the language of the application form — translate faithfully if they answered in another — " +
			"with answer_bank_save (or screening_answers_set for work authorization, visa, salary, notice period, relocation, age), then call auto_apply_retry."
	case autoapply.StatusFailed:
		if mayHaveReachedEmployer {
			return "Do not apply again automatically: the employer may already have this application. The person should check with the employer."
		}
		return "Call auto_apply_retry to try again."
	case autoapply.StatusTailorFailed:
		return "Call auto_apply_retry to tailor the CV again."
	case autoapply.StatusDeclined:
		return "Stopped by a decline. To apply after all, call auto_apply_cancel, then auto_apply_start."
	case autoApplyStatusSubmitted:
		return "Done: the application was submitted to the employer."
	case autoApplyStatusCancelled:
		return "Cancelled. Call auto_apply_start to apply again."
	default:
		return ""
	}
}

func (h *assistantHandlers) autoApplyListTool() assistant.Tool {
	return assistant.Tool{
		Name:  "auto_apply_list",
		Title: "List automatic applications",
		Description: "List the person's automatic applications: every live one with its status and when " +
			"anything last happened to it, and the ones submitted in the last 30 days. Use auto_apply_get " +
			"for one attempt's questions and history.",
		Effect: assistant.EffectReads,
		Schema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			if err := assistant.DecodeArgs(raw, &struct{}{}); err != nil {
				return nil, err
			}
			return h.autoApplyList(ctx, userID)
		},
	}
}

type autoApplyListItem struct {
	JobSlug        string    `json:"job_slug"`
	Title          string    `json:"title"`
	Company        string    `json:"company"`
	Status         string    `json:"status"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

func (h *assistantHandlers) autoApplyList(ctx context.Context, userID int64) (any, error) {
	live, err := h.queries.ListAutoApplyEntries(ctx, userID)
	if err != nil {
		return nil, err
	}
	submitted, err := h.queries.ListRecentAutoApplySubmissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]autoApplyListItem, 0, len(live)+len(submitted))
	for _, r := range live {
		status := autoapply.DeriveStatus(autoapply.DerivedFrom{
			HasTailoredCV:      r.TailoredCvID != nil,
			HasResolvedPreview: r.HasPreview,
			ReviewDecision:     pgStr(r.ReviewDecision),
			Blocked:            r.BlockedAt.Valid,
			Failed:             r.FailedAt.Valid || r.PreviewFailedAt.Valid,
			TailorFailed:       r.TailorFailedAt.Valid,
		})
		last := r.CreatedAt.Time
		if r.LastEventAt.Valid {
			last = r.LastEventAt.Time
		}
		items = append(items, autoApplyListItem{JobSlug: r.PublicSlug, Title: r.Title, Company: r.Company, Status: string(status), LastActivityAt: last.UTC()})
	}
	for _, r := range submitted {
		items = append(items, autoApplyListItem{JobSlug: r.PublicSlug, Title: r.Title, Company: r.Company, Status: autoApplyStatusSubmitted, LastActivityAt: r.CreatedAt.Time.UTC()})
	}
	slices.SortStableFunc(items, func(a, b autoApplyListItem) int { return b.LastActivityAt.Compare(a.LastActivityAt) })
	return map[string]any{"attempts": items}, nil
}

func (h *assistantHandlers) answerBankListTool() assistant.Tool {
	return assistant.Tool{
		Name:  "answer_bank_list",
		Title: "List saved screening answers",
		Description: "List the person's saved answers to employers' open-ended screening questions " +
			"(\"Why do you want to work here?\", \"Desired start date?\"). Automatic applications reuse " +
			"them on every form that asks the same question.",
		Effect: assistant.EffectReads,
		Schema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			if err := assistant.DecodeArgs(raw, &struct{}{}); err != nil {
				return nil, err
			}
			answers, err := h.answerBank.List(ctx, userID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"answers": toBankedAnswerResponses(answers)}, nil
		},
	}
}

// answerBankSaveTool records an answer as the CANDIDATE's own (answerbank.AuthorCandidate), so
// it is sent on the next form that asks. The account server only ever acts for the person who
// approved the connection, and the description tells the agent to pass on their words rather
// than compose its own — the same rule screening_answers_set already carries.
func (h *assistantHandlers) answerBankSaveTool() assistant.Tool {
	return assistant.Tool{
		Name:  "answer_bank_save",
		Title: "Save a screening answer",
		Description: "Save the person's answer to one screening question an employer's form asks, so " +
			"automatic applications can send it — use the question's wording from auto_apply_get. The " +
			"answer is sent to employers in the person's name: pass on what THEY said, never an answer " +
			"you composed or guessed. Replaces an earlier answer to the same question.",
		Effect: assistant.EffectOverwrites,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string", "description": "The question as the form words it."},
				"answer":   map[string]any{"type": "string", "description": "The person's answer, in their words."},
			},
			"required":             []string{"question", "answer"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in saveAnswerRequest
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := bankSaveError(h.answerBank.Save(ctx, userID, in.Question, in.Answer, answerbank.AuthorCandidate)); err != nil {
				return nil, toolError(err)
			}
			return map[string]any{"saved": true}, nil
		},
	}
}

func (h *assistantHandlers) answerBankDeleteTool() assistant.Tool {
	return assistant.Tool{
		Name:        "answer_bank_delete",
		Title:       "Delete a screening answer",
		Description: "Delete one of the person's saved screening answers by the id answer_bank_list shows.",
		Effect:      assistant.EffectOverwrites,
		Schema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"id": map[string]any{"type": "integer"}},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID int64 `json:"id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := h.answerBank.Delete(ctx, userID, in.ID); err != nil {
				if errors.Is(err, answerbank.ErrNotFound) {
					return nil, errors.New("no such answer")
				}
				return nil, err
			}
			return map[string]any{"deleted": true}, nil
		},
	}
}

func (h *assistantHandlers) screeningAnswersGetTool() assistant.Tool {
	return assistant.Tool{
		Name:  "screening_answers_get",
		Title: "Read screening answers",
		Description: "Read the person's answers to the six screening questions that repeat across " +
			"application forms: countries they may work in, visa sponsorship, desired salary, notice " +
			"period, relocation, and being 18 or older — plus privacy_consent, whether they agree to an " +
			"employer's privacy / personal-data terms being accepted for them on every application. A " +
			"missing field has not been stated.",
		Effect: assistant.EffectReads,
		Schema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			if err := assistant.DecodeArgs(raw, &struct{}{}); err != nil {
				return nil, err
			}
			answers, err := h.screeningAnswers.Get(ctx, userID)
			if errors.Is(err, screeninganswers.ErrNotFound) {
				return screeninganswers.Answers{}, nil
			}
			return answers, err
		},
	}
}
