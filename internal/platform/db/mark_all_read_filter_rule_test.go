package db

import (
	"reflect"
	"sort"
	"testing"
)

// listingOnlyEmailParams are the fields ListEmails has because it PAGES and RENDERS,
// which a bulk update has no use for. Everything else on that struct narrows which
// messages are in scope, and is therefore a filter MarkAllEmailsRead must carry too.
var listingOnlyEmailParams = map[string]bool{
	"WithBody": true, // whether to select the body — a projection, not a filter
	"Unread":   true, // the bulk update is unconditionally `read_at IS NULL`
	"Off":      true, // paging
	"Lim":      true, // paging
}

// TestMarkAllEmailsReadTakesEveryListingFilter pins the one thing a comment cannot.
//
// "Mark all read" means "everything currently shown", so its scope has to be the
// listing's scope. The two statements are deliberately separate — the mark-as-read
// methods stay outside inbox.Queries (docs/agents/mail-stack.md), so the predicates are
// hand-maintained copies of each other and free to drift.
//
// They drifted once already, and the shape of the failure is why this test exists: the
// query carried four of the listing's six filters while the handler parsed and validated
// all six, so `read-all?unclassified=1` — the triage queue's own button — emptied the
// whole unread mailbox instead of the page in front of the person pressing it. Nothing
// refused the extra arguments, nothing logged, and there is no undo.
//
// A filter that is simply ABSENT cannot be caught by a behavioural test nobody thought to
// write for it, so this compares the two generated parameter sets directly: add a seventh
// filter to ListEmails and this fails until MarkAllEmailsRead has it too.
func TestMarkAllEmailsReadTakesEveryListingFilter(t *testing.T) {
	fields := func(v any) map[string]bool {
		out := map[string]bool{}
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			out[typ.Field(i).Name] = true
		}
		return out
	}

	listing, bulk := fields(ListEmailsParams{}), fields(MarkAllEmailsReadParams{})

	var missing, extra []string
	for name := range listing {
		if !listingOnlyEmailParams[name] && !bulk[name] {
			missing = append(missing, name)
		}
	}
	for name := range bulk {
		if !listing[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("MarkAllEmailsRead does not narrow by %v, which ListEmails does — "+
			"the button would mark messages the page never showed. Add the predicate to "+
			"queries/gmail.sql and pass it from internal/api/handler/inbox.go, or name the "+
			"field in listingOnlyEmailParams with the reason it is not a filter", missing)
	}
	if len(extra) > 0 {
		t.Errorf("MarkAllEmailsRead narrows by %v, which ListEmails does not — the button "+
			"would leave messages unread that the page did show", extra)
	}
}
