package searchping

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2/jwt"
)

// serviceAccountKey is the part of a Google service account key file this needs. The
// file also carries project_id, client_id and the certificate URLs, none of which
// matter to a two-legged JWT exchange.
type serviceAccountKey struct {
	Type         string `json:"type"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI     string `json:"token_uri"`
}

// jwtConfigFromServiceAccountKey builds the token source from the key file.
//
// oauth2/jwt rather than oauth2/google, though the latter has a one-line helper for
// exactly this: google's package reaches for cloud.google.com/go/compute/metadata so it
// can also authenticate from inside a GCE instance, a path this fleet has no use for,
// and the module is not otherwise a dependency. The four fields below are the whole of
// what the helper would have read.
func jwtConfigFromServiceAccountKey(raw []byte) (*jwt.Config, error) {
	var key serviceAccountKey
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, err
	}
	// A key file for the wrong credential kind parses fine and then fails at the token
	// exchange with a signing error, which reads like a bug here rather than the wrong
	// file having been copied to the host.
	if key.Type != "service_account" {
		return nil, fmt.Errorf("credential type is %q, want service_account", key.Type)
	}
	if key.ClientEmail == "" || key.PrivateKey == "" {
		return nil, fmt.Errorf("credential is missing client_email or private_key")
	}
	tokenURI := key.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	return &jwt.Config{
		Email:        key.ClientEmail,
		PrivateKey:   []byte(key.PrivateKey),
		PrivateKeyID: key.PrivateKeyID,
		Scopes:       []string{googleIndexScope},
		TokenURL:     tokenURI,
	}, nil
}

// GoogleEngine announces a URL through Google's Indexing API.
//
// The API is open to exactly two kinds of page — JobPosting, and BroadcastEvent inside
// a VideoObject — which is why it is available to this site at all and why nothing but
// a job URL may be sent through it. Submitting anything else is a terms violation, and
// the penalty is the quota, so the eligibility gate lives in SQL where it cannot be
// bypassed by a caller with a URL in hand.
//
// It requests a CRAWL, not an index entry: Google still decides whether the page is
// worth holding. That is the honest ceiling on what this can do — it removes the
// discovery delay, not Google's judgement.
type GoogleEngine struct {
	client   *http.Client
	budget   int
	endpoint string
}

const (
	googleEngineName  = "google"
	googleIndexingAPI = "https://indexing.googleapis.com/v3/urlNotifications:publish"
	googleIndexScope  = "https://www.googleapis.com/auth/indexing"

	// The documented default. An approved increase raises it, so it is configurable —
	// but it is not discoverable: the API answers 429 when the day is spent and offers
	// no way to ask what the allowance is, so this number has to be told to us.
	googleDefaultDailyBudget = 200
)

// NewGoogleEngine builds the engine from a service account key file, or returns nil
// when none is configured — the fleet's convention for "this channel is off", which is
// also how the feature ships before the credential exists and how it is rolled back.
//
// A FILE rather than the JSON itself in the environment: the key is multi-line and the
// host passes configuration through /opt/freehire/.env, where a newline ends the
// value. A truncated credential fails at the first call with an error about signing,
// which reads like a bug in this code rather than a mangled secret.
func NewGoogleEngine(ctx context.Context, keyPath string, dailyBudget int, timeout time.Duration) (*GoogleEngine, error) {
	keyPath = strings.TrimSpace(keyPath)
	if keyPath == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read indexing credentials: %w", err)
	}
	config, err := jwtConfigFromServiceAccountKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse indexing credentials: %w", err)
	}
	if dailyBudget <= 0 {
		dailyBudget = googleDefaultDailyBudget
	}

	client := config.Client(ctx)
	client.Timeout = timeout
	return &GoogleEngine{client: client, budget: dailyBudget, endpoint: googleIndexingAPI}, nil
}

func (g *GoogleEngine) Name() string { return googleEngineName }

func (g *GoogleEngine) DailyBudget() int { return g.budget }

// Announce publishes each URL in turn. One HTTP call per URL is the API's own shape —
// batching there saves HTTP overhead and NOT quota, which is counted per URL, so the
// loop costs nothing a batch would have saved.
//
// A 429 stops the run for this engine rather than walking the rest of the batch: the
// day's allowance is gone, every further call would fail the same way, and a failed
// call still consumes quota on every status BUT 429 — so continuing past the one
// status that is free would start spending tomorrow's.
func (g *GoogleEngine) Announce(ctx context.Context, urls []string) ([]string, error) {
	accepted := make([]string, 0, len(urls))
	for _, url := range urls {
		err := g.publish(ctx, url)
		switch {
		case err == nil:
			accepted = append(accepted, url)
		case isQuotaExhausted(err):
			return accepted, fmt.Errorf("daily quota exhausted after %d of %d: %w", len(accepted), len(urls), err)
		default:
			return accepted, fmt.Errorf("publish %s: %w", url, err)
		}
	}
	return accepted, nil
}

type quotaError struct{ body string }

func (e *quotaError) Error() string { return "indexing api 429: " + e.body }

func isQuotaExhausted(err error) bool {
	var q *quotaError
	return errors.As(err, &q)
}

func (g *GoogleEngine) publish(ctx context.Context, url string) error {
	payload, err := json.Marshal(map[string]string{"url": url, "type": "URL_UPDATED"})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return &quotaError{body: strings.TrimSpace(string(body))}
	default:
		// The body carries Google's own words, and for the failure an operator will
		// actually meet — a 403 from a service account that is a full user rather than
		// an owner — those words are the only thing that names the cause.
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}
