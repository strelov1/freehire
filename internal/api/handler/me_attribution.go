package handler

import (
	"context"
	"errors"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/ga4mp"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// signupAttributionRequest is the POST /me/attribution body: the visit that led to
// the sign-up, as the browser saw it. Every field is optional and untrusted.
//
// GAClientID and GASessionID come from the visitor's own `_ga` cookies, which exist
// only after they accepted analytics; they are used to send the GA4 sign-up from the
// server and are never stored.
type signupAttributionRequest struct {
	UTMSource    string
	UTMMedium    string
	UTMCampaign  string
	UTMContent   string
	UTMTerm      string
	ReferrerHost string
	LandingPath  string
	Method       string
	GAClientID   string
	GASessionID  string
}

var hostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// cleanText trims, drops control characters and cuts to max runes. Empty is "".
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return strings.TrimSpace(s)
}

// normalizeReferrerHost accepts a bare host or a full URL and keeps the host only.
// Anything that is not a plausible hostname is dropped, never stored.
func normalizeReferrerHost(s string) string {
	// Reject rather than truncate: a hostname cut at 255 characters is a different,
	// made-up hostname that would pass the pattern below.
	if len(s) > 2048 {
		return ""
	}
	s = strings.ToLower(cleanText(s, 2048))
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			s = u.Hostname()
		}
	}
	s = strings.TrimPrefix(s, "www.")
	if len(s) > 255 || !hostPattern.MatchString(s) {
		return ""
	}
	return s
}

// normalizeLandingPath keeps the path only: the query string and fragment are cut,
// because they can carry a token or an address.
func normalizeLandingPath(s string) string {
	s = cleanText(s, 300)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if !strings.HasPrefix(s, "/") {
		return ""
	}
	return s
}

func (in signupAttributionRequest) params(userID int64) db.InsertSignupAttributionParams {
	return db.InsertSignupAttributionParams{
		UserID:       userID,
		UtmSource:    pgconv.Text(strings.ToLower(cleanText(in.UTMSource, 100))),
		UtmMedium:    pgconv.Text(strings.ToLower(cleanText(in.UTMMedium, 100))),
		UtmCampaign:  pgconv.Text(cleanText(in.UTMCampaign, 200)),
		UtmContent:   pgconv.Text(cleanText(in.UTMContent, 200)),
		UtmTerm:      pgconv.Text(cleanText(in.UTMTerm, 200)),
		ReferrerHost: pgconv.Text(normalizeReferrerHost(in.ReferrerHost)),
		LandingPath:  pgconv.Text(normalizeLandingPath(in.LandingPath)),
	}
}

// oauthAppMedium marks an account that was created to connect an OAuth client (Claude,
// ChatGPT, Cursor over MCP) rather than on the site itself.
const oauthAppMedium = "oauth_app"

// attributeOAuthAppSignup records how an account created inside an OAuth client's sign-in
// arrived. That person goes from sign-in through the server-rendered consent screen straight
// back to the client and never loads the SPA, so the browser's report never comes. The source
// is the client's https redirect host (a loopback or custom-scheme app has none). The SQL
// ignores an account older than ten minutes, so connecting a client later records nothing.
// The GA4 sign_up goes out only for the first record and only with the visitor's own `_ga`
// cookies (present after consent), sent with the consent form from our origin.
// Best effort, like the browser report: a failure must not block the approval.
func (h *authHandlers) attributeOAuthAppSignup(ctx context.Context, userID int64, redirectURI string, cookie func(string) string) {
	in := signupAttributionRequest{UTMMedium: oauthAppMedium, LandingPath: "/api/v1/oauth/authorize"}
	if u, err := url.Parse(redirectURI); err == nil && strings.EqualFold(u.Scheme, "https") {
		in.ReferrerHost = u.Hostname()
	}
	inserted, err := h.queries.InsertSignupAttribution(ctx, in.params(userID))
	if err != nil {
		log.Printf("attribution: storing oauth app sign-up for user %d: %v", userID, err)
		return
	}
	if inserted == 1 && h.ga4 != nil {
		clientID, sessionID := ga4mp.IDsFromCookies(cookie, os.Getenv("GA4_MEASUREMENT_ID"))
		// Off the request: nobody waits on the answer, and the client's redirect must not.
		go h.sendSignUp(userID, clientID, sessionID, "oauth") //nolint:gosec // G118: ctx is fasthttp's RequestCtx, recycled once the handler returns
	}
}

// sendSignUp sends the GA4 sign_up and reports whether the browser must stay out of it.
// `sent` answers "should the browser stay out of it?", not "did Google confirm?". GA4 cannot
// dedupe, so doubtful cases settle toward the server owning the event: a send that may have
// reached Google before the connection failed. Only a definite failure (nothing configured,
// no client id, a refusal) leaves it to the browser.
func (h *authHandlers) sendSignUp(userID int64, clientID, sessionID, method string) (sent bool) {
	if h.ga4 == nil || !ga4mp.ValidClientID(clientID) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	switch err := h.ga4.SignUp(ctx, clientID, sessionID, method); {
	case err == nil:
		return true
	case errors.Is(err, ga4mp.ErrUncertain):
		log.Printf("attribution: GA4 sign_up for user %d may not have been delivered: %v", userID, err)
		return true
	default:
		log.Printf("attribution: GA4 sign_up for user %d: %v", userID, err)
		return false
	}
}

// ReportSignupAttribution records how the caller's brand-new account arrived and,
// when the browser handed over a Google Analytics client id, sends the GA4 `sign_up`
// from here. A failure to store or send never surfaces as an error: attribution is
// observation, and it must not disturb a person who has just signed up. Cookie-only,
// like the other /me writes.
func (h *authHandlers) ReportSignupAttribution(ctx context.Context, req *apigen.AccountAttribution) (*apigen.ReportSignupAttributionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := signupAttributionRequest{
		UTMSource: req.UtmSource.Value, UTMMedium: req.UtmMedium.Value, UTMCampaign: req.UtmCampaign.Value,
		UTMContent: req.UtmContent.Value, UTMTerm: req.UtmTerm.Value, ReferrerHost: req.ReferrerHost.Value,
		LandingPath: req.LandingPath.Value, Method: req.Method.Value,
		GAClientID: req.GaClientID.Value, GASessionID: req.GaSessionID.Value,
	}

	inserted, err := h.queries.InsertSignupAttribution(ctx, in.params(userID))
	if err != nil {
		log.Printf("attribution: storing for user %d: %v", userID, err)
		return &apigen.ReportSignupAttributionOK{Data: apigen.AccountAttributionResult{}}, nil
	}
	// Only the first report for an account sends the event, so a retry or replay cannot count
	// one sign-up twice in GA. A report that lost the insert race leaves it to the winner.
	sent := inserted == 0
	if inserted == 1 {
		method := "oauth"
		if in.Method == "password" {
			method = "password"
		}
		sent = h.sendSignUp(userID, in.GAClientID, in.GASessionID, method)
	}
	return &apigen.ReportSignupAttributionOK{Data: apigen.AccountAttributionResult{Ga4Sent: sent}}, nil
}
