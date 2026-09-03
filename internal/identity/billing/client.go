package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// apiBaseURL is the provider's REST API. Overridden only by tests.
const apiBaseURL = "https://api.revenuecat.com/v1"

// requestTimeout bounds one read of subscriber state.
//
// It is short on purpose. The webhook handler makes this call inline so that a candidate
// who has just paid sees Pro immediately, and the provider gives that handler 60 seconds
// before it disconnects and schedules a retry. Failing the apply quickly and leaving the
// event for the reconciler is strictly better than risking the delivery.
const requestTimeout = 8 * time.Second

// errorBodyLimit caps how much of a failure response is read into an error message.
const errorBodyLimit = 512

// client reads subscriber state from the provider. It is the only thing in this package
// that talks to the network.
type client struct {
	http    *http.Client
	baseURL string
	apiKey  string
}

// newProviderClient is the production constructor: an SSRF-guarded client against the real
// API.
func newProviderClient(apiKey string) *client {
	return newClient(apiKey, apiBaseURL, safehttp.NewClient(requestTimeout))
}

// newClient builds a client against an arbitrary base URL with an arbitrary HTTP client.
// The seam exists for tests: safehttp refuses private addresses, which is correct in
// production and makes a loopback test server unreachable.
func newClient(apiKey, baseURL string, httpc *http.Client) *client {
	return &client{http: httpc, baseURL: baseURL, apiKey: apiKey}
}

// subscriberState reads the provider's current record for one identifier.
//
// THIS GET CREATES THE SUBSCRIBER if the identifier is unknown to the provider — a read
// with a write's consequences. Callers must therefore only ask about accounts that have
// actually transacted; the reconciler's query enforces that structurally by starting from
// billing_events rather than from users.
func (c *client) subscriberState(ctx context.Context, appUserID string) (subscriber, error) {
	endpoint := c.baseURL + "/subscribers/" + url.PathEscape(appUserID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return subscriber{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return subscriber{}, fmt.Errorf("billing: reading subscriber %q: %w", appUserID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return subscriber{}, fmt.Errorf("billing: reading subscriber %q: provider returned %d: %s", appUserID, resp.StatusCode, snippet)
	}

	var payload struct {
		Subscriber subscriber `json:"subscriber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return subscriber{}, fmt.Errorf("billing: decoding subscriber %q: %w", appUserID, err)
	}
	return payload.Subscriber, nil
}
