package pushnotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakePruner records which tokens were pruned, so tests can assert the
// notifier calls it exactly when Expo reports a token permanently dead.
type fakePruner struct {
	pruned []string
}

func (p *fakePruner) PruneDeadPushToken(ctx context.Context, token string) error {
	p.pruned = append(p.pruned, token)
	return nil
}

// fakeQueuer records enqueued tickets in place of the real outbox table.
type fakeQueuer struct {
	enqueued []Ticket
}

func (q *fakeQueuer) EnqueuePushTicket(ctx context.Context, token, ticketID string) error {
	q.enqueued = append(q.enqueued, Ticket{Token: token, TicketID: ticketID})
	return nil
}

// fakeTicketStore stands in for the outbox's claim/delete queries.
type fakeTicketStore struct {
	due     []Ticket
	deleted []int64
}

func (s *fakeTicketStore) ClaimDuePushTickets(ctx context.Context, minAgeMinutes, batchSize int32) ([]Ticket, error) {
	return s.due, nil
}

func (s *fakeTicketStore) DeletePushTickets(ctx context.Context, ids []int64) error {
	s.deleted = append(s.deleted, ids...)
	return nil
}

func stubExpo(t *testing.T, status string, errorCode string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []struct {
			To string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		msg := map[string]any{"status": status}
		if status == "ok" {
			msg["id"] = "receipt-1"
		} else {
			msg["message"] = "send failed"
			msg["details"] = map[string]any{"error": errorCode}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{msg}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestNotifier(pruner TokenPruner, queuer TicketQueuer, tickets TicketStore) *ExpoNotifier {
	n := NewExpoNotifier(pruner, queuer, tickets)
	return n
}

func TestExpoNotifier_Send_SuccessEnqueuesTicketForLaterCheck(t *testing.T) {
	srv := stubExpo(t, "ok", "")
	pruner := &fakePruner{}
	queuer := &fakeQueuer{}
	n := newTestNotifier(pruner, queuer, &fakeTicketStore{})
	n.apiURL = srv.URL

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none on success", pruner.pruned)
	}
	if len(queuer.enqueued) != 1 || queuer.enqueued[0] != (Ticket{Token: "ExponentPushToken[abc]", TicketID: "receipt-1"}) {
		t.Errorf("enqueued = %v, want one ticket for ExponentPushToken[abc]/receipt-1", queuer.enqueued)
	}
}

func TestExpoNotifier_Send_DeviceNotRegisteredPrunesTokenAtSendTime(t *testing.T) {
	srv := stubExpo(t, "error", "DeviceNotRegistered")
	pruner := &fakePruner{}
	queuer := &fakeQueuer{}
	n := newTestNotifier(pruner, queuer, &fakeTicketStore{})
	n.apiURL = srv.URL

	err := n.Send(context.Background(), "ExponentPushToken[dead]", "Hello", "World")
	if !errors.Is(err, ErrTokenPruned) {
		t.Fatalf("Send err = %v, want ErrTokenPruned — a caller (e.g. a self-test endpoint) must be able to tell a prune apart from a real delivery", err)
	}
	if len(pruner.pruned) != 1 || pruner.pruned[0] != "ExponentPushToken[dead]" {
		t.Errorf("pruned = %v, want [ExponentPushToken[dead]]", pruner.pruned)
	}
	if len(queuer.enqueued) != 0 {
		t.Errorf("enqueued = %v, want none — a token already known dead has nothing to check later", queuer.enqueued)
	}
}

func TestExpoNotifier_Send_OtherErrorSurfacesWithoutPruningOrEnqueuing(t *testing.T) {
	srv := stubExpo(t, "error", "MessageTooBig")
	pruner := &fakePruner{}
	queuer := &fakeQueuer{}
	n := newTestNotifier(pruner, queuer, &fakeTicketStore{})
	n.apiURL = srv.URL

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World"); err == nil {
		t.Fatal("Send: want error for a non-DeviceNotRegistered failure")
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none — a transient error must not delete the token", pruner.pruned)
	}
	if len(queuer.enqueued) != 0 {
		t.Errorf("enqueued = %v, want none — nothing to check later for a failed send", queuer.enqueued)
	}
}

// stubExpoReceipts serves Expo's getReceipts shape: {"data": {ticketID: {...}}}.
func stubExpoReceipts(t *testing.T, receipts map[string]map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": receipts})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExpoNotifier_CheckReceipts_PrunesFreshlyDeadToken(t *testing.T) {
	srv := stubExpoReceipts(t, map[string]map[string]any{
		"ticket-1": {"status": "error", "message": "not delivered", "details": map[string]any{"error": "DeviceNotRegistered"}},
	})
	pruner := &fakePruner{}
	store := &fakeTicketStore{due: []Ticket{{ID: 1, Token: "ExponentPushToken[fresh-dead]", TicketID: "ticket-1"}}}
	n := newTestNotifier(pruner, &fakeQueuer{}, store)
	n.receiptsURL = srv.URL

	if err := n.CheckReceipts(context.Background()); err != nil {
		t.Fatalf("CheckReceipts: %v", err)
	}
	if len(pruner.pruned) != 1 || pruner.pruned[0] != "ExponentPushToken[fresh-dead]" {
		t.Errorf("pruned = %v, want [ExponentPushToken[fresh-dead]]", pruner.pruned)
	}
	if len(store.deleted) != 1 || store.deleted[0] != 1 {
		t.Errorf("deleted ticket ids = %v, want [1]", store.deleted)
	}
}

func TestExpoNotifier_CheckReceipts_DeliveredTicketClearedWithoutPruning(t *testing.T) {
	srv := stubExpoReceipts(t, map[string]map[string]any{
		"ticket-2": {"status": "ok"},
	})
	pruner := &fakePruner{}
	store := &fakeTicketStore{due: []Ticket{{ID: 2, Token: "ExponentPushToken[fine]", TicketID: "ticket-2"}}}
	n := newTestNotifier(pruner, &fakeQueuer{}, store)
	n.receiptsURL = srv.URL

	if err := n.CheckReceipts(context.Background()); err != nil {
		t.Fatalf("CheckReceipts: %v", err)
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none for a delivered ticket", pruner.pruned)
	}
	if len(store.deleted) != 1 || store.deleted[0] != 2 {
		t.Errorf("deleted ticket ids = %v, want [2]", store.deleted)
	}
}

func TestExpoNotifier_CheckReceipts_NothingDueIsANoOp(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	t.Cleanup(srv.Close)

	store := &fakeTicketStore{due: nil}
	n := newTestNotifier(&fakePruner{}, &fakeQueuer{}, store)
	n.receiptsURL = srv.URL

	if err := n.CheckReceipts(context.Background()); err != nil {
		t.Fatalf("CheckReceipts: %v", err)
	}
	if called {
		t.Error("getReceipts was called with nothing due")
	}
}

// A ticket Expo doesn't answer for (absent from the getReceipts response —
// no answer ready yet, or an id Expo no longer recognizes) must not be
// treated as "checked, resolved": deleting it from the outbox would give up
// its only detection window for going dead. Left queued, the next scheduled
// pass claims it again — matching the design's no-retry-bookkeeping posture
// (the row simply stays due), not a special retry path.
func TestExpoNotifier_CheckReceipts_TicketAbsentFromResponseIsLeftQueued(t *testing.T) {
	srv := stubExpoReceipts(t, map[string]map[string]any{
		"ticket-answered": {"status": "ok"},
		// ticket-no-answer deliberately has no entry.
	})
	pruner := &fakePruner{}
	store := &fakeTicketStore{due: []Ticket{
		{ID: 1, Token: "ExponentPushToken[answered]", TicketID: "ticket-answered"},
		{ID: 2, Token: "ExponentPushToken[no-answer]", TicketID: "ticket-no-answer"},
	}}
	n := newTestNotifier(pruner, &fakeQueuer{}, store)
	n.receiptsURL = srv.URL

	if err := n.CheckReceipts(context.Background()); err != nil {
		t.Fatalf("CheckReceipts: %v", err)
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none", pruner.pruned)
	}
	if len(store.deleted) != 1 || store.deleted[0] != 1 {
		t.Errorf("deleted ticket ids = %v, want [1] — only the answered ticket", store.deleted)
	}
}
