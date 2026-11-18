// Package inbox is the mail store's use cases: reading the caller's mail,
// classifying it, and resolving which application each message belongs to.
//
// The mail surface has two readers: the HTTP API under /me/inbox and /me/emails (web inbox and
// users' agent harnesses), and the in-app assistant, which calls these methods directly with
// the session owner's id. A rule enforced in a Fiber handler is never met by the second reader
// (see Search), so the rules live here and only rendering is written twice: the handler maps
// errors to HTTP statuses, the assistant's tools to a sentence the model can correct from.
package inbox

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/application/maillink"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Queries is the slice of the store this package reaches.
//
// It deliberately holds no way to mark a message read: read_at means "a human saw this", and an
// agent sweeping the backlog would silently zero its owner's unread count. Marking read stays
// in the handler.
type Queries interface {
	ListEmails(ctx context.Context, arg db.ListEmailsParams) ([]db.ListEmailsRow, error)
	CountEmails(ctx context.Context, arg db.CountEmailsParams) (db.CountEmailsRow, error)
	CountEmailsByState(ctx context.Context, userID int64) ([]db.CountEmailsByStateRow, error)
	GetEmail(ctx context.Context, arg db.GetEmailParams) (db.GetEmailRow, error)
	GetInterviewInvitation(ctx context.Context, arg db.GetInterviewInvitationParams) (db.GetInterviewInvitationRow, error)
	// Slugs here resolve to an id only, so use the slim lookup: the wide row would TOAST a
	// description and an enrichment blob on every call.
	GetJobIDBySlug(ctx context.Context, arg db.GetJobIDBySlugParams) (int64, error)
	AgentTriageEmail(ctx context.Context, arg db.AgentTriageEmailParams) (int64, error)
	LinkEmailToJob(ctx context.Context, arg db.LinkEmailToJobParams) (int64, error)
	UnlinkEmail(ctx context.Context, arg db.UnlinkEmailParams) (int64, error)
	ConfirmEmailLink(ctx context.Context, arg db.ConfirmEmailLinkParams) (int64, error)
	RejectEmailLink(ctx context.Context, arg db.RejectEmailLinkParams) (int64, error)
	GetUserJobStage(ctx context.Context, arg db.GetUserJobStageParams) (string, error)
	AdvanceUserJobStage(ctx context.Context, arg db.AdvanceUserJobStageParams) error
	RetractSupersededEmailEvent(ctx context.Context, arg db.RetractSupersededEmailEventParams) (int64, error)
	RecordEmailApplicationEvent(ctx context.Context, arg db.RecordEmailApplicationEventParams) error
}

// Applications records an application reconstructed from mail, borrowing the tracking use case
// so the applied_count guarantee stays in one place.
type Applications interface {
	// source is the appevent source, derived from the message's own store: the ledger records
	// that the mail observed the application, not whoever clicked.
	MarkAppliedAt(ctx context.Context, userID int64, slug string, at time.Time, source string) error
}

// Service is the mail use cases.
type Service struct {
	q      Queries
	apps   Applications
	ingest Ingester
}

// Option configures a Service at construction.
type Option func(*Service)

// WithIngester attaches the harness-push writer. Without it Ingest reports itself unavailable.
func WithIngester(i Ingester) Option { return func(s *Service) { s.ingest = i } }

