// Package contactfinder is the client of the private contact-finder service, which returns a
// company's recruiter contacts. Which data source answers is that service's business: nothing
// here, in the database or on the wire names it.
package contactfinder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// ErrUnavailable is any failure to get an answer: the service is down, refused, or answered
// something unreadable. The caller charges nobody for it.
var ErrUnavailable = errors.New("contactfinder: unavailable")

// Contact is one person as the service returns it and as the API serves it.
type Contact struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Email      string `json:"email"`
	Confidence string `json:"confidence"` // "high" | "medium"
}

// Client calls the service with its bearer token.
type Client struct {
	url   string
	token string
	http  *http.Client
}

// New returns a client for the service at url.
func New(url, token string) *Client {
	return &Client{url: url, token: token, http: &http.Client{Timeout: 15 * time.Second}}
}

// FromEnv returns a client from CONTACT_FINDER_URL and CONTACT_FINDER_TOKEN, or nil when the
// URL is unset: the feature is off.
func FromEnv() *Client {
	url := os.Getenv("CONTACT_FINDER_URL")
	if url == "" {
		return nil
	}
	return New(url, os.Getenv("CONTACT_FINDER_TOKEN"))
}

// Find returns up to three recruiter contacts at domain, [] when nothing usable is known.
// userRef is recorded by the service so a person can learn to whom their address was shown.
func (c *Client) Find(ctx context.Context, domain, userRef string) ([]Contact, error) {
	var out struct {
		Contacts []Contact `json:"contacts"`
	}
	if err := c.post(ctx, "/v1/find", map[string]string{"domain": domain, "user_ref": userRef}, &out); err != nil {
		return nil, err
	}
	if out.Contacts == nil {
		return []Contact{}, nil
	}
	return out.Contacts, nil
}

// Suppressed returns which of emails were asked to be removed since they were found.
func (c *Client) Suppressed(ctx context.Context, emails []string) (map[string]bool, error) {
	var out struct {
		Suppressed []string `json:"suppressed"`
	}
	if err := c.post(ctx, "/v1/check", map[string][]string{"emails": emails}, &out); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(out.Suppressed))
	for _, e := range out.Suppressed {
		set[e] = true
	}
	return set, nil
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}
