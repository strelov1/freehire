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

// serviceAccountKey is the part of a service account key file a two-legged JWT exchange needs.
type serviceAccountKey struct {
	Type         string `json:"type"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI     string `json:"token_uri"`
}

// jwtConfigFromServiceAccountKey builds the token source from the key file.
//
// It uses oauth2/jwt rather than oauth2/google, which would pull in
// cloud.google.com/go/compute/metadata (GCE auth) as a new dependency.
func jwtConfigFromServiceAccountKey(raw []byte) (*jwt.Config, error) {
	var key serviceAccountKey
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, err
	}
	// A wrong-kind key file parses fine but fails later with a misleading signing error.
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
// The API is open only to JobPosting and BroadcastEvent-in-VideoObject pages, so only job
// URLs may be sent. Anything else is a terms violation penalized with the quota, so the
// eligibility gate lives in SQL where a caller with a URL cannot bypass it.
//
// It requests a CRAWL, not an index entry.
type GoogleEngine struct {
	client   *http.Client
	budget   int
	endpoint string
}

const (
	googleEngineName  = "google"
	googleIndexingAPI = "https://indexing.googleapis.com/v3/urlNotifications:publish"
	googleIndexScope  = "https://www.googleapis.com/auth/indexing"

	// The documented default. An approved increase raises it, but the API cannot be
	// asked for its allowance (it only answers 429), so the number is configured.
	googleDefaultDailyBudget = 200
)

// NewGoogleEngine builds the engine from a service account key file, or returns nil
// when none is configured (the fleet's "this channel is off").
//
// A FILE rather than JSON in the environment: the key is multi-line and a newline ends a
// value in /opt/freehire/.env.
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

// Accepts admits only the posting events. The Indexing API is open to exactly two kinds
// of page, so a company page sent here is a terms violation penalized with the quota.
// The refusal lives here because it is a fact about Google, not about the run.
func (g *GoogleEngine) Accepts(kind Kind) bool { return !kind.isCompany() }

// Announce publishes each URL in turn; quota is counted per URL, so batching would save
// nothing.
//
// A 429 stops the run for this engine: the day's allowance is gone, and a failed call
// consumes quota on every status BUT 429, so continuing would spend tomorrow's.
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
		// Google's body is the only thing naming the cause (e.g. a 403 for a service
		// account that is a full user rather than an owner).
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}