// New builds the service. apps may be nil in a caller that never records an
// application from mail; RecordApplication then reports itself unavailable rather
// than panicking.
func New(q Queries, apps Applications, opts ...Option) *Service {
	s := &Service{q: q, apps: apps}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// The link states partition the mailbox: every live message is in exactly one, so their counts
// sum to the total. A linked message carrying a stale suggestion reads as linked.
const (
	LinkStateLinked    = "linked"
	LinkStateSuggested = "suggested"
	LinkStateUnlinked  = "unlinked"
)

// Sources is the account-switcher vocabulary; "" means every account. 'external' is mail the
// caller's own harness pushed; it is never classified server-side.
var Sources = []string{"gmail", "hosted", "external"}

// LinkStates is the link-state vocabulary; "" means no filter.
var LinkStates = []string{LinkStateLinked, LinkStateSuggested, LinkStateUnlinked}

// ErrNotFound is a message, job or application the caller cannot address. A
// message belonging to someone else is reported as missing, never as forbidden.
var ErrNotFound = errors.New("not found")

// ErrPendingSuggestion refuses to record an application over an unanswered matcher proposal,
// which would leave the link's provenance ambiguous.
var ErrPendingSuggestion = errors.New("the message carries a pending suggestion; confirm or reject it first")

// ErrUnavailable is an operation whose collaborator was not wired up.
var ErrUnavailable = errors.New("unavailable")

// ErrSlugRequired names the one argument that cannot be defaulted: which
// application the message belongs to.
var ErrSlugRequired = errors.New("slug is required — it names the application the message belongs to")

// InvalidError is a caller-supplied value outside a controlled vocabulary. It carries the
// vocabulary so the HTTP 400 and the model's self-correction can both say what is valid.
type InvalidError struct {
	Field string
	Value string
	Valid []string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("unknown %s %q — valid values are: %s", e.Field, e.Value, strings.Join(e.Valid, ", "))
}

func invalid(field, value string, valid []string) error {
	return &InvalidError{Field: field, Value: value, Valid: valid}
}

// Message is one message as both readers see it. The wire shapes are projections
// of this: the HTTP listing renders a subset, the assistant's tools a smaller one.
type Message struct {
	ID         int64
	Source     string
	ExternalID string
	Direction  string
	ThreadID   string
	ToAddrs    string
	CcAddrs    string
	FromAddr   string
	FromName   string
	Subject    string
	Snippet    string
	// BodyText is the text part, or the HTML part rendered down when there is no text/plain.
	// Empty unless the query asked for bodies.
	BodyText   string
	BodyHTML   string
	ReceivedAt time.Time
	Read       bool

	StatusSignal     string
	LinkSource       string
	LinkState        string
	LinkedSlug       string
	LinkedCompany    string
	SuggestedSlug    string
	SuggestedCompany string
}

// Query filters a listing. Every zero value means "no filter".
type Query struct {
	Source       string
	Status       string
	Link         string
	Q            string
	Unread       bool
	Unclassified bool
	// WithBody asks for each message's readable body; opt-in because bodies are heavy.
	WithBody bool
	// IncludeOther asks for mail the classifier judged not to be about an application. Opt-in;
	// Page.Hidden reports how much the default omitted, so a hidden misclassification is findable.
	IncludeOther bool
	Limit        int
	Offset       int
}

// ShowsOther reports whether this query wants the mail the default omits.
//
// Asking FOR the label is asking for it: otherwise `?status=other` and the hide default cancel
// out and return an empty page. Exported for marking a filtered page read, which must read
// `other` the same way or `read-all?status=other` marks nothing.
func (q Query) ShowsOther() bool { return q.IncludeOther || q.Status == "other" }

// Validate checks the filters against their controlled vocabularies. Search calls
// it; it is exported for marking a filtered page read, which must reject an unknown value
// identically.
func (q Query) Validate() error {
	if q.Source != "" && !slices.Contains(Sources, q.Source) {
		return invalid("source", q.Source, Sources)
	}
	if q.Status != "" && !mailclassify.IsValidSignal(q.Status) {
		return invalid("status", q.Status, mailclassify.SignalValues)
	}
	if q.Link != "" && !slices.Contains(LinkStates, q.Link) {
		return invalid("link", q.Link, LinkStates)
	}
	return nil
}

// DefaultLimit is the page size a caller gets when it names none.
const DefaultLimit = 20

// Page is one listing page, the total matching the same filters, and how many the `other`
// default omitted under those same filters.
type Page struct {
	Messages []Message
	Total    int64
	// Hidden is the count the default suppressed: zero when the caller asked for everything or
	// nothing was suppressed.
	Hidden int64
}

// Search lists the caller's live mail, newest first, under the given filters.
//
// It cannot mark anything read (see Queries), which is why it exists rather than a loop over
// Get.
func (s *Service) Search(ctx context.Context, userID int64, q Query) (Page, error) {
	if err := q.Validate(); err != nil {
		return Page{}, err
	}
	// LIMIT 0 is a legal query returning nothing, so an unset limit must not reach the store.
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	rows, err := s.q.ListEmails(ctx, db.ListEmailsParams{
		UserID: userID, Src: q.Source, Unread: q.Unread, Status: q.Status, Q: q.Q,
		Unclassified: q.Unclassified, Link: q.Link, WithBody: q.WithBody,
		IncludeOther: q.ShowsOther(),
		Lim:          int32(limit), Off: int32(max(q.Offset, 0)),
	})
	if err != nil {
		return Page{}, err
	}
	counts, err := s.q.CountEmails(ctx, db.CountEmailsParams{
		UserID: userID, Src: q.Source, Unread: q.Unread, Status: q.Status, Q: q.Q,
		Unclassified: q.Unclassified, Link: q.Link, IncludeOther: q.ShowsOther(),
	})
	if err != nil {
		return Page{}, err
	}

	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		m := Message{
			ID: r.ID, Source: r.Source, ExternalID: r.ExternalID, Direction: r.Direction, ThreadID: r.ThreadID, ToAddrs: r.ToAddrs, CcAddrs: r.CcAddrs,
			FromAddr: r.FromAddr, FromName: r.FromName, Subject: r.Subject,
			Snippet: r.Snippet, ReceivedAt: r.ReceivedAt.Time, Read: r.Read,
			StatusSignal:     pgStr(r.StatusSignal),
			LinkSource:       pgStr(r.LinkSource),
			LinkState:        linkState(r.JobID, r.SuggestedJobID),
			LinkedSlug:       pgStr(r.LinkedSlug),
			LinkedCompany:    r.LinkedCompany,
			SuggestedSlug:    pgStr(r.SuggestedSlug),
			SuggestedCompany: pgStr(r.SuggestedCompany),
		}
		if q.WithBody {
			// The same body the classification worker reads, so the two judgments cannot drift.
			// Many ATS senders mail HTML only, leaving body_text empty.
			m.BodyText = maillink.ReadableBody(r.BodyText, r.BodyHtml)
		}
		out = append(out, m)
	}
	return Page{Messages: out, Total: counts.Total, Hidden: counts.Hidden}, nil
}

