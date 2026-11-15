package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// revenuecatName is what billing_events.provider records for this provider.
const revenuecatName = "revenuecat"

// revenuecatSignatureHeader is the header RevenueCat signs deliveries with, using the scheme
// verifySignature implements (`t=<unix>,v1=<hex>` over "<t>.<raw body>").
const revenuecatSignatureHeader = "X-RevenueCat-Webhook-Signature"

// revenuecatAPIBaseURL is the provider's REST API. Overridden only by tests.
const revenuecatAPIBaseURL = "https://api.revenuecat.com/v1"

// defaultRevenueCatEntitlement is the entitlement id a purchase must confer to mean Pro.
const defaultRevenueCatEntitlement = "pro"

// revenuecatSignatureWindow bounds how old a signed delivery may be. It is wider than Stripe's
// five minutes because RevenueCat's docs do not say whether retries are re-signed and its last
// retry arrives 80 minutes after the first; a narrow window would silently drop every retry and
// paid subscriptions with it. A too-wide window only lengthens the life of a captured delivery,
// whose replay is idempotent by event id. Narrow it once a real retried delivery is inspected.
const revenuecatSignatureWindow = 90 * time.Minute

// revenuecatLifetimeHorizon is how far a NON-EXPIRING entitlement is written forward.
// RevenueCat returns a null `expires_date` for a lifetime purchase or a dashboard promo grant.
// We sell no such product, so nobody would notice this being wrong.
//
// The column cannot say "forever", and a sentinel is the hazard entitlement.go documents, so
// "forever" is written as "for as long as we keep confirming": the reconciler's second pass
// re-reads accounts near expiry and pushes the horizon out, and the grant lapses within a month
// if revoked or if we stop asking (failing closed).
//
// It is COUPLED TO THE RECONCILER'S WINDOW. A row at now+35d enters `dueSoon`'s band once and
// stays for twice the window; a non-expiring entitlement sends no webhooks, so if every pass in
// that band is missed the grant lapses silently at the horizon. Tolerable because it fails
// closed, and bounded by two requirements asserted in
// TestTheLifetimeHorizonOutlivesTheSyncWindow: the horizon must exceed the window, and the
// hourly schedule must give many passes inside it.
const revenuecatLifetimeHorizon = 35 * 24 * time.Hour

// RevenueCatConfig is everything the environment decides about the store provider. It is
// separate from Config because the two providers are enabled and disabled independently.
type RevenueCatConfig struct {
	// APIKey is RevenueCat's SECRET key (sk_…), used server-side only.
	APIKey string
	// WebhookSecret signs incoming deliveries. See verifySignature.
	WebhookSecret string
	// Entitlement is the entitlement id that means Pro; anything else a subscriber holds is
	// another product.
	Entitlement string
}

