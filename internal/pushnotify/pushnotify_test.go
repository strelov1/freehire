package pushnotify

import (
	"context"
	"encoding/json"
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

func TestExpoNotifier_Send_Success(t *testing.T) {
	srv := stubExpo(t, "ok", "")
	pruner := &fakePruner{}
	n := NewExpoNotifier(pruner)
	n.apiURL = srv.URL

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none on success", pruner.pruned)
	}
}

func TestExpoNotifier_Send_DeviceNotRegisteredPrunesToken(t *testing.T) {
	srv := stubExpo(t, "error", "DeviceNotRegistered")
	pruner := &fakePruner{}
	n := NewExpoNotifier(pruner)
	n.apiURL = srv.URL

	if err := n.Send(context.Background(), "ExponentPushToken[dead]", "Hello", "World"); err != nil {
		t.Fatalf("Send: %v, want nil — a dead token is pruned, not a caller-facing error", err)
	}
	if len(pruner.pruned) != 1 || pruner.pruned[0] != "ExponentPushToken[dead]" {
		t.Errorf("pruned = %v, want [ExponentPushToken[dead]]", pruner.pruned)
	}
}

func TestExpoNotifier_Send_OtherErrorSurfacesWithoutPruning(t *testing.T) {
	srv := stubExpo(t, "error", "MessageTooBig")
	pruner := &fakePruner{}
	n := NewExpoNotifier(pruner)
	n.apiURL = srv.URL

	if err := n.Send(context.Background(), "ExponentPushToken[abc]", "Hello", "World"); err == nil {
		t.Fatal("Send: want error for a non-DeviceNotRegistered failure")
	}
	if len(pruner.pruned) != 0 {
		t.Errorf("pruned = %v, want none — a transient error must not delete the token", pruner.pruned)
	}
}
