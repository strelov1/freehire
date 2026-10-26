package messaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/platform/db"
)

// unreadAfter is how long a message sits unread before its recipient is told. Five minutes
// keeps a live exchange from mailing both people while still reaching someone who closed
// the tab.
const unreadAfter = 5 * time.Minute

// DefaultNotifyMaxPerRun bounds one NotifyUnread pass; what a run does not reach is still
// due on the next one.
const DefaultNotifyMaxPerRun = 500

// The notice is deliberately contentless: who wrote, and what, lives behind authorization
// in the chat, never in an inbox or a Telegram thread.
var noticeHTML = template.Must(mailtpl.Partials().New("messageUnread").Parse(`
{{template "p" .Line}}
{{template "button" (mailLink .URL "Open the chat")}}`))

// EmailSender is the slice of the SES transport the notifier needs; *emailnotify.Client
// satisfies it.
type EmailSender interface {
	Send(ctx context.Context, m emailnotify.Message) error
}

// TelegramSender is the slice of the Telegram client the notifier needs;
// *telegramnotify.Client satisfies it.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, html string) error
}

// Silencer answers whether an account has turned a group of mail off; *emailprefs.Service
// satisfies it. It governs the email channel only — Telegram is something the account
// connected themselves and turns off where they connected it.
type Silencer interface {
	Silenced(ctx context.Context, userID int64, g emailprefs.Group) bool
}

// NotifyResult counts one NotifyUnread pass.
type NotifyResult struct {
	// Notified is the number of notices delivered on at least one channel.
	Notified int
	// Failed is the number whose every channel failed; their claim was released.
	Failed int
}

// NotifierConfig wires a Notifier. Email and Telegram may each be nil, disabling that
// channel.
type NotifierConfig struct {
	Email    EmailSender
	From     string
	Telegram TelegramSender
	Links    *emailprefs.Links
	Silencer Silencer
	// Origin is the site origin the chat link and the mail shell point at.
	Origin string
	// MaxPerRun bounds one pass; zero means DefaultNotifyMaxPerRun.
	MaxPerRun int32
}

// Notifier tells participants about messages they have left unread, in each kind's words.
type Notifier struct {
	q      *db.Queries
	cfg    NotifierConfig
	layout *mailtpl.Layout
	kinds  map[string]Kind
}

// NewNotifier builds a Notifier over the kinds it words notices for. A conversation of a
// kind it was not built with is never claimed, so it stays due for a notifier that knows it.
func NewNotifier(q *db.Queries, cfg NotifierConfig, kinds ...Kind) *Notifier {
	if cfg.MaxPerRun <= 0 {
		cfg.MaxPerRun = DefaultNotifyMaxPerRun
	}
	cfg.Origin = strings.TrimRight(cfg.Origin, "/")
	n := &Notifier{q: q, cfg: cfg, layout: mailtpl.New(cfg.Origin), kinds: make(map[string]Kind, len(kinds))}
	for _, k := range kinds {
		n.kinds[k.Name()] = k
	}
	return n
}

