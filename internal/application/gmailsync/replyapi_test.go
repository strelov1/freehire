package gmailsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"testing"
)

func TestSendReplyUsesTheOriginalGmailThreadAndRFCMessageID(t *testing.T) {
	client := &http.Client{Transport: searchTransport(func(req *http.Request) (*http.Response, error) {
		var payload struct{ ThreadID, Raw string }
		if req.Method != http.MethodPost || req.URL.String() != gmailBaseURL+"/messages/send" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.ThreadID != "thread-1" {
			t.Errorf("unexpected Gmail payload: %+v, %v", payload, err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(payload.Raw)
		if err != nil {
			t.Fatal(err)
		}
		msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		for header, want := range map[string]string{
			"To": "<recruiter@example.test>", "From": "<candidate@example.test>", "Subject": "Re: Interview",
			"In-Reply-To": "<parent@example.test>", "References": "<parent@example.test>",
			"Message-ID": "<attempt@freehire.me>",
		} {
			if got := msg.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
		body, err := io.ReadAll(msg.Body)
		if err != nil || string(body) != "Tuesday at 10:00 works.\r\n" {
			t.Errorf("body = %q, err = %v", body, err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"sent-1","threadId":"thread-1"}`))}, nil
	})}
	input := Reply{
		ThreadID: "thread-1", Sender: "candidate@example.test", Recipient: "recruiter@example.test",
		Subject: "Interview", ParentMessageID: "<parent@example.test>", MessageID: "<attempt@freehire.me>",
		Body: "Tuesday at 10:00 works.",
	}
	if id, err := NewReplyClient(client).Send(context.Background(), input); err != nil || id != "sent-1" {
		t.Fatalf("send reply: id = %q, err = %v", id, err)
	}
	input.Subject = "Interview\r\nBcc: other@example.test"
	if _, err := NewReplyClient(client).Send(context.Background(), input); err == nil {
		t.Fatal("a newline in a mail header must stop delivery")
	}
}

func TestReplyThreadDistinguishesHumanMessagesFromAutomatedMail(t *testing.T) {
	client := &http.Client{Transport: searchTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/gmail/v1/users/me/threads/thread-1" ||
			req.URL.Query().Get("format") != "metadata" {
			t.Errorf("unexpected thread request: %s %s", req.Method, req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
			`{"id":"thread-1","messages":[
				{"id":"recruiter","payload":{"headers":[{"name":"Message-ID","value":"<recruiter@example.test>"},
					{"name":"From","value":"recruiter@example.test"},{"name":"Reply-To","value":"reply@example.test"},
					{"name":"Subject","value":"Interview"},{"name":"Auto-Submitted","value":"no"}]}},
				{"id":"candidate","labelIds":["SENT"]},
				{"id":"draft","labelIds":["DRAFT"]},{"id":"removed","labelIds":["TRASH"]},
				{"id":"spam","labelIds":["SPAM"]},
				{"id":"auto","payload":{"headers":[{"name":"Auto-Submitted","value":"auto-replied"}]}},
				{"id":"list","payload":{"headers":[{"name":"List-Id","value":"news.example.test"}]}},
				{"id":"bulk","payload":{"headers":[{"name":"Precedence","value":"Bulk"}]}}
			]}`))}, nil
	})}
	got, err := NewReplyClient(client).Thread(context.Background(), "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []ThreadMessage{
		{ID: "recruiter", MessageID: "<recruiter@example.test>", From: "recruiter@example.test", ReplyTo: "reply@example.test", Subject: "Interview"},
		{ID: "candidate", Sent: true},
		{ID: "spam", Automated: true},
		{ID: "auto", Automated: true},
		{ID: "list", Automated: true},
		{ID: "bulk", Automated: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("thread messages = %+v, want %+v", got, want)
	}
}
