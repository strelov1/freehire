package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Every way this pass can end must report the resume point it already holds. The scatter
// of `return 1` sites is what made that untrue twice in one evening on production: once
// through the pass's own error, and once through the companies reconcile that runs after
// it, which cancelled and returned before the id was ever printed (freehire#2876).
func TestRunOutcome_ReportsTheResumePointOnEveryPath(t *testing.T) {
	cancelled := context.Canceled
	boom := errors.New("sync companies: context canceled")

	for _, tc := range []struct {
		name     string
		pass     backfillRun
		orphaned int64
		err      error
		wantID   bool // the message must carry the id to continue from
		wantDone bool
		wantExit int
	}{
		{
			name:     "a full pass reports done",
			pass:     backfillRun{Scanned: 10, Updated: 2},
			wantDone: true,
			wantExit: 0,
		},
		{
			name:     "a budget stop reports the id and succeeds",
			pass:     backfillRun{Scanned: 10, Updated: 2, ResumeID: 77},
			wantID:   true,
			wantExit: 0,
		},
		{
			name:     "a cancelled pass reports the id and fails",
			pass:     backfillRun{Scanned: 10, Updated: 2, ResumeID: 77},
			err:      cancelled,
			wantID:   true,
			wantExit: 1,
		},
		{
			// The path that actually bit: the pass returned cleanly, the companies
			// reconcile after it did not, and the run ended with the id in hand.
			name:     "a failed companies reconcile still reports the id",
			pass:     backfillRun{Scanned: 10, Updated: 2, SlugsMoved: 1, ResumeID: 77},
			err:      boom,
			wantID:   true,
			wantExit: 1,
		},
		{
			// Nothing ran, so there is nothing to resume and nothing to call done.
			name:     "a failure before any row must not claim done",
			pass:     backfillRun{},
			err:      errors.New("load company slug aliases: dial tcp: refused"),
			wantExit: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, exit := runOutcome(tc.pass, tc.orphaned, tc.err)
			if exit != tc.wantExit {
				t.Errorf("exit=%d, want %d", exit, tc.wantExit)
			}
			hasID := strings.Contains(msg, "BACKFILL_DERIVE_FROM_ID=77")
			if hasID != tc.wantID {
				t.Errorf("message carries the resume id = %v, want %v\nmessage: %s", hasID, tc.wantID, msg)
			}
			hasDone := strings.Contains(msg, "done")
			if hasDone != tc.wantDone {
				t.Errorf("message claims done = %v, want %v\nmessage: %s", hasDone, tc.wantDone, msg)
			}
		})
	}
}
