// Package ga4mp reports a sign-up to Google Analytics 4 from the server, through the
// Measurement Protocol.
//
// The browser hands the server the client and session ids and the server sends the
// event, so it survives a tab lost in the OAuth redirect. The browser must NOT send
// it too: GA4 has no event-level dedup, so the account would count twice.
//
// Consent: a client id exists only for a visitor who accepted analytics cookies, and
// nothing is sent without one. This package never mints an id of its own.
package ga4mp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const endpoint = "https://www.google-analytics.com/mp/collect"

// clientIDPattern is the shape of a GA client id: two dot-separated integers. Anything
// else is a forged or garbled value and is dropped rather than relayed to Google.
var clientIDPattern = regexp.MustCompile(`^[0-9]{1,12}\.[0-9]{1,12}$`)

// sessionIDPattern is a GA session id: a unix timestamp.
var sessionIDPattern = regexp.MustCompile(`^[0-9]{1,12}$`)

// ErrUncertain means the request may have reached Google: it was attempted, then the
// connection failed or timed out. The sign-up might be counted, so a caller that would
// otherwise send it again must not (GA4 has no event-level dedup).
var ErrUncertain = errors.New("ga4mp: outcome unknown")

// Reporter sends sign-ups. A nil *Client is a valid, inert Reporter.
type Reporter interface {
	SignUp(ctx context.Context, clientID, sessionID, method string) error
}

// Client talks to the Measurement Protocol collect endpoint.
type Client struct {
	measurementID string
	apiSecret     string
	baseURL       string
	http          *http.Client
}

// FromEnv builds a Client from GA4_MEASUREMENT_ID and GA4_API_SECRET, or returns nil
// (inert) when either is missing, so a deployment without the secret simply sends
// nothing.
func FromEnv(getenv func(string) string) *Client {
	id, secret := getenv("GA4_MEASUREMENT_ID"), getenv("GA4_API_SECRET")
	if id == "" || secret == "" {
		return nil
	}
	return &Client{
		measurementID: id,
		apiSecret:     secret,
		baseURL:       endpoint,
		http:          &http.Client{Timeout: 5 * time.Second},
	}
}

// ValidClientID reports whether s looks like a GA client id.
func ValidClientID(s string) bool { return clientIDPattern.MatchString(s) }

// SignUp sends one `sign_up` event. An invalid client id is an error, not a send.
func (c *Client) SignUp(ctx context.Context, clientID, sessionID, method string) error {
	if c == nil {
		return nil
	}
	if !ValidClientID(clientID) {
		return fmt.Errorf("ga4mp: invalid client id")
	}
	params := map[string]any{"method": method, "engagement_time_msec": 1}
	if sessionIDPattern.MatchString(sessionID) {
		params["session_id"] = sessionID
	}
	body, err := json.Marshal(map[string]any{
		"client_id": clientID,
		"events":    []any{map[string]any{"name": "sign_up", "params": params}},
	})
	if err != nil {
		return err
	}
	q := url.Values{"measurement_id": {c.measurementID}, "api_secret": {c.apiSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// A *url.Error carries the request URL, whose query holds the API secret; keep
		// only the cause so the secret never reaches logs.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("%w: %v", ErrUncertain, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ga4mp: collect answered %d", resp.StatusCode)
	}
	return nil
}

var (
	clientCookiePattern  = regexp.MustCompile(`^GA\d+\.\d+\.(\d+\.\d+)$`)
	sessionCookiePattern = regexp.MustCompile(`^GS\d\.\d\.s?(\d+)`)
)

// IDsFromCookies reads the client and session ids from the visitor's own `_ga` and
// `_ga_<container>` cookies, the way the browser does (web/src/lib/attribution.ts), for
// a request that never reaches the SPA. Missing or malformed cookies give "": the cookies
// exist only after consent, so an absent id means nothing may be sent.
func IDsFromCookies(cookie func(name string) string, measurementID string) (clientID, sessionID string) {
	if m := clientCookiePattern.FindStringSubmatch(cookie("_ga")); m != nil {
		clientID = m[1]
	}
	if m := sessionCookiePattern.FindStringSubmatch(cookie("_ga_" + strings.TrimPrefix(measurementID, "G-"))); m != nil {
		sessionID = m[1]
	}
	return clientID, sessionID
}
