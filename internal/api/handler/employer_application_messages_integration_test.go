//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/application/mailingest"
	"github.com/strelov1/freehire/internal/platform/db"
)

type candidateMessageTransport struct {
	calls   int
	fail    bool
	to      string
	replyTo string
	body    string
}

func (m *candidateMessageTransport) SendCandidateMessage(_ context.Context, to, replyTo, _, _, body string) error {
	m.calls++
	m.to, m.replyTo, m.body = to, replyTo, body
	if m.fail {
		return errors.New("test transport unavailable")
	}
	return nil
}

type candidateReplySource []mailingest.Inbound

func (s candidateReplySource) Receive(context.Context) ([]mailingest.Inbound, error) {
	return s, nil
}

func (candidateReplySource) Ack(context.Context, string) error { return nil }

func testEmployerApplicationMessages(t *testing.T, ctx context.Context, queries *db.Queries, candidateMail *candidateMessageTransport, idPath, employerCookie, otherCookie string, request func(*testing.T, string, string, string, string) (int, []byte)) {
	t.Helper()

	t.Run("only the hiring team can open the application conversation", func(t *testing.T) {
		path := idPath + "/messages"
		resp, body := request(t, fiber.MethodGet, path, employerCookie, "")
		if resp != fiber.StatusOK || !strings.Contains(string(body), `"messages":[]`) {
			t.Fatalf("team conversation = %d %s, want an empty conversation", resp, body)
		}
		resp, body = request(t, fiber.MethodGet, path, otherCookie, "")
		if resp != fiber.StatusNotFound {
			t.Fatalf("other company conversation = %d %s, want 404", resp, body)
		}
	})

	t.Run("the team can retry email and read authenticated replies without moving the application", func(t *testing.T) {
		path := idPath + "/messages"
		const messagePayload = `{"body":"Are you available next Tuesday?","request_key":"00000000-0000-4000-8000-000000000001"}`
		const changed = `{"body":"Different contents","request_key":"00000000-0000-4000-8000-000000000001"}`
		type message struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
			Body   string `json:"body"`
		}
		send := func(payload string, want int) message {
			t.Helper()
			resp, body := request(t, fiber.MethodPost, path, employerCookie, payload)
			if resp != want {
				t.Fatalf("send = %d %s, want %d", resp, body, want)
			}
			var envelope struct{ Data message }
			if want == fiber.StatusOK {
				if err := json.Unmarshal(body, &envelope); err != nil {
					t.Fatal(err)
				}
			}
			return envelope.Data
		}
		candidateMail.fail = true
		failed := send(messagePayload, fiber.StatusOK)
		if failed.Status != "failed" || failed.ID == 0 || candidateMail.calls != 1 {
			t.Fatalf("failed attempt = %+v, calls = %d", failed, candidateMail.calls)
		}
		candidateMail.fail = false
		sent := send(messagePayload, fiber.StatusOK)
		replyTo := candidateMail.replyTo
		if sent.ID != failed.ID || sent.Status != "sent" || candidateMail.to != "c@example.test" ||
			candidateMail.body != "Are you available next Tuesday?" || !strings.HasSuffix(replyTo, "@mail.example.test") {
			t.Fatalf("retried attempt = %+v, transport = %+v", sent, candidateMail)
		}
		if again := send(messagePayload, fiber.StatusOK); again.ID != sent.ID || candidateMail.calls != 2 {
			t.Fatalf("replayed send = %+v, calls = %d", again, candidateMail.calls)
		}
		send(changed, fiber.StatusConflict)

		makeReply := func(sender, externalID, text, envelopeRecipient string, authenticated bool) mailingest.Inbound {
			return mailingest.Inbound{
				Recipients: []string{envelopeRecipient}, AuthenticatedReply: authenticated,
				Raw:   []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nMessage-ID: <%s@example.test>\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", sender, replyTo, externalID, text)),
				S3Key: "test/" + externalID,
			}
		}
		first := makeReply("c@example.test", "first", "Yes, I am available.", replyTo, true)
		second := makeReply("c@example.test", "second", "My time zone is UTC.", replyTo, true)
		source := candidateReplySource{
			makeReply("c@example.test", "spoof", "Spoofed From.", replyTo, false),
			makeReply("stranger@example.test", "stranger", "Wrong candidate.", replyTo, true),
			makeReply("c@example.test", "forged-to", "Wrong envelope.", "nobody@mail.example.test", true),
			first, first, second,
		}
		worker := mailingest.NewWorker(source, mailingest.NewDBStore(queries), "mail.example.test")
		if err := worker.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}

		resp, raw := request(t, fiber.MethodGet, path, employerCookie, "")
		var history struct {
			Data struct{ Messages []message }
		}
		if resp != fiber.StatusOK || json.Unmarshal(raw, &history) != nil {
			t.Fatalf("history = %d %s", resp, raw)
		}
		msgs := history.Data.Messages
		if len(msgs) != 3 || msgs[0].Body != "My time zone is UTC." || msgs[1].Body != "Yes, I am available." || msgs[2].ID != sent.ID {
			t.Fatalf("history contains an incorrect or duplicate reply: %s", raw)
		}

		read := func(through int64, wantUnread bool) {
			t.Helper()
			resp, body := request(t, fiber.MethodPost, path+"/read", employerCookie, fmt.Sprintf(`{"through_id":%d}`, through))
			want := fmt.Sprintf(`"has_unread_reply":%t`, wantUnread)
			if resp != fiber.StatusOK || !strings.Contains(string(body), want) {
				t.Fatalf("read through %d = %d %s, want %s", through, resp, body, want)
			}
		}
		list := func(wantUnread bool) {
			t.Helper()
			res, body := request(t, fiber.MethodGet, "/api/v1/employer/applications?job=acme-platform", employerCookie, "")
			if res != fiber.StatusOK || !strings.Contains(string(body), fmt.Sprintf(`"has_unread_reply":%t`, wantUnread)) ||
				!strings.Contains(string(body), `"stage":"new"`) {
				t.Fatalf("list after messaging = %d %s", res, body)
			}
		}
		list(true)
		read(msgs[1].ID, true)
		read(msgs[0].ID, false)
		list(false)
	})

}
