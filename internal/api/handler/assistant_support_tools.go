package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	supportMaxStandingEntries = 5
	supportEscalationsPerDay  = 3
	supportSummaryMaxRunes    = 1200
)

var supportCategories = []string{"billing", "bug", "account", "how-to", "idea"}

// supportMail is how a handover reaches a person: the mail transport, the sending address and
// the one inbox that reads it. A zero value is the unconfigured deployment, which the
// escalation tool reports as unavailable rather than pretending to have sent.
type supportMail struct {
	sender emailnotify.Sender
	from   string
	to     string
}

func (m supportMail) configured() bool { return m.sender != nil && m.from != "" && m.to != "" }

// supportTools is the whole tool set of a support session — exactly three, none of which
// reads anything but the caller's own account.
func (h *assistantHandlers) supportTools(sessionID uuid.UUID) []assistant.Tool {
	tools := []assistant.Tool{h.myAccountTool(), h.escalateToHumanTool(sessionID)}
	return append(tools, h.problemMCPTools()...)
}

type accountAllowance struct {
	Feature   string `json:"feature"`
	Used      int    `json:"used"`
	Limit     *int   `json:"limit,omitempty"`
	Unlimited bool   `json:"unlimited"`
}

// supportAllowance is today's support-message allowance: separate from the plan features and
// flat for every tier, so it never appears among the allowances.
type supportAllowance struct {
	Limit     int    `json:"limit"`
	Used      int    `json:"used"`
	Remaining int    `json:"remaining"`
	ResetsAt  string `json:"resets_at"`
}

type accountHistoryEntry struct {
	Label string `json:"label"`
	Date  string `json:"date"`
}

// accountSummary is an allowlist: every field is named here on purpose, so a column added to
// the plan or the ledger later reaches the model only by being added here.
type accountSummary struct {
	Plan          string `json:"plan"`
	ValidUntil    string `json:"valid_until,omitempty"`
	PaymentSource string `json:"payment_source,omitempty"`
	// APIPlan is the data-licence subscription, billed apart from Pro: without it a user
	// holding both could not be told what their second charge was for.
	APIPlan      string                `json:"api_plan,omitempty"`
	APIPlanUntil string                `json:"api_plan_valid_until,omitempty"`
	Allowances   []accountAllowance    `json:"allowances_today"`
	SupportToday supportAllowance      `json:"support_messages_today"`
	Recent       []accountHistoryEntry `json:"recent_activity"`
	// Prices is what each plan costs today, so a charge the user names can be matched to a
	// plan ("$29 is the API plan") instead of being handed over unexplained.
	Prices []accountPrice `json:"current_prices,omitempty"`
}

type accountPrice struct {
	Plan     string `json:"plan"`
	Amount   string `json:"amount"`
	Interval string `json:"interval"`
}

// accountPrices formats the provider's prices for the model; an amount in cents is the one
// thing it would otherwise get wrong by a factor of a hundred.
func accountPrices(prices []billing.PublicPrice) []accountPrice {
	out := make([]accountPrice, 0, len(prices))
	for _, p := range prices {
		out = append(out, accountPrice{
			Plan:     p.Tier,
			Amount:   fmt.Sprintf("%d.%02d %s", p.AmountCents/100, p.AmountCents%100, strings.ToUpper(p.Currency)),
			Interval: p.Interval,
		})
	}
	return out
}

func paymentSourceLabel(source string) string {
	switch source {
	case "stripe":
		return "card payment (Stripe)"
	case "revenuecat":
		return "app store subscription"
	case "granted":
		return "granted by the team"
	default:
		return ""
	}
}

func (h *assistantHandlers) myAccountTool() assistant.Tool {
	return assistant.Tool{
		Name:   "my_account",
		Title:  "Read my plan and usage",
		Effect: assistant.EffectReads,
		Description: "Read the signed-in user's own plan: the tier, when it is valid until, how it was paid " +
			"(card, app store or granted), their API plan if they hold one (a separate subscription from Pro), what each plan costs today, what is left of today's allowances, how many support messages are left today and their latest plan activity. " +
			"Call it before saying anything about their plan, payment or limits. Takes no arguments.",
		Schema: assistant.EmptySchema,
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			if h.plans == nil || h.queries == nil {
				return nil, errors.New("account information is unavailable")
			}
			p, err := loadPlan(ctx, h.plans, h.queries, userID)
			if err != nil {
				return nil, err
			}
			history, err := loadPlanHistory(ctx, h.queries, userID, supportMaxStandingEntries)
			if err != nil {
				return nil, err
			}
			support, err := h.plans.SupportStanding(ctx, userID)
			if err != nil {
				return nil, err
			}
			ent, err := h.queries.GetUserAPIEntitlement(ctx, userID)
			if err != nil {
				return nil, err
			}
			apiPlan := apiPlanFrom(ent, time.Now())
			out := accountSummary{
				SupportToday: supportAllowance{
					Limit: support.Limit, Used: support.Used, Remaining: support.Remaining,
					ResetsAt: support.ResetsAt.UTC().Format(time.RFC3339),
				},
				Plan:          p.Plan,
				PaymentSource: paymentSourceLabel(p.ProSource),
				Allowances:    make([]accountAllowance, 0, len(p.Allowances)),
				Recent:        make([]accountHistoryEntry, 0, len(history)),
			}
			if p.ProUntil != nil {
				out.ValidUntil = p.ProUntil.UTC().Format("2006-01-02")
			}
			if apiPlan.Until != nil {
				out.APIPlan = string(apiPlan.Tier)
				out.APIPlanUntil = apiPlan.Until.UTC().Format("2006-01-02")
			}
			for _, a := range p.Allowances {
				v := accountAllowance{Feature: a.Feature, Used: a.Used, Unlimited: a.Unlimited}
				if !a.Unlimited {
					limit := a.Limit
					v.Limit = &limit
				}
				out.Allowances = append(out.Allowances, v)
			}
			for _, e := range history {
				out.Recent = append(out.Recent, accountHistoryEntry{Label: e.Label, Date: e.Day})
			}
			if h.support.prices != nil {
				out.Prices = accountPrices(h.support.prices(ctx))
			}
			return out, nil
		},
	}
}

