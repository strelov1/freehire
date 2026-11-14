package changehook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// requestTimeout bounds one delivery attempt: a slow endpoint must not hold the run.
const requestTimeout = 10 * time.Second

// ErrGone is a 410 from the endpoint: the one answer that says "stop", so the subscription
// is disabled at once instead of being retried.
var ErrGone = errors.New("changehook: endpoint responded 410 Gone")

// Sender POSTs signed events through an SSRF-guarded client.
type Sender struct{ http *http.Client }

// NewSender is the production sender: safehttp refuses private and loopback addresses, and
// redirects are not followed (a redirect is a failed delivery, so a signed body never leaves
// for a URL the owner did not register).
func NewSender() *Sender { return newSender(safehttp.NewClient(requestTimeout)) }

// newSender takes any client; tests use it to reach a loopback server.
func newSender(c *http.Client) *Sender {
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Sender{http: c}
}

// ValidateURL is the creation-time check: an absolute http(s) URL with a host.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be an absolute http or https URL")
	}
	return nil
}

// Send signs and POSTs ev. A 2xx is success, 410 is ErrGone, anything else an error to retry.
func (s *Sender) Send(ctx context.Context, dest, secret string, ev Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("changehook: encode event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("changehook: build request: %w", err)
	}
	ts := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, Sign(secret, ts, body))
	req.Header.Set(HeaderDelivery, ev.ID)
	req.Header.Set(HeaderEvent, ev.Type)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("changehook: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("changehook: endpoint responded %d", resp.StatusCode)
	}
	return nil
}

// Test sends an event with no changes, so an owner can prove the URL and the signature check
// before relying on them.
func (s *Sender) Test(ctx context.Context, dest, secret string) error {
	return s.Send(ctx, dest, secret, Event{ID: "test-" + strconv.FormatInt(time.Now().UnixNano(), 36), Type: EventTest, Changes: []Change{}})
}
