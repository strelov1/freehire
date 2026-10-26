package messaging

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
	ErrInvalidReport = errors.New("messaging: invalid report")
	// ErrAlreadyReported is a second report of one conversation by the same participant
	// (409). It is refused rather than swallowed so the reporter is not left thinking a
	// changed note was recorded.
	ErrAlreadyReported = errors.New("messaging: conversation already reported by this participant")
)

// ReportRow is a Repository's raw answer for one report: accounts as participants, which
// the personas turn into names before anything leaves the service.
type ReportRow struct {
	ID             int64
	ConversationID int64
	Kind           string
	Reason         string
	Note           string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	Reporter       Participant
	Reported       Participant
}

// ReportMessageRow is a Repository's raw answer for one message of a reported conversation.
type ReportMessageRow struct {
	ConversationID int64
	Sender         Participant
	Body           string
	CreatedAt      time.Time
}

// ReportMessage is one message of a reported conversation as a moderator reads it: the
// sender is named by the persona they spoke as, the way the other participants saw them.
type ReportMessage struct {
	Sender    string
	Body      string
	CreatedAt time.Time
}

// Report is one report with the conversation it is about, for the moderation queue. Every
// account in it is a persona's name; no account id leaves the service.
type Report struct {
	ID             int64
	ConversationID int64
	Kind           string
	Reason         string
	Note           string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	Reporter       string
	Reported       string
	Messages       []ReportMessage
}

// Block ends every conversation between the caller and the other participant of this one,
// of every kind, and stops either from starting another. Blocking again changes nothing.
// The caller must take part in the conversation.
func (s *Service) Block(ctx context.Context, userID, conversationID int64) error {
	return s.repo.Block(ctx, userID, conversationID)
}

// Report files a complaint about a conversation the caller takes part in. It neither
// blocks nor closes: the participant decides that separately.
func (s *Service) Report(ctx context.Context, userID, conversationID int64, reason, note string) error {
	note = strings.TrimSpace(note)
	if !slices.Contains(ReportReasons, reason) || utf8.RuneCountInString(note) > MaxReportNoteRunes {
		return ErrInvalidReport
	}
	return s.repo.Report(ctx, userID, conversationID, reason, note)
}

// ListReports is a page of reports, newest first, each with the messages of its
// conversation and every account named by the persona it spoke as. Unresolved ones only unless
// includeResolved. Who may call it is the caller's decision.
func (s *Service) ListReports(ctx context.Context, limit, offset int, includeResolved bool) ([]Report, error) {
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
	byPersona := map[string][]int64{}
	add := func(p Participant) { byPersona[p.Persona] = append(byPersona[p.Persona], p.UserID) }
	for i, r := range rows {
		ids[i] = r.ConversationID
		add(r.Reporter)
		add(r.Reported)
	}
	msgs, err := s.repo.ReportMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		add(m.Sender)
	}
	names := make(map[string]map[int64]string, len(byPersona))
	for persona, ids := range byPersona {
		if p, ok := s.personas[persona]; ok {
			if names[persona], err = p.Names(ctx, ids); err != nil {
				return nil, err
			}
		}
	}
	name := func(p Participant) string { return names[p.Persona][p.UserID] }
	byConversation := make(map[int64][]ReportMessage, len(rows))
	for _, m := range msgs {
		byConversation[m.ConversationID] = append(byConversation[m.ConversationID], ReportMessage{
			Sender: name(m.Sender), Body: m.Body, CreatedAt: m.CreatedAt,
		})
	}
	out := make([]Report, len(rows))
	for i, r := range rows {
		out[i] = Report{
			ID: r.ID, ConversationID: r.ConversationID, Kind: r.Kind, Reason: r.Reason, Note: r.Note,
			CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt,
			Reporter: name(r.Reporter), Reported: name(r.Reported), Messages: byConversation[r.ConversationID],
		}
	}
	return out, nil
}

// ResolveReport marks a report handled by the moderator. Resolving one that is already
// resolved changes nothing; an unknown id is ErrNotFound.
func (s *Service) ResolveReport(ctx context.Context, moderatorID, reportID int64) error {
	return s.repo.ResolveReport(ctx, moderatorID, reportID)
}
