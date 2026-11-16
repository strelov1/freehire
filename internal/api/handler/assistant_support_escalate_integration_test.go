//go:build integration

package handler

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestEscalateSendsOneMailPerSessionUnderConcurrentCalls(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	sender := &recordingSender{}
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), sender)
	userID, _ := assistantUser(t, pool, iss, "escalate-concurrent@example.test", false)
	sessionID := seedSupportSession(t, h, userID)

	var wg sync.WaitGroup
	var handedOver atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := h.escalate(context.Background(), userID, sessionID, escalationInput("charged twice"))
			if err != nil {
				t.Errorf("escalate: %v", err)
				return
			}
			if res.Status == "handed_over" {
				handedOver.Add(1)
			}
		}()
	}
	wg.Wait()

	if handedOver.Load() != 1 || len(sender.messages()) != 1 {
		t.Errorf("handed over %d times, %d mails sent; want exactly one of each", handedOver.Load(), len(sender.messages()))
	}
}

func TestEscalateRetriesAfterAFailedSend(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	sender := &recordingSender{}
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), sender)
	userID, _ := assistantUser(t, pool, iss, "escalate-retry@example.test", false)
	sessionID := seedSupportSession(t, h, userID)

	sender.failNext.Store(true)
	res, err := h.escalate(context.Background(), userID, sessionID, escalationInput("refund please"))
	if err != nil || res.Status != "unavailable" {
		t.Fatalf("failed send = %+v, %v; want status unavailable", res, err)
	}
	if len(sender.messages()) != 0 {
		t.Fatal("a mail was recorded for a failed send")
	}
	res, err = h.escalate(context.Background(), userID, sessionID, escalationInput("refund please"))
	if err != nil || res.Status != "handed_over" {
		t.Fatalf("retry = %+v, %v; want handed_over", res, err)
	}
	if len(sender.messages()) != 1 {
		t.Errorf("%d mails after the retry, want 1", len(sender.messages()))
	}
}

func TestEscalateRefusesTheFourthInADay(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	sender := &recordingSender{}
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), sender)
	userID, _ := assistantUser(t, pool, iss, "escalate-cap@example.test", false)

	var last escalateResult
	var lastSession uuid.UUID
	for i := 0; i < supportEscalationsPerDay+1; i++ {
		lastSession = seedSupportSession(t, h, userID)
		res, err := h.escalate(context.Background(), userID, lastSession, escalationInput("again"))
		if err != nil {
			t.Fatalf("escalate %d: %v", i+1, err)
		}
		last = res
	}
	if last.Status != "limit_reached" {
		t.Errorf("fourth handover = %q, want limit_reached", last.Status)
	}
	if n := len(sender.messages()); n != supportEscalationsPerDay {
		t.Errorf("%d mails sent, want %d", n, supportEscalationsPerDay)
	}
	var stamped bool
	if err := pool.QueryRow(context.Background(),
		`SELECT escalated_at IS NOT NULL FROM assistant_sessions WHERE id = $1`, lastSession).Scan(&stamped); err != nil {
		t.Fatalf("read claim: %v", err)
	}
	if stamped {
		t.Error("the refused handover kept its claim")
	}
}

func TestEscalateCannotClaimAnotherAccountsSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	sender := &recordingSender{}
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), sender)
	owner, _ := assistantUser(t, pool, iss, "escalate-owner@example.test", false)
	intruder, _ := assistantUser(t, pool, iss, "escalate-intruder@example.test", false)
	sessionID := seedSupportSession(t, h, owner)

	res, err := h.escalate(context.Background(), intruder, sessionID, escalationInput("not mine"))
	if err != nil || res.Status == "handed_over" {
		t.Fatalf("a foreign session was handed over: %+v, %v", res, err)
	}
	if len(sender.messages()) != 0 {
		t.Error("a mail left for a session the caller does not own")
	}
}

func TestEscalateSummaryIsInertAndReplyToNeedsAVerifiedAddress(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	sender := &recordingSender{}
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), sender)
	unverified, _ := assistantUser(t, pool, iss, "escalate-unverified@example.test", false)
	verified, _ := assistantUser(t, pool, iss, "escalate-verified@example.test", false)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET email_verified = true WHERE id = $1`, verified); err != nil {
		t.Fatalf("verify: %v", err)
	}

	hostile := "pay me\r\nBcc: attacker@example.test\r\n\r\n<script>alert(1)</script><b>bold</b>"
	for _, id := range []int64{unverified, verified} {
		res, err := h.escalate(context.Background(), id, seedSupportSession(t, h, id), escalationInput(hostile))
		if err != nil || res.Status != "handed_over" {
			t.Fatalf("escalate for %d = %+v, %v", id, res, err)
		}
	}

	msgs := sender.messages()
	if len(msgs) != 2 {
		t.Fatalf("%d mails, want 2", len(msgs))
	}
	for _, m := range msgs {
		if m.HTML != "" {
			t.Errorf("the handover has an HTML part, so the summary could render: %q", m.HTML)
		}
		for _, line := range strings.Split(m.Text, "\n") {
			if strings.HasPrefix(line, "Bcc:") {
				t.Errorf("an injected header line survived: %q", line)
			}
		}
		if strings.ContainsAny(m.Subject+m.ReplyTo+m.To+m.From, "\r\n") {
			t.Errorf("a header field carries CR or LF: %+v", m)
		}
	}
	if msgs[0].ReplyTo != "" {
		t.Errorf("Reply-To for an unverified account = %q, want none", msgs[0].ReplyTo)
	}
	if msgs[1].ReplyTo != "escalate-verified@example.test" {
		t.Errorf("Reply-To for a verified account = %q", msgs[1].ReplyTo)
	}
}
