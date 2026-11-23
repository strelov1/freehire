package interviewscheduling

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

func RequiredScopes() []string {
	return []string{gmailsync.GmailReadonlyScope, gmailsync.GmailSendScope, gmailsync.CalendarScope, gmailsync.CalendarEventsScope}
}

type Worker struct {
	planner func(context.Context, jsonGenerator, plannerInput) (Decision, error)
	q       *db.Queries
	booking *bookinglink.Service
	plans   *plan.Store
	model   *llm.Client
	keys    *llmkey.Resolver
	cipher  *tokencrypt.Cipher
	client  func(context.Context, string) *http.Client
	origin  string
	allow   []int64

	pilotRecruiter string
}

func NewWorker(q *db.Queries, booking *bookinglink.Service, plans *plan.Store, model *llm.Client,
	keys *llmkey.Resolver, cipher *tokencrypt.Cipher, connector *gmailsync.Connector, origin string, allow []int64, pilotRecruiter string,
) *Worker {
	return &Worker{planner: readDecision, q: q, booking: booking, plans: plans, model: model, keys: keys,
		cipher: cipher, client: connector.HTTPClient, origin: strings.TrimRight(origin, "/"),
		allow: append([]int64{}, allow...), pilotRecruiter: pilotRecruiter}
}

func (w *Worker) Run(ctx context.Context, limit int) error {
	if err := w.q.SeedInterviewSchedulingThreads(ctx, db.SeedInterviewSchedulingThreadsParams{
		RequiredScopes: RequiredScopes(), AllowUsers: w.allow, PilotRecruiter: w.pilotRecruiter,
	}); err != nil {
		return err
	}
	var failures error
	for range limit {
		row, err := w.q.ClaimInterviewSchedulingThread(ctx, db.ClaimInterviewSchedulingThreadParams{
			RequiredScopes: RequiredScopes(), AllowUsers: w.allow, PilotRecruiter: w.pilotRecruiter,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return errors.Join(failures, err)
		}
		threadCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		err = w.handle(threadCtx, row)
		cancel()
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("scheduling thread %d: %w", row.ID, err))
		}
	}
	return failures
}

func (w *Worker) handle(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow) error {
	if row.Status != "calendar" {
		if booked, err := w.q.FinishSchedulingLinkBooking(ctx, row.ID); err != nil || booked > 0 {
			return err
		}
	}
	refresh, err := w.cipher.Decrypt(row.RefreshTokenEnc)
	if err != nil {
		return err
	}
	client := w.client(ctx, refresh)
	gmail := gmailsync.NewReplyClient(client)
	thread, err := gmail.Thread(ctx, row.GmailThreadID)
	if err != nil {
		return err
	}
	if row.PendingKind != "" {
		return w.deliver(ctx, row, gmail, thread)
	}
	if len(thread) == 0 {
		return w.end(ctx, row, "review")
	}
	latest := thread[len(thread)-1]
	if automationInterrupted(thread, row) {
		return w.end(ctx, row, "closed")
	}
	if row.Status == "calendar" {
		if latest.ID != row.LastProcessedID {
			return w.end(ctx, row, "review")
		}
		return w.reserve(ctx, row, latest)
	}
	if latest.Sent || latest.ID == row.LastProcessedID {
		return w.followup(ctx, row, thread)
	}
	if row.LastProcessedID == "" && latest.ID != row.AnchorProviderID || !replyable(latest, row.RecruiterEmail) {
		return w.end(ctx, row, "review")
	}

	message, err := gmailsync.NewAPIReader(client, nil).GetMessage(ctx, latest.ID)
	if err != nil {
		return err
	}
	body := message.BodyText
	if body == "" {
		body = html.UnescapeString(htmltext.ToText(message.BodyHTML))
	}
	body = stripQuotedBody(body)
	if message.CalendarUID != "" || !schedulingBody(body) {
		return w.end(ctx, row, "review")
	}
	decision, err := w.decide(ctx, row, latest.ID, latest.Subject, body)
	if errors.Is(err, plan.ErrRefused) || errors.Is(err, plan.ErrPaymentRequired) {
		return w.end(ctx, row, "review")
	}
	if err != nil {
		return err
	}
	return w.apply(ctx, row, latest, body, decision)
}

