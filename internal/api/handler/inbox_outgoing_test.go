package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/application/inbox"
)

func TestOutgoingMessageReadersKeepConversationMetadata(t *testing.T) {
	m := inbox.Message{ID: 7, Source: "gmail", Direction: "outgoing", ThreadID: "thread", ToAddrs: "hr@example.test", CcAddrs: "team@example.test"}
	body := bodyOf(m)
	if body.Direction.Value != m.Direction || body.ThreadID.Value != m.ThreadID || body.ToAddrs.Value != m.ToAddrs || body.CcAddrs.Value != m.CcAddrs {
		t.Fatalf("HTTP reader lost outgoing metadata: %+v", body)
	}
	agent := mailMessageView(m, false)
	if agent.Direction != m.Direction || agent.ThreadID != m.ThreadID || agent.ToAddrs != m.ToAddrs || agent.CcAddrs != m.CcAddrs {
		t.Fatalf("assistant reader lost outgoing metadata: %+v", agent)
	}
}
