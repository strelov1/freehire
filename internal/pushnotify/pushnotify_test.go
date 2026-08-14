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
// errFor lets a test make a specific token's prune call fail, to exercise
// CheckReceipts' handling of a partial-batch prune failure.
type fakePruner struct {
	pruned []string
	errFor map[string]error
}

func (p *fakePruner) PruneDeadPushToken(ctx context.Context, token string) error {
	if err := p.errFor[token]; err != nil {
		return err
	}
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
	return stubExpoCapture(t, status, errorCode, nil)
}

// stubExpoCapture is stubExpo with a hook to capture the decoded request body,
// so a test can assert on what was actually sent to Expo (e.g. the data payload).
func stubExpoCapture(t *testing.T, status string, errorCode string, capture *[]expoMessage) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []expoMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if capture != nil {
			*capture = body
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

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World", nil); err != nil {
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

	err := n.Send(context.Background(), "ExponentPushToken[dead]", "Hello", "World", nil)
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

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World", nil); err == nil {
		t.Fatal("Send: want error for a non-DeviceNotRegistered failure")
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none — a transient error must not delete the token", pruner.pruned)
	}
	if len(queuer.enqueued) != 0 {
		t.Errorf("enqueued = %v, want none — nothing to check later for a failed send", queuer.enqueued)
	}
}

func TestExpoNotifier_Send_IncludesDataPayload(t *testing.T) {
	var captured []expoMessage
	srv := stubExpoCapture(t, "ok", "", &captured)
	n := newTestNotifier(&fakePruner{}, &fakeQueuer{}, &fakeTicketStore{})
	n.apiURL = srv.URL

	err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World", map[string]string{"slug": "acme-backend-engineer"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured %d messages, want 1", len(captured))
	}
	if got := captured[0].Data["slug"]; got != "acme-backend-engineer" {
		t.Errorf("data[slug] = %q, want %q", got, "acme-backend-engineer")
	}
}

