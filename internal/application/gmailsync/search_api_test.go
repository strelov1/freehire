package gmailsync

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSearchSkipsUnreadableMessage(t *testing.T) {
	transport := searchTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", req.Method)
		}
		status, body := http.StatusOK, ""
		switch req.URL.Path {
		case "/gmail/v1/users/me/messages":
			if q := req.URL.Query().Get("q"); q != `"Acme"` {
				t.Errorf("query = %q, want quoted Acme", q)
			}
			if limit := req.URL.Query().Get("maxResults"); limit != "10" {
				t.Errorf("maxResults = %q, want 10", limit)
			}
			body = `{"messages":[{"id":"bad"},{"id":"good"}]}`
		case "/gmail/v1/users/me/messages/bad":
			status, body = http.StatusInternalServerError, `{"error":{"code":500}}`
		case "/gmail/v1/users/me/messages/good":
			body = `{"id":"good","threadId":"thread-good","internalDate":"1700000000000","payload":{"mimeType":"text/plain","headers":[{"name":"Subject","value":"Acme interview"}],"body":{"data":"SGVsbG8"}}}`
		default:
			t.Fatalf("unexpected request: %s", req.URL)
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	reader := NewAPIReader(&http.Client{Transport: transport}, nil).(*apiReader)
	messages, err := reader.Search(context.Background(), `"Acme"`, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "good" {
		t.Fatalf("messages = %+v, want only good", messages)
	}
	if messages[0].Subject != "Acme interview" || messages[0].BodyText != "Hello" {
		t.Errorf("message = %+v, want the readable message's subject and body", messages[0])
	}
}

func TestSearchEmptyQueryDoesNotCallHTTP(t *testing.T) {
	transport := searchTransport(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("empty query made an HTTP request: %s", req.URL)
		return nil, nil
	})
	reader := NewAPIReader(&http.Client{Transport: transport}, nil).(*apiReader)
	messages, err := reader.Search(context.Background(), "", 10)
	if messages != nil || err != nil {
		t.Fatalf("Search = %+v, %v; want nil, nil", messages, err)
	}
}
