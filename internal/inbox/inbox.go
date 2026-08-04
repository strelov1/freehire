// Package inbox is the mail store's use cases: reading the caller's mail,
// classifying it, and resolving which application each message belongs to.
//
// It exists because the mail surface has two readers with nothing in common. One
// is the HTTP API under /me/inbox and /me/emails, driven by the web inbox and by a
// user's own agent harness through the freehire CLI. The other is the in-app
// assistant, which issues no HTTP request, carries no credential, and calls these
// methods directly with the session owner's id. A rule enforced in a Fiber handler
// is a rule the second reader never meets — that is how the CV-tailoring contact
// guard was lost once, and mail carries a rule of exactly that kind (see Search).
//
// So the rules live here and only the rendering is written twice: the handler maps
// these errors to HTTP statuses, the assistant's tools map them to a sentence the
// model can correct itself from.
package inbox

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailclassify"
	"github.com/strelov1/freehire/internal/maillink"
)

// Queries is the slice of the store this package reaches.
//
// It deliberately holds no way to mark a message read. Opening a message sets
// read_at, and read_at means "a human saw this" — an agent sweeping the backlog
// through such a call would silently zero its owner's unread count. Marking read
// stays in the handler, where the caller is a person opening a message.
type Queries interface {
	ListEmails(ctx context.Context, arg db.ListEmailsParams) ([]db.ListEmailsRow, error)
	CountEmails(ctx context.Context, arg db.CountEmailsParams) (db.CountEmailsRow, error)
	CountEmailsByState(ctx context.Context, userID int64) ([]db.CountEmailsByStateRow, error)
	GetEmail(ctx context.Context, arg db.GetEmailParams) (db.GetEmailRow, error)
	GetInterviewInvitation(ctx context.Context, arg db.GetInterviewInvitationParams) (db.GetInterviewInvitationRow, error)
	GetJobBySlug(ctx context.Context, publicSlug string) (db.Job, error)
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

// Applications records an application reconstructed from mail. The mail surface
// borrows the tracking use case rather than writing its own apply, so the
// applied_count guarantee stays in one place.
type Applications interface {
	// source is the appevent source of the recording, derived from the message's own
	// store. An application reconstructed from mail was observed by the mail, and the
	// ledger records that rather than crediting whoever clicked.
	MarkAppliedAt(ctx context.Context, userID int64, slug string, at time.Time, source string) error
}

// Service is the mail use cases.
type Service struct {
	q    Queries
	apps Applications
}

// New builds the service. apps may be nil in a caller that never records an
// application from mail; RecordApplication then reports itself unavailable rather
// than panicking.
func New(q Queries, apps Applications) *Service { return &Service{q: q, apps: apps} }

// The link states partition the mailbox: every live message is in exactly one, so
// their counts always sum to the total. A message that is both linked and carrying
// a stale suggestion reads as linked — the resolved answer wins over the proposal
// it superseded.
const (
	LinkStateLinked    = "linked"
	LinkStateSuggested = "suggested"
	LinkStateUnlinked  = "unlinked"
)

// Sources is the account-switcher vocabulary; "" means every account. 'external'
// is mail the caller's own harness pushed — it reads like any other source, it is
// simply never classified server-side.
var Sources = []string{"gmail", "hosted", "external"}

// LinkStates is the link-state vocabulary; "" means no filter.
var LinkStates = []string{LinkStateLinked, LinkStateSuggested, LinkStateUnlinked}

// ErrNotFound is a message, job or application the caller cannot address. A
// message belonging to someone else is reported as missing, never as forbidden.
var ErrNotFound = errors.New("not found")

// ErrPendingSuggestion refuses to record an application over a matcher proposal
// nobody has answered yet: letting it through would leave the resulting link's
// provenance ambiguous.
var ErrPendingSuggestion = errors.New("the message carries a pending suggestion; confirm or reject it first")

// ErrUnavailable is an operation whose collaborator was not wired up.
var ErrUnavailable = errors.New("unavailable")

// ErrSlugRequired names the one argument that cannot be defaulted: which
// application the message belongs to.
var ErrSlugRequired = errors.New("slug is required — it names the application the message belongs to")

// InvalidError is a caller-supplied value outside a controlled vocabulary. It
// carries the vocabulary because both readers need it: the HTTP caller gets a 400
// that says what was wrong, and the model gets its only route to self-correction
// within the turn.
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
	FromAddr   string
	FromName   string
	Subject    string
	Snippet    string
	// BodyText is the readable body — the text part, or the HTML part rendered down
	// when the sender mailed no text/plain. It is empty unless the query asked for
	// bodies.
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
	// WithBody asks for each message's readable body. It is opt-in because bodies
	// are the one listing payload heavy enough to matter.
	WithBody bool
	// IncludeOther asks for mail the classifier judged not to be about an application at
	// all. It is opt-IN, so the inbox defaults to the applications a person came to find —
	// and Page.Hidden reports how much that default cost them, because a filter nobody can
	// see is a misclassification nobody can find.
	IncludeOther bool
	Limit        int
	Offset       int
}