// NotifyUnread sends each participant at most one notice per run of unread messages: a
// participant is due once the oldest message another wrote since its read cursor is five
// minutes old, and is not due again until it reads and a newer message ages the same way.
//
// The claim comes first. A worker that sends and then records sends twice whenever the
// second step fails, so the participant's stamp is moved before anything leaves and only
// the run that moved it sends. When every channel fails the claim is released and the next
// run retries; one channel succeeding counts as sent.
func (n *Notifier) NotifyUnread(ctx context.Context, now time.Time) (NotifyResult, error) {
	due, err := n.q.ListUnreadDue(ctx, db.ListUnreadDueParams{
		Cutoff:  pgtype.Timestamptz{Time: now.Add(-unreadAfter), Valid: true},
		MaxRows: n.cfg.MaxPerRun,
	})
	if err != nil {
		return NotifyResult{}, fmt.Errorf("messaging: reading due notices: %w", err)
	}

	var res NotifyResult
	for _, d := range due {
		kind, ok := n.kinds[d.Kind]
		if !ok {
			continue
		}
		claimedAt := pgtype.Timestamptz{Time: now, Valid: true}
		_, err := n.q.ClaimUnread(ctx, db.ClaimUnreadParams{
			ClaimedAt: claimedAt, ConversationID: d.ConversationID, UserID: d.UserID,
			PrevNotifiedAt: d.PrevNotifiedAt, LastRead: d.LastRead,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // somebody else has it, or the participant read in the meantime
		}
		if err != nil {
			return res, fmt.Errorf("messaging: claiming notice: %w", err)
		}

		if err := n.deliver(ctx, d, kind.Notice(d.ConversationID, d.Pending)); err != nil {
			log.Printf("messaging: notice for conversation %d failed: %v", d.ConversationID, err)
			// The release must land even when the run's own context is what failed the
			// delivery: a claim left in place silences this notice until a newer message.
			rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			rerr := n.q.ReleaseUnread(rctx, db.ReleaseUnreadParams{
				ConversationID: d.ConversationID, UserID: d.UserID,
				PrevNotifiedAt: d.PrevNotifiedAt, ClaimedAt: claimedAt,
			})
			cancel()
			if rerr != nil {
				return res, fmt.Errorf("messaging: releasing claim: %w", rerr)
			}
			res.Failed++
			continue
		}
		res.Notified++
	}
	return res, nil
}

// deliver sends one notice over every enabled channel and records it in the notification
// center. It fails only when at least one channel was tried and none got through.
func (n *Notifier) deliver(ctx context.Context, d db.ListUnreadDueRow, nt Notice) error {
	url := n.cfg.Origin + nt.Path

	var attempted int
	var errs []error

	if n.cfg.Email != nil && d.Email != "" &&
		(n.cfg.Silencer == nil || !n.cfg.Silencer.Silenced(ctx, d.UserID, emailprefs.GroupActivity)) {
		attempted++
		if err := n.sendEmail(ctx, d, nt, url); err != nil {
			errs = append(errs, err)
		}
	}
	if n.cfg.Telegram != nil && d.ChatID.Valid && d.ChatID.Int64 != 0 {
		attempted++
		msg := fmt.Sprintf(`%s <a href="%s">Open the chat</a>.`, html.EscapeString(nt.Line), html.EscapeString(url))
		if err := n.cfg.Telegram.SendMessage(ctx, d.ChatID.Int64, msg); err != nil {
			errs = append(errs, err)
		}
	}
	if attempted > 0 && len(errs) == attempted {
		return errors.Join(errs...)
	}

	// Best-effort history row: a failure here must never fail the delivery it records.
	if _, err := n.q.RecordNotification(ctx, db.RecordNotificationParams{
		UserID: d.UserID, Kind: nt.NotificationKind, Title: nt.Title, Body: nt.Line,
		DedupKey: pgtype.Text{String: fmt.Sprintf("%s:%d:%d", d.Kind, d.ConversationID, d.OldestID), Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("messaging: recording notification: %v", err)
	}
	return nil
}

func (n *Notifier) sendEmail(ctx context.Context, d db.ListUnreadDueRow, nt Notice, url string) error {
	unsubscribe, err := n.cfg.Links.For(d.UserID, emailprefs.GroupActivity)
	if err != nil {
		return fmt.Errorf("messaging: unsubscribe link for user %d: %w", d.UserID, err)
	}
	var content bytes.Buffer
	_ = noticeHTML.Execute(&content, struct{ Line, URL string }{nt.Line, url}) // trusted template; a failure is a template bug
	body := n.layout.Render(mailtpl.Body{
		Preheader:      nt.Title,
		Heading:        nt.Title,
		Content:        template.HTML(content.String()),             //nolint:gosec // rendered by the trusted template above
		Footer:         template.HTML(html.EscapeString(nt.Footer)), //nolint:gosec // escaped here
		UnsubscribeURL: unsubscribe,
	})
	return n.cfg.Email.Send(ctx, emailnotify.Message{
		From: n.cfg.From, To: d.Email, Subject: nt.Title,
		HTML: body, Text: nt.Line + " Open the chat: " + url + "\n" + emailprefs.TextFooter(unsubscribe),
		Group: emailprefs.GroupActivity, UnsubscribeURL: unsubscribe,
	})
}