// RevenueCatConfigFromEnv reads the configuration. It NEVER fails: absent credentials mean
// the store provider is switched off, not that the deployment is broken.
func RevenueCatConfigFromEnv() RevenueCatConfig {
	cfg := RevenueCatConfig{
		APIKey:        strings.TrimSpace(os.Getenv("REVENUECAT_API_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("REVENUECAT_WEBHOOK_SECRET")),
		Entitlement:   strings.TrimSpace(os.Getenv("REVENUECAT_ENTITLEMENT")),
	}
	if cfg.Entitlement == "" {
		cfg.Entitlement = defaultRevenueCatEntitlement
	}
	return cfg
}

// Enabled reports whether the store provider can verify a delivery, read a subscriber's state
// and knows which entitlement to look for. The entitlement is required despite its default:
// without a name to match, every sync would derive "no Pro" and quietly downgrade every store
// subscriber.
func (c RevenueCatConfig) Enabled() bool {
	return c.APIKey != "" && c.WebhookSecret != "" && c.Entitlement != ""
}

// RevenueCat is the App Store and Google Play, behind the shared engine. It has no checkout,
// portal, prices or receipts: store subscriptions are managed inside the store, where we have
// no API.
type RevenueCat struct {
	*engine
	cfg RevenueCatConfig
}

// NewRevenueCat constructs the store provider. It never fails; an unconfigured one is disabled
// and refuses every operation with ErrDisabled.
func NewRevenueCat(cfg RevenueCatConfig, q *db.Queries) *RevenueCat {
	return NewRevenueCatWithBase(cfg, q, revenuecatAPIBaseURL)
}

// NewRevenueCatWithBase is NewRevenueCat pointed at a different base URL, for tests with a
// stub. It dials without the SSRF guard for the reason NewWithBase gives.
func NewRevenueCatWithBase(cfg RevenueCatConfig, q *db.Queries, baseURL string) *RevenueCat {
	p := &revenuecatProvider{cfg: cfg, q: q, baseURL: baseURL}
	if cfg.Enabled() {
		if baseURL == revenuecatAPIBaseURL {
			client := safehttp.NewClient(requestTimeout)
			client.CheckRedirect = refuseRedirect
			p.http = client
		} else {
			p.http = &http.Client{Timeout: requestTimeout}
		}
	}
	return &RevenueCat{engine: &engine{p: p, q: q}, cfg: cfg}
}

// revenuecatProvider is RevenueCat behind the provider seam.
type revenuecatProvider struct {
	cfg     RevenueCatConfig
	q       *db.Queries
	http    *http.Client
	baseURL string
}

func (p *revenuecatProvider) name() string { return revenuecatName }

func (p *revenuecatProvider) enabled() bool { return p.cfg.Enabled() }

func (p *revenuecatProvider) signatureHeader() string { return revenuecatSignatureHeader }

func (p *revenuecatProvider) accept(raw []byte, signature string, now time.Time) (Event, error) {
	if err := verifySignature(raw, signature, p.cfg.WebhookSecret, revenuecatSignatureWindow, now); err != nil {
		return Event{}, err
	}
	return parseRevenueCatEvent(raw)
}

// account resolves the user with one lookup and no database write, because `app_user_id` IS
// our users.id (Stripe needs a stored customer id because a first purchase arrives before any
// binding).
//
// It still CONFIRMS the account exists: billing_events has a foreign key on user_id, so a
// delivery naming a deleted account or a sandbox app pointed at production would fail the
// insert, answer 500, and after five retries RevenueCat may disable the endpoint for everyone.
// Returning false records the event with a NULL user instead.
//
// It falls back to CustomerID when UserRef is empty: `applyPending` rebuilds an event from the
// stored row and can fill UserRef only from a non-NULL user_id, which is exactly the row that
// needs attributing. Without the fallback the worker would read "nothing to apply" and stamp a
// purchase processed. Both fields carry the same string for this provider.
func (p *revenuecatProvider) account(ctx context.Context, ev Event) (int64, bool) {
	ref := ev.UserRef
	if ref == "" {
		ref = ev.CustomerID
	}

	id, ok := userIDFromRef(ref)
	if !ok {
		return 0, false
	}
	if _, err := p.q.UserEmail(ctx, id); err != nil {
		return 0, false
	}
	return id, true
}

// bind is a no-op: there is no second identifier to remember, since the reconciler reaches a
// subscriber by the same users.id the webhook carried.
func (p *revenuecatProvider) bind(context.Context, int64, Event) error { return nil }

// knows reports whether RevenueCat has ever been heard from about this account: a recorded
// delivery of theirs, or a store entitlement we already hold. The v1 subscribers endpoint
// CREATES the subscriber for an unknown identifier, so a pass over the user table without this
// check would register every account with the provider. An unreadable answer counts as "not
// known": the sync is merely retried.
func (p *revenuecatProvider) knows(ctx context.Context, userID int64) (bool, error) {
	known, err := p.q.HasRevenueCatFootprint(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("billing: looking for a RevenueCat footprint on user %d: %w", userID, err)
	}
	return known.Valid && known.Bool, nil
}

// reach reads the subscriber's entitlements and reduces the configured one to how far it
// extends. It does not ask knows(): whether an unknown account may be asked about depends on
// WHO is asking (see engine.SyncUser and engine.SyncCaller). Ultra is reported as ZERO, not
// omitted, because this provider owns that column and could never clear it otherwise.
func (p *revenuecatProvider) reach(ctx context.Context, userID int64) (entitlement, error) {
	body, err := p.subscriberBody(ctx, userID)
	if err != nil {
		return entitlement{}, err
	}
	until, err := revenuecatReach(body, p.cfg.Entitlement, time.Now().UTC())
	if err != nil {
		return entitlement{}, err
	}
	return entitlement{Pro: until}, nil
}

// store writes both of this provider's source columns, Ultra always NULL today. Writing the
// column for a nonexistent product keeps it revocable on the day one exists.
func (p *revenuecatProvider) store(ctx context.Context, userID int64, ent entitlement) error {
	return p.q.SetRevenueCatEntitlement(ctx, db.SetRevenueCatEntitlementParams{
		ProUntil:   nullable(ent.Pro),
		UltraUntil: nullable(ent.Ultra),
		ID:         userID,
	})
}

// dueSoon reaches the accounts whose store entitlement is about to lapse: those that hold one,
// which is also the set reach() may ask about.
func (p *revenuecatProvider) dueSoon(ctx context.Context, from, to time.Time, max int32) ([]int64, error) {
	return p.q.ListSubscribersNearProExpiryRevenueCat(ctx, db.ListSubscribersNearProExpiryRevenueCatParams{
		FromTime: stamp(from),
		ToTime:   stamp(to),
		MaxRows:  max,
	})
}

// subscriberBody fetches one subscriber, whole, as received. The identifier is escaped as a
// PATH SEGMENT even though a users.id needs none, so a change in its shape is not an injection.
func (p *revenuecatProvider) subscriberBody(ctx context.Context, userID int64) ([]byte, error) {
	path := p.baseURL + "/subscribers/" + url.PathEscape(fmt.Sprint(userID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("billing: GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return nil, fmt.Errorf("billing: GET %s: provider returned %d: %s", path, resp.StatusCode, snippet)
	}

	// Bounded: the body is decided by the provider, and a subscriber needs far less than 1 MiB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, subscriberBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("billing: reading GET %s: %w", path, err)
	}
	return body, nil
}

// subscriberBodyLimit caps how much of a subscriber response is read.
const subscriberBodyLimit = 1 << 20

// revenuecatSubscriber is the reply shape, trimmed to what decides a plan.
//
// The date fields are json.RawMessage, not *string, so ABSENT and NULL stay distinguishable: a
// present-but-null expiry means non-expiring and confers Pro, while an object with neither key
// is an unreadable entitlement and must confer nothing. As pointers, `{}` and
// `{"expires_date":null}` would be equal and the first would grant a plan nobody bought.
type revenuecatSubscriber struct {
	Subscriber struct {
		Entitlements map[string]struct {
			ExpiresDate            json.RawMessage `json:"expires_date"`
			GracePeriodExpiresDate json.RawMessage `json:"grace_period_expires_date"`
		} `json:"entitlements"`
	} `json:"subscriber"`
}

// revenuecatReach reduces a subscriber's entitlements to the instant users.pro_until_revenuecat
// holds: how far the configured entitlement stands. The zero time with no error means this
// provider confers nothing, NOT that the account is free; another source may confer.
//
// An entitlement whose reach cannot be read returns an error and the zero time: failing closed
// costs a support message, failing open costs revenue unnoticed.
func revenuecatReach(body []byte, entitlement string, now time.Time) (time.Time, error) {
	var raw revenuecatSubscriber
	if err := json.Unmarshal(body, &raw); err != nil {
		return time.Time{}, fmt.Errorf("billing: decoding the subscriber: %w", err)
	}

	ent, held := raw.Subscriber.Entitlements[entitlement]
	if !held {
		return time.Time{}, nil
	}

	// NEITHER KEY PRESENT is an unreadable entitlement, not a lifetime one: `{}` is what a renamed
	// field looks like, and granting a month of Pro for it would fail open invisibly.
	if len(ent.ExpiresDate) == 0 && len(ent.GracePeriodExpiresDate) == 0 {
		return time.Time{}, fmt.Errorf("billing: entitlement %q carries neither an expiry nor a grace period", entitlement)
	}

	expires, expiresNull, err := parseRevenueCatDate(ent.ExpiresDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("billing: entitlement %q has an unreadable expiry: %w", entitlement, err)
	}
	grace, graceNull, err := parseRevenueCatDate(ent.GracePeriodExpiresDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("billing: entitlement %q has an unreadable grace period: %w", entitlement, err)
	}

	// Present and explicitly null on both: "this does not expire". See revenuecatLifetimeHorizon.
	if expires.IsZero() && grace.IsZero() {
		if expiresNull || graceNull {
			return now.Add(revenuecatLifetimeHorizon), nil
		}
		return time.Time{}, fmt.Errorf("billing: entitlement %q has no readable reach", entitlement)
	}
	if grace.After(expires) {
		return grace, nil
	}
	return expires, nil
}

// parseRevenueCatDate reads one ISO 8601 instant and reports whether the field was present and
// explicitly null. ABSENT and NULL both yield the zero time but only NULL means "does not
// expire"; collapsing them would turn an unknown shape into a month of free Pro.
func parseRevenueCatDate(raw json.RawMessage) (at time.Time, wasNull bool, err error) {
	if len(raw) == 0 {
		return time.Time{}, false, nil
	}
	if string(raw) == "null" {
		return time.Time{}, true, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, false, err
	}
	if s == "" {
		return time.Time{}, false, nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false, err
	}
	return parsed.UTC(), false, nil
}

// parseRevenueCatEvent reads the webhook envelope: `{"api_version":…, "event":{…}}`. It
// requires only an id, the one field idempotency depends on, so a new event type is not a
// rejected delivery. The event's own entitlement claims are deliberately not read: a webhook
// says something changed, and the new state is read from the provider.
func parseRevenueCatEvent(raw []byte) (Event, error) {
	var envelope struct {
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Event{}, err
	}
	if len(envelope.Event) == 0 {
		return Event{}, errNoEvent
	}

	var fields struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		AppUserID string `json:"app_user_id"`
	}
	if err := json.Unmarshal(envelope.Event, &fields); err != nil {
		return Event{}, err
	}
	if fields.ID == "" {
		// No id means no idempotency key; refusing makes the provider retry, the safe direction.
		return Event{}, errors.New("billing: event has no id")
	}

	// CustomerID and UserRef are the same string here and both are filled: the first is the
	// recorded subject, the second is what account() resolves.
	return Event{
		ID:         fields.ID,
		Type:       fields.Type,
		CustomerID: fields.AppUserID,
		UserRef:    fields.AppUserID,
		Payload:    envelope.Event,
	}, nil
}
