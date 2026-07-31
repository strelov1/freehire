// This file is package appevent_test on purpose: internal/inbox records events and so
// imports appevent, and an in-package test importing it back would be an import cycle.
package appevent_test

import (
	"testing"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/inbox"
)

// SourceForMail and inbox.Sources are pinned to each other in both directions, in the
// manner mailclassify pins its prompt to validSignals.
//
// Left unpinned, adding a fourth mail store to inbox.Sources would compile, ship, and
// then fail at the moment its first message was linked — a runtime error on a path whose
// whole purpose is to record something that just happened and cannot be replayed.
func TestEveryInboxMailSourceHasAnEventSource(t *testing.T) {
	for _, s := range inbox.Sources {
		got, err := appevent.SourceForMail(s)
		if err != nil {
			t.Errorf("inbox source %q has no event source: %v", s, err)
			continue
		}
		if !appevent.TrustedForDayMath(got) {
			t.Errorf("inbox source %q maps to %q, which is not trusted for day math — mail timestamps are the only ones we treat as observed", s, got)
		}
	}
}

func TestNoEventMailSourceIsOrphaned(t *testing.T) {
	mapped := map[string]bool{}
	for _, s := range inbox.Sources {
		if got, err := appevent.SourceForMail(s); err == nil {
			mapped[got] = true
		}
	}
	for _, s := range appevent.Sources {
		if !appevent.TrustedForDayMath(s) {
			continue // the manual sources have no inbox counterpart by design
		}
		if !mapped[s] {
			t.Errorf("event source %q is trusted for day math but no inbox source maps to it — it can never be written", s)
		}
	}
}
