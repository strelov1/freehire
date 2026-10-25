package buddy

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxReportNoteRunes bounds a report's free-text note.
const MaxReportNoteRunes = 1000

// Report listing page sizes.
const (
	DefaultReportsLimit = 50
	MaxReportsLimit     = 100
)

// ReportReasons is the closed list a report is filed under.
var ReportReasons = []string{"spam", "harassment", "scam", "other"}

var (
	// ErrInvalidReport is a report with a reason outside the list or a note that is too
	// long (400).
	ErrInvalidReport = errors.New("buddy: invalid report")
	// ErrAlreadyReported is a second report of one conversation by the same member (409).
	// It is refused rather than swallowed so the member is not left thinking a changed
	// note was recorded.
	ErrAlreadyReported = errors.New("buddy: conversation already reported by this member")
)

// ReportMessage is one message of a reported conversation as a moderator reads it: the
// sender is a handle, like everywhere else in the network.
type ReportMessage struct {
	SenderHandle string
	Body         string
	CreatedAt    time.Time
}

// ReportRow is a Repository's raw answer for one report.
type ReportRow struct {
	ID             int64
	ConversationID int64
	Reason         string
	Note           string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	ReporterHandle string
	ReportedHandle string
}

// ReportMessageRow is a Repository's raw answer for one message of a reported conversation.
type ReportMessageRow struct {
	ConversationID int64
	ReportMessage
}

// ReportView is a report with the conversation it is about, for the moderation queue.
type ReportView struct {
	ReportRow
	Messages []ReportMessage
}

// Block ends the caller's conversation with the other member and hides the pair from
// each other everywhere: the conversation is closed from whatever status it was in, and
// Similar, the feed, invitations and profiles all refuse the pair from then on. Blocking
// again changes nothing. The caller must be a party to the conversation.
func (s *Service) Block(ctx context.Context, userID, conversationID int64) error {
	if err := s.requireActive(ctx, userID); err != nil {
		return err
	}
	return s.repo.Block(ctx, userID, conversationID)
}

// Report files a complaint about a conversation the caller is a party to. It neither
// blocks nor closes: the member decides that separately.
func (s *Service) Report(ctx context.Context, userID, conversationID int64, reason, note string) error {
	note = strings.TrimSpace(note)
	if !slices.Contains(ReportReasons, reason) || utf8.RuneCountInString(note) > MaxReportNoteRunes {
		return ErrInvalidReport
	}
	if err := s.requireActive(ctx, userID); err != nil {
		return err
	}
	return s.repo.Report(ctx, userID, conversationID, reason, note)
}

// ListReports is a page of reports, newest first, each with the messages of its
// conversation. Unresolved ones only unless includeResolved. Who may call it is the
// caller's decision.
func (s *Service) ListReports(ctx context.Context, limit, offset int, includeResolved bool) ([]ReportView, error) {
	switch {
	case limit <= 0:
		limit = DefaultReportsLimit
	case limit > MaxReportsLimit:
		limit = MaxReportsLimit
	}
	rows, err := s.repo.Reports(ctx, limit, max(offset, 0), includeResolved)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ConversationID
	}
	msgs, err := s.repo.ReportMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	byConversation := make(map[int64][]ReportMessage, len(rows))
	for _, m := range msgs {
		byConversation[m.ConversationID] = append(byConversation[m.ConversationID], m.ReportMessage)
	}
	out := make([]ReportView, len(rows))
	for i, r := range rows {
		out[i] = ReportView{ReportRow: r, Messages: byConversation[r.ConversationID]}
	}
	return out, nil
}

// ResolveReport marks a report handled by the moderator. Resolving one that is already
// resolved changes nothing; an unknown id is ErrNotFound.
func (s *Service) ResolveReport(ctx context.Context, moderatorID, reportID int64) error {
	return s.repo.ResolveReport(ctx, moderatorID, reportID)
}
