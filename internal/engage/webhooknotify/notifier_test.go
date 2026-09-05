package webhooknotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/engage/notify"
)

func TestSend_PostsJSONBody(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The server's own client, not safehttp's: safehttp refuses private
	// addresses, which is exactly right in production and exactly wrong for a
	// loopback test server (see internal/identity/billing's client for the
	// same seam).
	n := newNotifier(srv.Client())
	d := notify.Digest{SavedSearchName: "Go jobs", Total: 1, Jobs: []notify.DigestJob{{Title: "Gopher", Slug: "gopher"}}}

	if err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, d); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	var got struct {
		SavedSearchName string `json:"saved_search_name"`
		Total           int    `json:"total"`
		Jobs            []struct{ Title string }
	}
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("body is not valid JSON: %v (%q)", err, gotBody)
	}
	if got.SavedSearchName != "Go jobs" || got.Total != 1 || len(got.Jobs) != 1 || got.Jobs[0].Title != "Gopher" {
		t.Errorf("decoded body = %+v, want the digest's contents", got)
	}
}

func TestSend_410IsRecipientGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	n := newNotifier(srv.Client())
	err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, notify.Digest{})
	if !errors.Is(err, notify.ErrRecipientGone) {
		t.Errorf("Send error = %v, want wrapping notify.ErrRecipientGone", err)
	}
}

func TestSend_ServerErrorIsPlainError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := newNotifier(srv.Client())
	err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, notify.Digest{})
	if err == nil {
		t.Fatal("Send: want an error for a 500 response")
	}
	if errors.Is(err, notify.ErrRecipientGone) {
		t.Error("Send: a 500 must follow the normal retry/dead-letter path, not ErrRecipientGone")
	}
}

func TestSend_SuccessOnAny2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	n := newNotifier(srv.Client())
	if err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, notify.Digest{}); err != nil {
		t.Errorf("Send returned error for a 202: %v", err)
	}
}

func TestSend_RejectsNonHTTPScheme(t *testing.T) {
	n := newNotifier(http.DefaultClient)
	if err := n.Send(context.Background(), notify.ChannelWebhook, "ftp://example.com/hook", notify.Digest{}); err == nil {
		t.Error("Send: want an error for a non-http(s) URL, got none")
	}
}

func TestSend_InvalidDestIsError(t *testing.T) {
	n := newNotifier(http.DefaultClient)
	if err := n.Send(context.Background(), notify.ChannelWebhook, "://not a url", notify.Digest{}); err == nil {
		t.Error("Send: want an error for a malformed dest, got none")
	}
}

// NewNotifier's production client is SSRF-guarded (internal/platform/safehttp):
// pointed at a loopback address, the send must be refused rather than delivered.
func TestSend_ProductionClientRejectsPrivateAddress(t *testing.T) {
	n := NewNotifier()
	if err := n.Send(context.Background(), notify.ChannelWebhook, "http://127.0.0.1:1/hook", notify.Digest{}); err == nil {
		t.Error("Send: want an error when the production client targets a loopback address")
	}
}
