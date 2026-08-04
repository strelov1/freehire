package inbox

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// The read path must not be able to mark mail read. `read_at` means "a human saw
// this", and the assistant sweeps the backlog through Search on the user's behalf;
// a Search that could mark would silently zero its owner's unread count. Enforcing
// that by convention is what the transport-keyed CV guard did before it stopped
// firing, so enforce it structurally instead: the store this package can reach has
// no read-marking method at all.
func TestTheStoreHasNoReadMarkingMethod(t *testing.T) {
	iface := reflect.TypeOf((*Queries)(nil)).Elem()
	for i := range iface.NumMethod() {
		name := iface.Method(i).Name
		if strings.Contains(name, "MarkEmailRead") || strings.Contains(name, "MarkAllEmailsRead") {
			t.Errorf("inbox.Queries exposes %q; Search must not be able to mark mail read", name)
		}
	}
}

func TestSearchRejectsValuesOutsideTheVocabularies(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query Query
		field string
	}{
		{"source", Query{Source: "imap"}, "source"},
		{"status", Query{Status: "ghosted"}, "status"},
		{"link state", Query{Link: "maybe"}, "link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(&fakeQueries{}, nil).Search(context.Background(), 7, tc.query)

			var invalid *InvalidError
			if !errors.As(err, &invalid) {
				t.Fatalf("Search with a bogus %s = %v, want an *InvalidError", tc.field, err)
			}
			if invalid.Field != tc.field {
				t.Errorf("InvalidError.Field = %q, want %q", invalid.Field, tc.field)
			}
			// The message is the model's only route to self-correction within a turn,
			// so it has to carry the vocabulary rather than just refusing.
			if len(invalid.Valid) == 0 || !strings.Contains(invalid.Error(), invalid.Value) {
				t.Errorf("InvalidError %q does not name the bad value and its vocabulary", invalid.Error())
			}
		})
	}
}

// Bodies are the one listing payload heavy enough to matter, so they are opt-in.
func TestSearchOmitsBodiesUnlessAsked(t *testing.T) {
	q := &fakeQueries{list: []db.ListEmailsRow{{ID: 1, BodyText: "plain", BodyHtml: "<p>rich</p>"}}}

	page, err := New(q, nil).Search(context.Background(), 7, Query{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("Search returned %d messages, want 1", len(page.Messages))
	}
	if page.Messages[0].BodyText != "" {
		t.Errorf("body = %q with WithBody unset, want empty", page.Messages[0].BodyText)
	}
	if q.lastList.WithBody {
		t.Error("Search asked the store for bodies nobody requested")
	}
}

// Many ATS senders mail HTML with no text/plain part, so body_text alone is empty
// and a reader judging from it sees only the subject line. Search must hand back
// the same readable body the classification worker reads.
func TestSearchReadsTheReadableBodyOfHTMLOnlyMail(t *testing.T) {
	q := &fakeQueries{list: []db.ListEmailsRow{{ID: 1, BodyText: "", BodyHtml: "<p>We would like to talk</p>"}}}

	page, err := New(q, nil).Search(context.Background(), 7, Query{WithBody: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(page.Messages[0].BodyText, "We would like to talk") {
		t.Errorf("body = %q, want the HTML part rendered to text", page.Messages[0].BodyText)
	}
}

// A message's label, link state and linked application ride along without bodies:
// that is what makes a body-less page enough to answer most questions.
func TestSearchCarriesTheClassificationWithoutBodies(t *testing.T) {
	q := &fakeQueries{list: []db.ListEmailsRow{{
		ID: 1, FromName: "Workable", Subject: "Interview with Acme",
		ReceivedAt:   pgtype.Timestamptz{Time: time.Unix(1_700_000_000, 0), Valid: true},
		StatusSignal: pgtype.Text{String: "interview_invitation", Valid: true},
		JobID:        pgtype.Int8{Int64: 42, Valid: true},
		LinkedSlug:   pgtype.Text{String: "go-dev-acme", Valid: true},
	}}, total: 1}

	page, err := New(q, nil).Search(context.Background(), 7, Query{Status: "interview_invitation"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := page.Messages[0]
	switch {
	case got.StatusSignal != "interview_invitation":
		t.Errorf("StatusSignal = %q", got.StatusSignal)
	case got.LinkedSlug != "go-dev-acme":
		t.Errorf("LinkedSlug = %q", got.LinkedSlug)
	case got.LinkState != LinkStateLinked:
		t.Errorf("LinkState = %q, want %q", got.LinkState, LinkStateLinked)
	case page.Total != 1:
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

// The three link states partition the mailbox, and a message that is both linked
// and carrying a stale suggestion reads as linked — the resolved answer wins over
// the proposal it superseded.
func TestLinkStatePartitionsTheMailbox(t *testing.T) {
	linked := pgtype.Int8{Int64: 42, Valid: true}
	for _, tc := range []struct {
		name      string
		job, sugg pgtype.Int8
		want      string
	}{
		{"linked", linked, pgtype.Int8{}, LinkStateLinked},
		{"linked over a stale suggestion", linked, pgtype.Int8{Int64: 9, Valid: true}, LinkStateLinked},
		{"suggested", pgtype.Int8{}, pgtype.Int8{Int64: 9, Valid: true}, LinkStateSuggested},
		{"unlinked", pgtype.Int8{}, pgtype.Int8{}, LinkStateUnlinked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQueries{list: []db.ListEmailsRow{{ID: 1, JobID: tc.job, SuggestedJobID: tc.sugg}}}
			page, err := New(q, nil).Search(context.Background(), 7, Query{})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if got := page.Messages[0].LinkState; got != tc.want {
				t.Errorf("LinkState = %q, want %q", got, tc.want)
			}
		})
	}
}

// Asking for the label the default hides is asking for it. Without this the two rules
// cancel and `?status=other` answers "nothing" about its own subject.
func TestAskingForOtherOverridesTheDefault(t *testing.T) {
	for name, q := range map[string]Query{
		"the default":       {},
		"another label":     {Status: "rejection"},
		"asking for other":  {Status: "other"},
		"asking for it all": {IncludeOther: true},
	} {
		want := name == "asking for other" || name == "asking for it all"
		if got := q.showsOther(); got != want {
			t.Errorf("%s: showsOther() = %v, want %v", name, got, want)
		}
	}
}
