package webhooknotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/changehook"
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

	if err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, msgOf(d)); err != nil {
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
	err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, msgOf(notify.Digest{}))
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
	err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, msgOf(notify.Digest{}))
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
	if err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, msgOf(notify.Digest{})); err != nil {
		t.Errorf("Send returned error for a 202: %v", err)
	}
}

func TestSend_RejectsNonHTTPScheme(t *testing.T) {
	n := newNotifier(http.DefaultClient)
	if err := n.Send(context.Background(), notify.ChannelWebhook, "ftp://example.com/hook", msgOf(notify.Digest{})); err == nil {
		t.Error("Send: want an error for a non-http(s) URL, got none")
	}
}

func TestSend_InvalidDestIsError(t *testing.T) {
	n := newNotifier(http.DefaultClient)
	if err := n.Send(context.Background(), notify.ChannelWebhook, "://not a url", msgOf(notify.Digest{})); err == nil {
		t.Error("Send: want an error for a malformed dest, got none")
	}
}

// NewNotifier's production client is SSRF-guarded (internal/platform/safehttp):
// pointed at a loopback address, the send must be refused rather than delivered.
func TestSend_ProductionClientRejectsPrivateAddress(t *testing.T) {
	n := NewNotifier()
	if err := n.Send(context.Background(), notify.ChannelWebhook, "http://127.0.0.1:1/hook", msgOf(notify.Digest{})); err == nil {
		t.Error("Send: want an error when the production client targets a loopback address")
	}
}

// Test sends a recognisable sample so the receiver can tell a probe from a
// digest, with an empty (not null) job list a template can still range over.
func TestTest_PostsSampleAndReportsStatus(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := newNotifier(srv.Client()).Test(context.Background(), srv.URL, testSecret); err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	if got["saved_search_name"] != TestSearchName {
		t.Errorf("saved_search_name = %v, want %q", got["saved_search_name"], TestSearchName)
	}
	if jobs, ok := got["jobs"].([]any); !ok || len(jobs) != 0 {
		t.Errorf("jobs = %#v, want an empty array", got["jobs"])
	}
}

func TestTest_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := newNotifier(srv.Client()).Test(context.Background(), srv.URL, testSecret)
	if err == nil {
		t.Fatal("Test: want an error for a 404")
	}
}

// msgOf wraps one digest as the single-digest message every case here sends.
func msgOf(d notify.Digest) notify.Message {
	return notify.Message{WebhookSecret: testSecret, Digests: []notify.Digest{d}}
}

const testSecret = "whsec_test"

// A receiver verifies a digest with the same check change webhooks document
// (changehook.Verify), and tells a probe from a digest by X-Freehire-Event.
func TestSend_SignsTheBody(t *testing.T) {
	var got []*http.Request
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got, bodies = append(got, r), append(bodies, b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	n := newNotifier(srv.Client())

	if err := n.Send(context.Background(), notify.ChannelWebhook, srv.URL, msgOf(notify.Digest{SavedSearchName: "Go"})); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := n.Test(context.Background(), srv.URL, testSecret); err != nil {
		t.Fatalf("Test: %v", err)
	}
	for i, want := range []string{EventDigest, EventTest} {
		r := got[i]
		if ev := r.Header.Get(changehook.HeaderEvent); ev != want {
			t.Errorf("request %d: event = %q, want %q", i, ev, want)
		}
		err := changehook.Verify(testSecret, r.Header.Get(changehook.HeaderTimestamp), r.Header.Get(changehook.HeaderSignature),
			bodies[i], time.Now(), changehook.DefaultTolerance)
		if err != nil {
			t.Errorf("request %d: Verify: %v", i, err)
		}
		if changehook.Verify("whsec_other", r.Header.Get(changehook.HeaderTimestamp), r.Header.Get(changehook.HeaderSignature),
			bodies[i], time.Now(), changehook.DefaultTolerance) == nil {
			t.Errorf("request %d: verified with the wrong secret", i)
		}
	}
}

func TestSend_RefusesWithoutSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("an unsigned delivery reached the destination")
	}))
	defer srv.Close()

	m := msgOf(notify.Digest{})
	m.WebhookSecret = ""
	if err := newNotifier(srv.Client()).Send(context.Background(), notify.ChannelWebhook, srv.URL, m); err == nil {
		t.Fatal("Send without a secret: want an error")
	}
}
