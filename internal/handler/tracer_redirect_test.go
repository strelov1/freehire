package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeTracerLinks stands in for the three queries the redirect makes, and records what it was
// asked to write.
type fakeTracerLinks struct {
	row       db.TracerLinkByTokenRow
	lookupErr error
	recordErr error
	clicks    []db.RecordTracerClickParams
	touched   int
}

func (f *fakeTracerLinks) TracerLinkByToken(context.Context, string) (db.TracerLinkByTokenRow, error) {
	if f.lookupErr != nil {
		return db.TracerLinkByTokenRow{}, f.lookupErr
	}
	return f.row, nil
}

func (f *fakeTracerLinks) RecordTracerClick(_ context.Context, p db.RecordTracerClickParams) error {
	f.clicks = append(f.clicks, p)
	return f.recordErr
}

func (f *fakeTracerLinks) TouchCVLastClick(context.Context, pgtype.UUID) error {
	f.touched++
	return nil
}

func redirectApp(t *testing.T, links *fakeTracerLinks, signedInAs int64) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h := &tracerHandlers{links: links, salt: "pepper"}
	var gate = func(c *fiber.Ctx) error { return c.Next() }
	if signedInAs != 0 {
		gate = signedIn(signedInAs)
	}
	app.Get("/cv/:token", gate, h.Redirect)
	return app
}

// A real browser always sends a user agent; httptest does not. Every case standing in for a
// person must therefore say so, or it is testing the automated path by accident.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func humanGet(target string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	req.Header.Set("User-Agent", browserUA)
	return req
}

func linkRow() db.TracerLinkByTokenRow {
	return db.TracerLinkByTokenRow{
		ID:             pgtype.UUID{Bytes: uuid.New(), Valid: true},
		DestinationUrl: "https://github.com/ada",
		OwnerID:        7,
	}
}

func TestARedirectSendsTheVisitorOnAndCountsTheClick(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	resp, err := redirectApp(t, links, 0).Test(humanGet("/cv/acme-x7abc"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusFound)
	}
	if got := resp.Header.Get("Location"); got != "https://github.com/ada" {
		t.Errorf("Location = %q, want the stored destination", got)
	}
	if len(links.clicks) != 1 {
		t.Fatalf("recorded %d clicks, want 1", len(links.clicks))
	}
	if links.touched != 1 {
		t.Errorf("stamped the CV %d times, want 1", links.touched)
	}
}

// A token whose CV was deleted must explain itself. The recruiter holding that PDF did nothing
// wrong, and a bare 404 reads as a broken site rather than an expired link.
func TestAnUnknownTokenIsGone(t *testing.T) {
	links := &fakeTracerLinks{lookupErr: pgx.ErrNoRows}
	resp, err := redirectApp(t, links, 0).Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/cv/never-existed", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != fiber.StatusGone {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusGone)
	}
	if len(links.clicks) != 0 {
		t.Errorf("recorded a click against a token that does not exist")
	}
}

// The redirect is the contract; the count is a bonus. A broken redirect lives in a PDF the
// candidate can neither see nor fix, so a failed write must never cost the visitor their
// destination.
func TestAFailedClickWriteStillRedirects(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow(), recordErr: errors.New("database is down")}
	resp, err := redirectApp(t, links, 0).Test(humanGet("/cv/acme-x7abc"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Errorf("status = %d, want %d — the write failed, the redirect must not", resp.StatusCode, fiber.StatusFound)
	}
	if got := resp.Header.Get("Location"); got != "https://github.com/ada" {
		t.Errorf("Location = %q, want the destination", got)
	}
}