// Get reads one message without touching its read state. It is the shared tail of every
// mutation that returns the message it just changed.
func (s *Service) Get(ctx context.Context, userID, id int64) (Message, error) {
	row, err := s.q.GetEmail(ctx, db.GetEmailParams{ID: id, UserID: userID})
	if err != nil {
		return Message{}, err
	}
	return Message{
		ID: row.ID, Source: row.Source, ExternalID: row.ExternalID, Direction: row.Direction, ThreadID: row.ThreadID, ToAddrs: row.ToAddrs, CcAddrs: row.CcAddrs,
		FromAddr: row.FromAddr, FromName: row.FromName, Subject: row.Subject,
		BodyText: row.BodyText, BodyHTML: row.BodyHtml,
		ReceivedAt: row.ReceivedAt.Time, Read: row.Read,
		StatusSignal:     pgStr(row.StatusSignal),
		LinkSource:       pgStr(row.LinkSource),
		LinkState:        linkState(row.JobID, row.SuggestedJobID),
		LinkedSlug:       pgStr(row.LinkedSlug),
		LinkedCompany:    row.LinkedCompany,
		SuggestedSlug:    pgStr(row.SuggestedSlug),
		SuggestedCompany: pgStr(row.SuggestedCompany),
	}, nil
}

// InterviewInvitation reads the employer's most recent invitation to interview for
// one application, so a rehearsal can open on what the employer said about the format.
//
// It lives here because leaving read_at alone is the mail store's rule. A candidate with no such
// mail gets the store's no-rows error, an ordinary answer.
//
// The Message is a projection: Source, ExternalID and the linked/suggested slugs are empty.
func (s *Service) InterviewInvitation(ctx context.Context, userID, jobID int64) (Message, error) {
	row, err := s.q.GetInterviewInvitation(ctx, db.GetInterviewInvitationParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Message{}, err
	}
	return Message{
		ID: row.ID, FromAddr: row.FromAddr, FromName: row.FromName,
		Subject: row.Subject, ReceivedAt: row.ReceivedAt.Time, Read: row.Read,
		// The readable body, not the raw column: ATS mail is often HTML with no text/plain part.
		BodyText:     maillink.ReadableBody(row.BodyText, row.BodyHtml),
		StatusSignal: string(mailclassify.SignalInterviewInvitation),
		LinkState:    LinkStateLinked,
	}, nil
}

// linkState places a message in exactly one of the three states.
func linkState(job, suggested pgtype.Int8) string {
	switch {
	case job.Valid:
		return LinkStateLinked
	case suggested.Valid:
		return LinkStateSuggested
	default:
		return LinkStateUnlinked
	}
}

// pgStr unwraps a nullable text column to a plain string ("" when NULL).
func pgStr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}