func replyable(message gmailsync.ThreadMessage, recipient string) bool {
	address, err := mail.ParseAddress(message.From)
	if err != nil || message.Sent || message.Automated || message.MessageID == "" ||
		!strings.EqualFold(address.Address, recipient) {
		return false
	}
	local, _, _ := strings.Cut(strings.ToLower(address.Address), "@")
	if strings.Contains(local, "noreply") || strings.Contains(local, "no-reply") ||
		strings.Contains(local, "donotreply") || strings.Contains(local, "do-not-reply") {
		return false
	}
	if message.ReplyTo == "" {
		return true
	}
	replyTo, err := mail.ParseAddress(message.ReplyTo)
	return err == nil && strings.EqualFold(replyTo.Address, recipient)
}

func stripQuotedBody(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ">") || line == "--" ||
			(strings.HasPrefix(line, "On ") && strings.HasSuffix(line, " wrote:")) {
			return strings.Join(lines[:i], "\n")
		}
	}
	return body
}

func schedulingBody(body string) bool {
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > 6000 {
		return false
	}
	body = strings.ToLower(body)
	for _, forbidden := range []string{
		"calendly.", "goodtime.", "ashbyhq.com/scheduling", "salary", "compensation", "portfolio",
		"social security", "passport", "bank account", "credit card", "ignore previous", "ignore the instructions",
	} {
		if strings.Contains(body, forbidden) {
			return false
		}
	}
	return true
}

func (w *Worker) decide(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, gmailID, subject, body string) (Decision, error) {
	if _, err := w.plans.RequirePaid(ctx, row.UserID); err != nil {
		return Decision{}, err
	}
	ref := fmt.Sprintf("interview-scheduling:%d:%s", row.ID, gmailID)
	if _, err := w.plans.Consume(ctx, row.UserID, plan.FeatureMailReply, ref); err != nil {
		return Decision{}, err
	}
	model := llmkey.Bind(plan.WithPaidAI(ctx), w.keys, w.model, row.UserID, llm.Feature("interview-scheduling"))
	times := make([]time.Time, 0, len(row.OfferedStarts))
	for _, at := range row.OfferedStarts {
		times = append(times, at.Time)
	}
	decision, err := w.planner(ctx, model, plannerInput{
		Subject: subject, Body: body, Now: time.Now().UTC(), Offers: times, Zone: row.OfferZone,
	})
	if err != nil {
		return Decision{}, errors.Join(errors.New("scheduling model unavailable"),
			w.plans.Release(context.WithoutCancel(ctx), row.UserID, plan.FeatureMailReply, ref))
	}
	return decision, nil
}

func (w *Worker) end(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, status string) error {
	return w.q.EndInterviewScheduling(ctx, db.EndInterviewSchedulingParams{ID: row.ID, Status: status})
}

func (w *Worker) apply(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, parent gmailsync.ThreadMessage, body string, d Decision) error {
	zone := zoneFor(row, d)
	switch d.Action {
	case Offer, Decline:
		return w.offer(ctx, row, parent, d.Language, zone)
	case Accept, Propose:
		offered := make([]time.Time, 0, len(row.OfferedStarts))
		for _, start := range row.OfferedStarts {
			offered = append(offered, start.Time)
		}
		times := decidedTimes(body, d, offered, zone)
		if len(times) == 0 {
			return w.end(ctx, row, "review")
		}
		at, err := w.firstAvailable(ctx, row, times, zone)
		if err != nil {
			return err
		}
		if at.IsZero() {
			return w.offer(ctx, row, parent, d.Language, zone)
		}
		return w.q.SelectInterviewTime(ctx, db.SelectInterviewTimeParams{
			ID: row.ID, MessageID: parent.ID, StartsAt: pgconv.Instant(at), Language: language(d.Language), OfferZone: zone.String(),
		})
	default:
		return w.end(ctx, row, "review")
	}
}

func findMessage(thread []gmailsync.ThreadMessage, id string) (gmailsync.ThreadMessage, bool) {
	index := slices.IndexFunc(thread, func(message gmailsync.ThreadMessage) bool { return message.ID == id })
	if index == -1 {
		return gmailsync.ThreadMessage{}, false
	}
	return thread[index], true
}

// Stop when the candidate has sent anything after the last agent reply. A missing
// checkpoint also makes it impossible to verify the thread's intervening history.
func automationInterrupted(thread []gmailsync.ThreadMessage, row db.ClaimInterviewSchedulingThreadRow) bool {
	checkpoint := row.LastSentID
	if checkpoint == "" {
		checkpoint = row.AnchorProviderID
	}
	index := slices.IndexFunc(thread, func(message gmailsync.ThreadMessage) bool { return message.ID == checkpoint })
	return index < 0 || slices.ContainsFunc(thread[index+1:], func(message gmailsync.ThreadMessage) bool { return message.Sent })
}
