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
//
// v2, and not by preference: the project's secret key is a v2 key, and v1 refuses it with
// "You're trying to use a secret API key incompatible with RevenueCat API V1". The first
// draft of this package spoke v1 and would have answered 403 to every sync.
const apiBaseURL = "https://api.revenuecat.com/v2"

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
	http      *http.Client
	baseURL   string
	apiKey    string
	projectID string
}

// newProviderClient is the production constructor: an SSRF-guarded client against the real
// API.
func newProviderClient(apiKey, projectID string) *client {
	return newClient(apiKey, projectID, apiBaseURL, safehttp.NewClient(requestTimeout))
}

// newClient builds a client against an arbitrary base URL with an arbitrary HTTP client.
// The seam exists for tests: safehttp refuses private addresses, which is correct in
// production and makes a loopback test server unreachable.
func newClient(apiKey, projectID, baseURL string, httpc *http.Client) *client {
	return &client{http: httpc, baseURL: baseURL, apiKey: apiKey, projectID: projectID}
}

// subscriberState reads the provider's current record for one identifier.
//
// A customer the provider has never seen answers 404, and that is NOT an error: it means
// no purchase, which derives to no entitlement and the free plan. Treating it as a failure
// would leave events unprocessed forever for every identifier that was never ours.
//
// Unlike v1's subscriber GET, this read has no write behind it — v2 does not create a
// customer for an unknown id. The rule that callers only ask about accounts which have
// transacted therefore stops being load-bearing, though the reconciler's query still holds
// to it because it is also the cheaper way to find candidates.
func (c *client) subscriberState(ctx context.Context, appUserID string) (subscriber, error) {
	endpoint := c.baseURL + "/projects/" + url.PathEscape(c.projectID) + "/customers/" + url.PathEscape(appUserID)

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

	// Never purchased anything, so there is nothing to be entitled to. An empty customer
	// derives to the zero time, which resolves to the free plan.
	if resp.StatusCode == http.StatusNotFound {
		return subscriber{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return subscriber{}, fmt.Errorf("billing: reading customer %q: provider returned %d: %s", appUserID, resp.StatusCode, snippet)
	}

	// The customer object carries active_entitlements inline; there is no envelope to
	// unwrap, unlike v1's {"subscriber": …}.
	var out subscriber
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return subscriber{}, fmt.Errorf("billing: decoding customer %q: %w", appUserID, err)
	}
	return out, nil
}
