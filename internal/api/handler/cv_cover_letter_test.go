package handler

import (
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
)

func bandCtx(t *testing.T, query string) *fiber.Ctx {
	t.Helper()
	app := fiber.New()
	fctx := &fasthttp.RequestCtx{}
	fctx.Request.SetRequestURI("/me/cvs/1/cover-letter?" + query)
	return app.AcquireCtx(fctx)
}

func TestCoverLetterBandReadsTheShortRequest(t *testing.T) {
	if got := coverLetterBand(bandCtx(t, "band=short")); got != coverletter.BandShort {
		t.Errorf("band = %q, want short", got)
	}
}

// The bands are a product decision rather than a measured limit, so a typo asking for brevity
// is better answered with a normal letter than with a refusal.
func TestCoverLetterBandFallsBackToStandard(t *testing.T) {
	for _, q := range []string{"", "band=", "band=tiny", "band=SHORT"} {
		if got := coverLetterBand(bandCtx(t, q)); got != coverletter.BandStandard {
			t.Errorf("query %q: band = %q, want standard", q, got)
		}
	}
}

// The ledger's uniqueness index is on (user_id, feature, ref) for a consume, so this string is
// the whole of what makes a retry idempotent and a redraft chargeable.
func TestCoverLetterRefSeparatesJobsAndAttempts(t *testing.T) {
	first := coverLetterRef(7, "first")
	sameAgain := coverLetterRef(7, "first")
	redraft := coverLetterRef(7, "2026-09-01T10:00:00Z")
	otherJob := coverLetterRef(8, "first")

	if first != sameAgain {
		t.Error("the same attempt must compute the same reference, or a retry charges twice")
	}
	if first == redraft {
		t.Error("a redraft must compute a new reference, or the second set of model calls is free")
	}
	if first == otherJob {
		t.Error("two vacancies must not share a reference")
	}
	if !strings.HasPrefix(first, "cover-letter#") {
		t.Errorf("ref = %q, want a feature-prefixed reference", first)
	}
}

// An unconfigured deployment must not be reported as a letter written by a nameless model:
// Stale compares this against the stored stamp, and "" == "" reads as "matches".
func TestModelIDOfIsEmptyWithoutAGateway(t *testing.T) {
	if got := modelIDOf(nil); got != "" {
		t.Errorf("model = %q, want empty when no gateway is configured", got)
	}
}

// A drafter missing any dependency must refuse in its caller's own vocabulary rather than
// panic inside it - on the tool's path that panic lands in a detached goroutine where no
// error path is listening.
func TestLetterDrafterIsNotReadyWhenAnythingIsMissing(t *testing.T) {
	if (letterDrafter{}).ready() {
		t.Error("an empty drafter reports ready")
	}
	if (&cvHandlers{}).letterDrafter().ready() {
		t.Error("an unwired cvHandlers reports a ready drafter")
	}
	if (&assistantHandlers{}).letterDrafter().ready() {
		t.Error("an unwired assistantHandlers reports a ready drafter")
	}
}