// The first thing a candidate does after enabling tracing is download the PDF and click the link
// to check it works. Reporting that back as "your CV was opened" would make the feature lie on
// first use.
func TestTheOwnersOwnClickIsMarkedAndNotCounted(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	resp, err := redirectApp(t, links, 7).Test(humanGet("/cv/acme-x7abc"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if len(links.clicks) != 1 || !links.clicks[0].IsOwner {
		t.Errorf("clicks = %+v, want one marked as the owner's", links.clicks)
	}
	if links.touched != 0 {
		t.Error("the owner's own click moved the CV-opened marker")
	}
}

func TestAnotherSignedInVisitorCountsNormally(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	resp, err := redirectApp(t, links, 8).Test(humanGet("/cv/acme-x7abc"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if len(links.clicks) != 1 || links.clicks[0].IsOwner {
		t.Errorf("clicks = %+v, want one NOT marked as the owner's", links.clicks)
	}
	if links.touched != 1 {
		t.Error("a signed-in stranger's click did not move the CV-opened marker")
	}
}

// The endpoint must not be usable as a redirector by anyone who can craft a URL. Nothing in the
// request may name a destination — the stored token is the only source.
func TestNothingInTheRequestCanChooseTheDestination(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	req := humanGet("/cv/acme-x7abc?url=https://evil.example/&to=https://evil.example/")
	req.Header.Set("Referer", "https://evil.example/")
	req.Header.Set("X-Forwarded-Host", "evil.example")
	resp, err := redirectApp(t, links, 0).Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if got := resp.Header.Get("Location"); got != "https://github.com/ada" {
		t.Errorf("Location = %q, want the stored destination — the request chose it", got)
	}
}

// A link checker issues HEAD. Freezing the verdict at write time is what stops a later edit to the
// detection rules from silently rewriting history.
func TestAHeadRequestIsRecordedAsAutomated(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	resp, err := redirectApp(t, links, 0).Test(httptest.NewRequestWithContext(context.Background(), http.MethodHead, "/cv/acme-x7abc", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if len(links.clicks) != 1 || !links.clicks[0].IsLikelyBot {
		t.Errorf("clicks = %+v, want one flagged automated", links.clicks)
	}
	if links.touched != 0 {
		t.Error("an automated fetch moved the CV-opened marker")
	}
}

// The referrer's host is kept; the rest of the URL is not. A full referrer carries the path and
// query of whatever page the reader came from, which is somebody else's business.
func TestOnlyTheReferrersHostIsKept(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	req := humanGet("/cv/acme-x7abc")
	req.Header.Set("Referer", "https://mail.example.com/inbox/thread/9182?q=secret")
	resp, err := redirectApp(t, links, 0).Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if len(links.clicks) != 1 {
		t.Fatalf("recorded %d clicks, want 1", len(links.clicks))
	}
	if got := links.clicks[0].ReferrerHost; got != "mail.example.com" {
		t.Errorf("referrer_host = %q, want just the host", got)
	}
}

// A request with no user agent at all is not a person reading a CV in a browser. Flagging it is
// deliberate, and it is stated here because the default httptest request looks exactly like this
// — a case that would otherwise be tested by accident rather than on purpose.
func TestARequestWithoutAUserAgentIsAutomated(t *testing.T) {
	links := &fakeTracerLinks{row: linkRow()}
	resp, err := redirectApp(t, links, 0).Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/cv/acme-x7abc", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if len(links.clicks) != 1 || !links.clicks[0].IsLikelyBot {
		t.Errorf("clicks = %+v, want one flagged automated", links.clicks)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Errorf("status = %d — a flagged visitor is still sent on their way", resp.StatusCode)
	}
}

// A database blip is not a deleted CV. Collapsing every lookup error into 410 would tell a
// recruiter the candidate removed their CV — a false statement about a person — and 410 means
// "gone for good", so a well-behaved gateway would stop retrying a link that is perfectly alive.
func TestATransientLookupFailureIsNotReportedAsDeletion(t *testing.T) {
	links := &fakeTracerLinks{lookupErr: errors.New("connection refused")}
	resp, err := redirectApp(t, links, 0).Test(humanGet("/cv/acme-x7abc"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode == fiber.StatusGone {
		t.Error("a transient failure was reported as the CV having been deleted")
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want %d — transient and retryable", resp.StatusCode, fiber.StatusInternalServerError)
	}
}
