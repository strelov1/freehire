package atsapply

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Found by code review: verifySubmission's poll loop returned a plain error whenever
// chromedp.Run failed for ANY reason — including the parent context's own deadline firing
// while that one call was in flight, which is the exact same "ran out of time waiting for
// a marker" outcome the between-polls case (ctx.Done() in the select below) already treats
// as unconfirmed (false, nil), not an error. A plain error here takes the ordinary retry
// path in internal/autoapply's runner, risking a second real submit click on a form whose
// first click may already have gone through — precisely what StatusUnconfirmed exists to
// prevent.
func TestClassifyPollError_ADeadlineFiringMidCallIsUnconfirmedNotAnError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-ctx.Done() // the deadline has now fired, same as it firing while chromedp.Run was in flight

	if !classifyPollError(ctx, errors.New("context deadline exceeded")) {
		t.Error("want a context-deadline error classified as unconfirmed (not a real failure)")
	}
}

func TestClassifyPollError_ACancelledParentIsUnconfirmedNotAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !classifyPollError(ctx, errors.New("context canceled")) {
		t.Error("want a cancelled-context error classified as unconfirmed (not a real failure)")
	}
}

// An ordinary chromedp failure — the target element genuinely not found, say — with a
// still-live context must stay a real, reportable error.
func TestClassifyPollError_AnOrdinaryFailureOnALiveContextIsARealError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if classifyPollError(ctx, errors.New("no node found for selector")) {
		t.Error("want an ordinary failure on a live context classified as a real error, not unconfirmed")
	}
}
