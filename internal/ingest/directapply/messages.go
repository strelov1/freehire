package directapply

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/mailbox"
	"github.com/strelov1/freehire/internal/platform/db"
)

var (
	ErrMessageUnavailable = errors.New("candidate email is not available")
	ErrMessageConflict    = errors.New("message key is already in use; reload the conversation")
)

// CandidateMessageMailer delivers the employer's plain-text message with the supplied reply address.
type CandidateMessageMailer interface {
	SendCandidateMessage(ctx context.Context, to, replyTo, title, company, body string) error
}

// ApplicationMessages owns the private conversation for a delivered direct application.
type ApplicationMessages struct {
	q      *db.Queries
	mailer CandidateMessageMailer
	domain string
}

func NewApplicationMessages(q *db.Queries, mailer CandidateMessageMailer, domain string) *ApplicationMessages {
	return &ApplicationMessages{q: q, mailer: mailer, domain: strings.TrimSpace(domain)}
}

type ApplicationMessage struct {
	ID         int64
	Body       string
	Status     string
	CreatedAt  time.Time
	RequestKey string
}

type ApplicationConversation struct {
	Messages []ApplicationMessage
	HasMore  bool
	CanSend  bool
}

const messagePageSize = 50

func (m *ApplicationMessages) target(ctx context.Context, employerID, applicationID int64) (db.GetEmployerApplicationMessageTargetRow, error) {
	target, err := m.q.GetEmployerApplicationMessageTarget(ctx, db.GetEmployerApplicationMessageTargetParams{
		ApplicationID: applicationID, EmployerID: employerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetEmployerApplicationMessageTargetRow{}, ErrNotFound
	}
	return target, err
}

func (m *ApplicationMessages) History(ctx context.Context, employerID, applicationID, before int64) (ApplicationConversation, error) {
	if before < 0 {
		return ApplicationConversation{}, fmt.Errorf("%w: invalid message cursor", ErrInvalid)
	}
	target, err := m.target(ctx, employerID, applicationID)
	if err != nil {
		return ApplicationConversation{}, err
	}
	rows, err := m.q.ListEmployerApplicationMessages(ctx, db.ListEmployerApplicationMessagesParams{
		ApplicationID: applicationID, EmployerID: employerID, BeforeID: before, RowLimit: messagePageSize + 1,
	})
	if err != nil {
		return ApplicationConversation{}, err
	}
	history := ApplicationConversation{
		HasMore:  len(rows) > messagePageSize,
		CanSend:  target.EmailVerified && m.mailer != nil && m.domain != "",
		Messages: make([]ApplicationMessage, 0, min(len(rows), messagePageSize)),
	}
	for _, row := range rows[:min(len(rows), messagePageSize)] {
		msg := ApplicationMessage{ID: row.ID, Body: row.Body, Status: row.Status, CreatedAt: row.CreatedAt.Time}
		if row.RequestKey.Valid {
			msg.RequestKey = uuid.UUID(row.RequestKey.Bytes).String()
		}
		history.Messages = append(history.Messages, msg)
	}
	return history, nil
}

func (m *ApplicationMessages) MarkRead(ctx context.Context, employerID, applicationID, through int64) (bool, error) {
	if through <= 0 {
		return false, fmt.Errorf("%w: invalid message ID", ErrInvalid)
	}
	if err := m.q.MarkEmployerApplicationRepliesRead(ctx, db.MarkEmployerApplicationRepliesReadParams{
		ApplicationID: applicationID, EmployerID: employerID, ThroughID: through,
	}); err != nil {
		return false, err
	}
	target, err := m.target(ctx, employerID, applicationID)
	return target.HasUnreadReply, err
}

func (m *ApplicationMessages) Send(ctx context.Context, employerID, applicationID int64, requestKey, body string) (ApplicationMessage, error) {
	key, err := uuid.Parse(requestKey)
	if err != nil || key == uuid.Nil || key.String() != requestKey {
		return ApplicationMessage{}, fmt.Errorf("%w: invalid message key", ErrInvalid)
	}
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > maxMessage || strings.ContainsRune(body, 0) {
		return ApplicationMessage{}, fmt.Errorf("%w: invalid message body", ErrInvalid)
	}
	target, err := m.target(ctx, employerID, applicationID)
	if err != nil {
		return ApplicationMessage{}, err
	}
	if !target.EmailVerified || m.mailer == nil || m.domain == "" {
		return ApplicationMessage{}, ErrMessageUnavailable
	}

	request := db.GetEmployerApplicationMessageRequestParams{
		ApplicationID: applicationID, EmployerID: employerID, Body: body,
		RequestKey: pgtype.UUID{Bytes: key, Valid: true},
	}
	started, err := m.q.BeginEmployerApplicationMessage(ctx, db.BeginEmployerApplicationMessageParams{
		ApplicationID: applicationID, EmployerID: employerID, Body: body, RequestKey: request.RequestKey,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ApplicationMessage{}, err
	}
	if err == nil {
		if err := m.deliver(ctx, started, body); err != nil {
			return ApplicationMessage{}, err
		}
	}

	row, err := m.q.GetEmployerApplicationMessageRequest(ctx, request)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationMessage{}, ErrMessageConflict
	}
	if err != nil {
		return ApplicationMessage{}, err
	}
	return ApplicationMessage{ID: row.ID, Body: row.Body, Status: row.Status, CreatedAt: row.CreatedAt.Time, RequestKey: requestKey}, nil
}

func (m *ApplicationMessages) deliver(ctx context.Context, started db.BeginEmployerApplicationMessageRow, body string) error {
	// The attempt is already recorded. Let the mail call and its outcome write finish
	// even if the browser disconnects, with separate bounds for transport and storage.
	ctx = context.WithoutCancel(ctx)
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	address := mailbox.ApplicationReplyAddress(uuid.UUID(started.ReplyToken.Bytes), m.domain)
	err := m.mailer.SendCandidateMessage(sendCtx, started.Email, address, started.Title, started.Company, body)
	cancel()
	status := "sent"
	if err != nil {
		status = "failed"
		log.Printf("directapply: candidate message %d could not confirm delivery", started.ID)
	}
	saveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return m.q.FinishEmployerApplicationMessage(saveCtx, db.FinishEmployerApplicationMessageParams{
		ID: started.ID, Status: status, StartedAt: started.UpdatedAt,
	})
}
