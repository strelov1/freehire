package handler

import (
	"os"
	"strings"
	"testing"
)

// The sweep must spend on the CALLER's gateway credential, not the service's, or its cost
// lands in the wrong row of the spend report and the account that ran it looks free.
//
// A source assertion rather than a behavioural one because of how this fails. userLLM's own
// doc names it: "a call site that forgets to attribute keeps working perfectly and simply
// spends anonymously, which is the failure nobody notices". Nothing observable changes —
// not a status, not a body, not a log line — so there is no behaviour left to assert on.
// What there is, is a single identifier to grep for, which is what this does.
func TestMailRecallSpendsAsTheCaller(t *testing.T) {
	src, err := os.ReadFile("mail_recall.go")
	if err != nil {
		t.Fatalf("read handler: %v", err)
	}
	for _, want := range []string{"h.llm.bind(", "tagMailRecall", "h.recall.As("} {
		if !strings.Contains(string(src), want) {
			t.Errorf("mail_recall.go no longer names %q. Without it the run goes out on the "+
				"SERVICE credential: every call still succeeds and the spend is filed against "+
				"nobody, which is why this is guarded here rather than by a status code.", want)
		}
	}
}
