package handler

import (
	"context"
	"errors"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/ga4mp"
	"github.com/strelov1/freehire/internal/platform/db"
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

func textOrNull(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

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
		UtmSource:    textOrNull(strings.ToLower(cleanText(in.UTMSource, 100))),
		UtmMedium:    textOrNull(strings.ToLower(cleanText(in.UTMMedium, 100))),
		UtmCampaign:  textOrNull(cleanText(in.UTMCampaign, 200)),
		UtmContent:   textOrNull(cleanText(in.UTMContent, 200)),
		UtmTerm:      textOrNull(cleanText(in.UTMTerm, 200)),
		ReferrerHost: textOrNull(normalizeReferrerHost(in.ReferrerHost)),
		LandingPath:  textOrNull(normalizeLandingPath(in.LandingPath)),
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
	// Only the first report for an account sends the event, so a retried or replayed
	// request cannot count one sign-up twice in GA.
	//
	// `sent` answers "should the browser stay out of it?", not strictly "did Google
	// confirm?". GA4 cannot dedupe, so each doubtful case is settled toward the server
	// owning the event rather than a double count: a report that lost the insert race
	// (another browser already reported this account) and a send that may have reached
	// Google before the connection failed. Only a definite failure — nothing configured,
	// no client id, a refusal from Google — leaves the browser to send it.
	sent := inserted == 0
	if inserted == 1 && h.ga4 != nil && ga4mp.ValidClientID(in.GAClientID) {
		method := "oauth"
		if in.Method == "password" {
			method = "password"
		}
		sendCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		switch err := h.ga4.SignUp(sendCtx, in.GAClientID, in.GASessionID, method); {
		case err == nil:
			sent = true
		case errors.Is(err, ga4mp.ErrUncertain):
			sent = true
			log.Printf("attribution: GA4 sign_up for user %d may not have been delivered: %v", userID, err)
		default:
			log.Printf("attribution: GA4 sign_up for user %d: %v", userID, err)
		}
	}
	return &apigen.ReportSignupAttributionOK{Data: apigen.AccountAttributionResult{Ga4Sent: sent}}, nil
}