func TestExpoNotifier_Send_NilDataOmitsPayloadField(t *testing.T) {
	var captured []expoMessage
	srv := stubExpoCapture(t, "ok", "", &captured)
	n := newTestNotifier(&fakePruner{}, &fakeQueuer{}, &fakeTicketStore{})
	n.apiURL = srv.URL

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured %d messages, want 1", len(captured))
	}
	if captured[0].Data != nil {
		t.Errorf("data = %v, want nil", captured[0].Data)
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

// fakeDeviceNotifier is a Notifier test double keyed by token, for exercising
// SendToDevices without going through an HTTP round trip.
type fakeDeviceNotifier struct {
	results map[string]error // token -> error to return; missing key = nil
	calls   []string
}

func (f *fakeDeviceNotifier) Send(ctx context.Context, token, title, body string, data map[string]string) error {
	f.calls = append(f.calls, token)
	return f.results[token]
}

// notifierFunc adapts a plain function to Notifier, mirroring http.HandlerFunc.
type notifierFunc func(ctx context.Context, token, title, body string, data map[string]string) error

func (f notifierFunc) Send(ctx context.Context, token, title, body string, data map[string]string) error {
	return f(ctx, token, title, body, data)
}

func TestSendToDevices_AllSucceed(t *testing.T) {
	n := &fakeDeviceNotifier{}
	tokens := []string{"tok-1", "tok-2", "tok-3"}

	if err := SendToDevices(context.Background(), n, tokens, "Hello", "World", nil); err != nil {
		t.Fatalf("SendToDevices: %v", err)
	}
	if len(n.calls) != 3 {
		t.Errorf("calls = %v, want all 3 tokens attempted", n.calls)
	}
}

func TestSendToDevices_PartialSuccessCountsAsDelivered(t *testing.T) {
	n := &fakeDeviceNotifier{results: map[string]error{
		"tok-bad": errors.New("transient failure"),
	}}
	tokens := []string{"tok-good", "tok-bad"}

	if err := SendToDevices(context.Background(), n, tokens, "Hello", "World", nil); err != nil {
		t.Fatalf("SendToDevices: %v, want nil — at least one device received it", err)
	}
	if len(n.calls) != 2 {
		t.Errorf("calls = %v, want both tokens attempted (no short-circuit)", n.calls)
	}
}

func TestSendToDevices_AllPrunedIsAFailure(t *testing.T) {
	n := &fakeDeviceNotifier{results: map[string]error{
		"tok-1": ErrTokenPruned,
		"tok-2": ErrTokenPruned,
	}}
	tokens := []string{"tok-1", "tok-2"}

	if err := SendToDevices(context.Background(), n, tokens, "Hello", "World", nil); err == nil {
		t.Fatal("SendToDevices: want an error when every device was pruned, nothing delivered")
	}
}

func TestSendToDevices_AllFailed(t *testing.T) {
	n := &fakeDeviceNotifier{results: map[string]error{
		"tok-1": errors.New("boom"),
		"tok-2": errors.New("boom"),
	}}
	tokens := []string{"tok-1", "tok-2"}

	if err := SendToDevices(context.Background(), n, tokens, "Hello", "World", nil); err == nil {
		t.Fatal("SendToDevices: want an error when every device failed")
	}
}

func TestSendToDevices_EmptyTokensIsAFailure(t *testing.T) {
	n := &fakeDeviceNotifier{}

	if err := SendToDevices(context.Background(), n, nil, "Hello", "World", nil); err == nil {
		t.Fatal("SendToDevices: want an error for zero tokens — nothing was delivered")
	}
	if len(n.calls) != 0 {
		t.Errorf("calls = %v, want none", n.calls)
	}
}

func TestSendToDevices_ForwardsDataPayload(t *testing.T) {
	n := &fakeDeviceNotifier{}
	var captured map[string]string
	wrapped := notifierFunc(func(ctx context.Context, token, title, body string, data map[string]string) error {
		captured = data
		return n.Send(ctx, token, title, body, data)
	})

	data := map[string]string{"slug": "acme-backend-engineer"}
	if err := SendToDevices(context.Background(), wrapped, []string{"tok-1"}, "Hello", "World", data); err != nil {
		t.Fatalf("SendToDevices: %v", err)
	}
	if captured["slug"] != "acme-backend-engineer" {
		t.Errorf("data forwarded = %v, want slug=acme-backend-engineer", captured)
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

// A PruneDeadPushToken failure partway through a batch must not discard the
// cleanup already done for tickets processed earlier in the same run: those
// still get deleted from the outbox, and only the ticket whose prune call
// failed stays queued for retry on the next scheduled pass.
func TestExpoNotifier_CheckReceipts_PartialPruneFailureKeepsEarlierProgress(t *testing.T) {
	srv := stubExpoReceipts(t, map[string]map[string]any{
		"ticket-ok-first":  {"status": "error", "message": "not delivered", "details": map[string]any{"error": "DeviceNotRegistered"}},
		"ticket-prune-err": {"status": "error", "message": "not delivered", "details": map[string]any{"error": "DeviceNotRegistered"}},
	})
	pruneErr := errors.New("transient db error")
	pruner := &fakePruner{errFor: map[string]error{"ExponentPushToken[prune-err]": pruneErr}}
	store := &fakeTicketStore{due: []Ticket{
		{ID: 1, Token: "ExponentPushToken[ok-first]", TicketID: "ticket-ok-first"},
		{ID: 2, Token: "ExponentPushToken[prune-err]", TicketID: "ticket-prune-err"},
	}}
	n := newTestNotifier(pruner, &fakeQueuer{}, store)
	n.receiptsURL = srv.URL

	err := n.CheckReceipts(context.Background())
	if err == nil {
		t.Fatal("CheckReceipts: want an error surfaced from the failed prune")
	}
	if len(pruner.pruned) != 1 || pruner.pruned[0] != "ExponentPushToken[ok-first]" {
		t.Errorf("pruned = %v, want [ExponentPushToken[ok-first]]", pruner.pruned)
	}
	if len(store.deleted) != 1 || store.deleted[0] != 1 {
		t.Errorf("deleted ticket ids = %v, want [1] — the ticket whose prune failed stays queued for retry", store.deleted)
	}
}