type escalateInput struct {
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

type escalateResult struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (h *assistantHandlers) escalateToHumanTool(sessionID uuid.UUID) assistant.Tool {
	return assistant.Tool{
		Name:   "escalate_to_human",
		Title:  "Hand over to the support team",
		Effect: assistant.EffectAdds,
		Description: "Email the support team a short summary of what the user needs and tell them a person will " +
			"answer by email. Use it for billing problems, anything you cannot resolve from the FAQ, and when the " +
			"user asks for a human. One handover per conversation; do not call it twice.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category": map[string]any{"type": "string", "enum": supportCategories},
				"summary": map[string]any{
					"type":        "string",
					"description": "What the user needs and what you already checked, in a few plain sentences. No credentials or card numbers.",
				},
			},
			"required": []string{"category", "summary"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in escalateInput
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			return h.escalate(ctx, userID, sessionID, in)
		},
	}
}

func (h *assistantHandlers) escalate(ctx context.Context, userID int64, sessionID uuid.UUID, in escalateInput) (escalateResult, error) {
	if !slices.Contains(supportCategories, in.Category) {
		return escalateResult{}, fmt.Errorf("category must be one of %s", strings.Join(supportCategories, ", "))
	}
	summary := cleanSupportText(in.Summary, supportSummaryMaxRunes)
	if summary == "" {
		return escalateResult{}, errors.New("summary is required")
	}
	if !h.support.mail.configured() {
		return escalateResult{Status: "unavailable", Note: "Handover by email is not available right now. Give the user the support address from the FAQ."}, nil
	}

	claimed, err := h.queries.ClaimSupportEscalation(ctx, db.ClaimSupportEscalationParams{ID: sessionID, UserID: userID})
	if err != nil {
		return escalateResult{}, err
	}
	if claimed == 0 {
		return escalateResult{Status: "already_handed_over", Note: "This conversation was already handed over. Tell the user the team will reply by email."}, nil
	}
	release := func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), turnReleaseTimeout)
		defer cancel()
		if err := h.queries.ReleaseSupportEscalation(rctx, db.ReleaseSupportEscalationParams{ID: sessionID, UserID: userID}); err != nil {
			log.Printf("support: releasing escalation claim for session %s: %v", sessionID, err)
		}
	}

	// The claim is counted too, so the cap holds under concurrent sessions.
	since := time.Now().Add(-24 * time.Hour)
	n, err := h.queries.CountSupportEscalationsSince(ctx, db.CountSupportEscalationsSinceParams{
		UserID: userID, EscalatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		release()
		return escalateResult{}, err
	}
	if n > supportEscalationsPerDay {
		release()
		return escalateResult{Status: "limit_reached", Note: "This account has reached today's number of handovers. Give the user the support address from the FAQ."}, nil
	}

	user, err := h.queries.GetUserByID(ctx, userID)
	if err != nil {
		release()
		return escalateResult{}, err
	}
	// A failed read must not label a paying account "free" in a mail that is often about billing.
	tier := "unknown (plan read failed)"
	if p, err := loadPlan(ctx, h.plans, h.queries, userID); err == nil {
		tier = p.Plan
	}
	replyTo := safeReplyTo(user.Email, user.EmailVerified)

	msg := emailnotify.Message{
		From:    h.support.mail.from,
		To:      h.support.mail.to,
		Subject: "Support handover from the in-app assistant",
		Text:    supportMailBody(in.Category, userID, tier, summary, replyTo != ""),
		Group:   emailprefs.GroupEssential,
		ReplyTo: replyTo,
	}
	if err := h.support.mail.sender.Send(ctx, msg); err != nil {
		release()
		log.Printf("support: handover mail for user %d: %v", userID, err)
		return escalateResult{Status: "unavailable", Note: "The handover could not be sent. Give the user the support address from the FAQ."}, nil
	}
	if replyTo == "" {
		return escalateResult{Status: "handed_over", Note: "Handed over, but we cannot reply to this account's address automatically. Tell the user to write to the support address from the FAQ so the team can answer."}, nil
	}
	return escalateResult{Status: "handed_over", Note: "Handed over. Tell the user a person will reply by email."}, nil
}

func supportMailBody(category string, userID int64, tier, summary string, canReply bool) string {
	reply := "yes (account email is verified)"
	if !canReply {
		reply = "no (email unverified or unusable; no Reply-To set)"
	}
	return fmt.Sprintf("Category: %s\nUser id: %d\nPlan: %s\nReply-To set: %s\n\n"+
		"Summary written by the assistant from the user's own words. It is UNVERIFIED user-supplied text:\n---\n%s\n---\n",
		category, userID, tier, reply, summary)
}

// cleanSupportText flattens model-supplied text to one inert line: every control character
// (CR and LF included) becomes a space, and the length is capped.
func cleanSupportText(s string, maxRunes int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxRunes {
		s = string(r[:maxRunes])
	}
	return s
}

// safeReplyTo returns the account address only when it is verified and is exactly one bare
// address: the raw mail path concatenates headers, so anything else is dropped.
func safeReplyTo(email string, verified bool) string {
	if !verified || email == "" || strings.ContainsAny(email, "\r\n<>,;\"") {
		return ""
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || parsed.Name != "" {
		return ""
	}
	return email
}