// showsOther reports whether this query wants the mail the default omits.
//
// Asking FOR the label is asking for it. Without this, `?status=other` and the hide default
// cancel each other out and the filter returns an empty page — a filter that answers
// "nothing" to its own subject is worse than one that does not exist.
func (q Query) showsOther() bool { return q.IncludeOther || q.Status == "other" }

// Validate checks the filters against their controlled vocabularies. Search calls
// it; it is exported for the one caller that reuses the same filters for a
// different statement (marking a filtered page read) and must reject an unknown
// value identically.
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
	// Hidden is the count the default suppressed. Zero when the caller asked for
	// everything, and zero when there was nothing to suppress — the two are the same
	// answer to "what am I not seeing".
	Hidden int64
}

// Search lists the caller's live mail, newest first, under the given filters.
//
// It cannot mark anything read — see Queries. That is the whole reason this method
// exists rather than a loop over Get: a reader sweeping a backlog must not spend
// its owner's unread count to do it.
func (s *Service) Search(ctx context.Context, userID int64, q Query) (Page, error) {
	if err := q.Validate(); err != nil {
		return Page{}, err
	}
	// LIMIT 0 is a legal query returning nothing, so an unset limit must not reach
	// the store as one: a caller that named no page size wants a page, not silence.
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	rows, err := s.q.ListEmails(ctx, db.ListEmailsParams{
		UserID: userID, Src: q.Source, Unread: q.Unread, Status: q.Status, Q: q.Q,
		Unclassified: q.Unclassified, Link: q.Link, WithBody: q.WithBody,
		IncludeOther: q.showsOther(),
		Lim:          int32(limit), Off: int32(max(q.Offset, 0)),
	})
	if err != nil {
		return Page{}, err
	}
	counts, err := s.q.CountEmails(ctx, db.CountEmailsParams{
		UserID: userID, Src: q.Source, Unread: q.Unread, Status: q.Status, Q: q.Q,
		Unclassified: q.Unclassified, Link: q.Link, IncludeOther: q.showsOther(),
	})
	if err != nil {
		return Page{}, err
	}

	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		m := Message{
			ID: r.ID, Source: r.Source, ExternalID: r.ExternalID,
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
			// The same body the classification worker reads, so what our LLM judges
			// and what a caller's agent judges cannot drift. Many ATS senders mail
			// HTML with no text/plain part, and body_text alone is then empty.
			m.BodyText = maillink.ReadableBody(r.BodyText, r.BodyHtml)
		}
		out = append(out, m)
	}
	return Page{Messages: out, Total: counts.Total, Hidden: counts.Hidden}, nil
}

// Get reads one message without touching its read state. It is the shared tail of
// every mutation that returns the message it just changed, so those cannot drift
// from one another.
func (s *Service) Get(ctx context.Context, userID, id int64) (Message, error) {
	row, err := s.q.GetEmail(ctx, db.GetEmailParams{ID: id, UserID: userID})
	if err != nil {
		return Message{}, err
	}
	return Message{
		ID: row.ID, Source: row.Source, ExternalID: row.ExternalID,
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
// one application, so a rehearsal can open on what the employer actually said about
// the format rather than on a guess.
//
// It lives here rather than in the caller for the reason the package doc gives: the
// rule that this read leaves read_at alone is the mail store's rule, and a second
// reader reaching past this package would not meet it. A candidate with no such mail
// gets the store's no-rows error — an ordinary answer, since plenty of interviews are
// arranged where we cannot see them.
//
// The Message it returns is a projection, not a full one: the query selects what a
// rehearsal reads, so Source, ExternalID and the linked/suggested slugs are empty.
func (s *Service) InterviewInvitation(ctx context.Context, userID, jobID int64) (Message, error) {
	row, err := s.q.GetInterviewInvitation(ctx, db.GetInterviewInvitationParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Message{}, err
	}
	return Message{
		ID: row.ID, FromAddr: row.FromAddr, FromName: row.FromName,
		Subject: row.Subject, ReceivedAt: row.ReceivedAt.Time, Read: row.Read,
		// The readable body, not the raw column: an invitation usually comes from an
		// ATS, and an ATS routinely mails HTML with no text/plain part at all.
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
