//go:build integration

package interviewscheduling

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/inboxdigest"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

type gmailStub func(*http.Request) (*http.Response, error)

func (f gmailStub) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type calendarStub struct{}

func (calendarStub) Invite(context.Context, bookinglink.Invitation) (string, error) {
	return "https://meet.google.com/abc-defg-hij", nil
}

func TestInterviewIsNegotiatedAndConfirmedInTheOriginalEmailThread(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	var userID, appID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_verified,last_active_at,timezone,pro_until_granted,
		interview_scheduling_enabled_at, interview_calendar_invites_enabled)
		VALUES ('candidate@example.test', true, now(), 'UTC', now() + interval '10 days', now() - interval '1 hour', true)
		RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO applications (user_id,company_slug,role_title,applied_at)
		VALUES ($1,'acme','Engineer',now() - interval '1 day') RETURNING id`, userID).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO emails(user_id,application_id,source,external_id,thread_id,from_addr,
		subject,body_text,received_at,direction,status_signal,classified_at,link_source)
		VALUES($1,$2,'gmail','m1','thread-1','recruiter@example.test','Interview with Acme','Can we schedule an interview?',
			now() - interval '30 minutes','incoming','interview_invitation',now(),'auto')`, userID, appID); err != nil {
		t.Fatal(err)
	}
	cipher, err := tokencrypt.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.Encrypt("test-oauth")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertGmailConnection(ctx, db.UpsertGmailConnectionParams{
		UserID: userID, Email: "candidate@example.test", RefreshTokenEnc: encrypted, Scopes: RequiredScopes(),
	}); err != nil {
		t.Fatal(err)
	}

	thread := []map[string]any{gmailMessage("m1", "<m1@example.test>", "recruiter@example.test", false)}
	bodies := map[string]string{"m1": "Can we schedule an interview?"}
	var sent []string
	client := &http.Client{Transport: gmailStub(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch {
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/threads/thread-1"):
			raw, _ := json.Marshal(map[string]any{"id": "thread-1", "messages": thread})
			body = string(raw)
		case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/messages/"):
			id := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			body = fmt.Sprintf(`{"id":%q,"threadId":"thread-1","payload":{"mimeType":"text/plain","body":{"data":%q}}}`,
				id, base64.RawURLEncoding.EncodeToString([]byte(bodies[id])))
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/messages/send"):
			var payload struct{ ThreadID, Raw string }
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.ThreadID != "thread-1" {
				t.Fatalf("wrong Gmail thread: %v, %v", payload.ThreadID, err)
			}
			raw, err := base64.RawURLEncoding.DecodeString(payload.Raw)
			if err != nil {
				t.Fatal(err)
			}
			msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			reply, _ := io.ReadAll(msg.Body)
			sent = append(sent, string(reply))
			parent := "<m1@example.test>"
			if len(sent) == 2 {
				parent = "<m2@example.test>"
			}
			if got := msg.Header.Get("In-Reply-To"); got != parent {
				t.Fatalf("reply parent = %q, want %q", got, parent)
			}
			id := fmt.Sprintf("sent-%d", len(sent))
			thread = append(thread, gmailMessage(id, msg.Header.Get("Message-ID"), "candidate@example.test", true))
			body = fmt.Sprintf(`{"id":%q,"threadId":"thread-1"}`, id)
		default:
			t.Fatalf("unexpected Google request: %s %s", req.Method, req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	plans := plan.NewStore(q, pool, plan.DefaultConfig())
	bookings := bookinglink.New(bookinglink.NewQueriesRepository(q, pool), plans, []byte("test-booking-key"), nil)
	worker := NewWorker(q, bookings, plans, nil, nil, cipher, gmailsync.NewConnector("test", "test", "https://freehire.test"),
		"https://freehire.test", []int64{userID})
	worker.client = func(context.Context, string) *http.Client { return client }
	worker.planner = func(_ context.Context, _ jsonGenerator, input plannerInput) (Decision, error) {
		switch input.Body {
		case bodies["m1"]:
			return Decision{Action: Offer, Language: "en"}, nil
		case bodies["m2"]:
			return Decision{Action: Accept, Choice: 2, Evidence: "second option", Language: "en"}, nil
		default:
			return Decision{}, fmt.Errorf("unexpected planning input: %q", input.Body)
		}
	}
	run := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE interview_scheduling_threads SET next_at = now() WHERE user_id = $1`, userID); err != nil {
			t.Fatal(err)
		}
		if err := worker.Run(ctx, 5); err != nil {
			t.Fatal(err)
		}
	}
	run()
	if len(sent) != 1 || !strings.Contains(sent[0], "3. ") || !strings.Contains(sent[0], "/booking/") {
		t.Fatalf("expected three times and a link, got %v", sent)
	}
	thread = append(thread, gmailMessage("m2", "<m2@example.test>", "recruiter@example.test", false))
	bodies["m2"] = "The second option works for me."
	run()
	if len(sent) != 1 {
		t.Fatal("agent confirmed before the calendar invitation")
	}
	if err := bookinglink.RunInvitations(ctx, bookinglink.NewInvitationQueries(q, 0), calendarStub{}, 5); err != nil {
		t.Fatal(err)
	}
	run()
	if len(sent) != 2 || !strings.Contains(sent[1], "calendar invitation with a Google Meet link has been sent") {
		t.Fatalf("confirmation not delivered in the thread: %v", sent)
	}
	var matches int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interview_scheduling_threads s
		JOIN application_interviews i ON i.application_id = s.application_id AND i.starts_at = s.offered_starts[2]
		WHERE s.user_id=$1 AND s.status='booked' AND i.join_url='https://meet.google.com/abc-defg-hij'`, userID).Scan(&matches); err != nil || matches != 1 {
		t.Fatalf("second option was not booked with Meet: %d, %v", matches, err)
	}
	run()
	if len(sent) != 2 {
		t.Fatal("another pass duplicated the confirmation")
	}

	digest := inboxdigest.NewDBStore(q)
	digestAt := time.Now().UTC().Truncate(24 * time.Hour).Add(36 * time.Hour)
	candidates, err := digest.Candidates(ctx, digestAt, 10)
	if err != nil || len(candidates) != 1 || candidates[0].UserID != userID {
		t.Fatalf("scheduling-only digest candidate missing: %v, %v", candidates, err)
	}
	snapshot, err := digest.Snapshot(ctx, candidates[0], digestAt)
	if err != nil || len(snapshot.Items) != 0 || len(snapshot.Activity) != 2 ||
		snapshot.Activity[0].Kind != "booking" || snapshot.Activity[1].Kind != "reply" {
		t.Fatalf("digest missed the reply or booking: %+v, %v", snapshot, err)
	}
}

func gmailMessage(id, rfcID, from string, sent bool) map[string]any {
	var labels []string
	if sent {
		labels = []string{"SENT"}
	}
	return map[string]any{
		"id": id, "labelIds": labels,
		"payload": map[string]any{"headers": []map[string]string{
			{"name": "Message-ID", "value": rfcID},
			{"name": "From", "value": from},
			{"name": "Subject", "value": "Interview with Acme"},
		}},
	}
}
