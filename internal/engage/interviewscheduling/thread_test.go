package interviewscheduling

import (
	"testing"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestGmailThreadDetectsCandidateInterventionBeforeRecruiterReply(t *testing.T) {
	thread := []gmailsync.ThreadMessage{
		{ID: "application", Sent: true},
		{ID: "invite"},
		{ID: "offer", Sent: true},
		{ID: "withdraw-offer", Sent: true},
		{ID: "recruiter-acceptance"},
	}
	row := db.ClaimInterviewSchedulingThreadRow{AnchorProviderID: "invite", LastSentID: "offer"}
	if !automationInterrupted(thread, row) {
		t.Fatal("candidate withdrew the agent's offer before the recruiter's acceptance")
	}
	if automationInterrupted(thread[:3], row) {
		t.Fatal("the agent's own offer must not stop negotiation")
	}
	row.LastSentID = ""
	if automationInterrupted(thread[:2], row) {
		t.Fatal("the original application predates the recruiter invitation")
	}
}
