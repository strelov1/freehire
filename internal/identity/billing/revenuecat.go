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

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// revenuecatName is what billing_events.provider records for this provider.
const revenuecatName = "revenuecat"

// revenuecatSignatureHeader is the header RevenueCat signs each delivery with. The scheme is
// the one verifySignature already implements — `t=<unix>,v1=<hex>` over "<t>.<raw body>" —
// so only the name differs from Stripe's.
const revenuecatSignatureHeader = "X-RevenueCat-Webhook-Signature"

// revenuecatAPIBaseURL is the provider's REST API. Overridden only by tests.
const revenuecatAPIBaseURL = "https://api.revenuecat.com/v1"

// defaultRevenueCatEntitlement is the entitlement id a purchase must confer to mean Pro.
const defaultRevenueCatEntitlement = "pro"

// revenuecatSignatureWindow bounds how old a signed delivery may be.
//
// WIDER THAN STRIPE'S FIVE MINUTES, and provisionally so. Stripe re-signs every retry, which
// is what lets its window be narrow — the window bounds the age of the SIGNATURE, not of the
// event. RevenueCat's documentation does not say whether it re-signs, and its last retry
// arrives 80 minutes after the first. If it does not re-sign, a five-minute window would
// reject every retry and leave the reconciler as the only path, silently.
//
// So this fails in the safe direction until somebody inspects a real retried delivery: a
// too-narrow window drops paid subscriptions, while a too-wide one only lengthens the life of
// a captured delivery whose replay is already idempotent by event id. Narrow it to five
// minutes once the answer is known — see task 7.2.
const revenuecatSignatureWindow = 90 * time.Minute

// revenuecatLifetimeHorizon is how far a NON-EXPIRING entitlement is written forward.
//
// RevenueCat returns a null `expires_date` for an entitlement that does not expire — a
// lifetime purchase, or a promotional grant made in the dashboard. We sell no such product,
// which is exactly why this must be right: nobody would notice it being wrong.
//
// The column is a timestamp and cannot say "forever", and inventing a sentinel that means it
// is the hazard entitlement.go already documents: the first Stripe draft had one, and a
// misread field would have turned every subscriber into a permanent one whose cancellation
// never took effect. So "forever" is written as "for as long as we keep confirming". The
// reconciler's second pass re-reads accounts near their expiry and pushes the horizon out
// again, so the grant persists while RevenueCat still reports it, and lapses within a month
// if it is revoked or if we stop asking. Failing closed on a stalled reconciler is the right
// direction; failing open would be indefinite.
//
// IT IS COUPLED TO THE RECONCILER'S WINDOW, and the coupling is the fragile part. A row at
// now+35d enters `dueSoon`'s band once, about 34 days later, and stays in it for twice the
// window. Miss every pass in that band — the worker down for two days, the pass erroring, the
// account past BILLING_SYNC_MAX_PER_RUN each time — and the row leaves the band for good: a
// non-expiring entitlement generates no further webhooks, so nothing else would ever look at
// it. The grant then lapses silently at the horizon.
//
// That is the failing-closed direction, which is why it is tolerable rather than a bug. It is
// bounded by two requirements, both asserted in TestTheLifetimeHorizonOutlivesTheSyncWindow:
// the horizon must exceed the window, and the hourly schedule must give many passes inside
// it. Shortening one or widening the other without reading this is how it would break.
const revenuecatLifetimeHorizon = 35 * 24 * time.Hour

// RevenueCatConfig is everything the environment decides about the store provider.
//
// Deliberately separate from Config rather than fields added to it: the two providers are
// configured, enabled and disabled independently. A deployment that sells only on the web is
// a legitimate one, and so is a self-hoster who sells nothing.
type RevenueCatConfig struct {
	// APIKey is RevenueCat's SECRET key (sk_…), used server-side only.
	APIKey string
	// WebhookSecret signs incoming deliveries. See verifySignature.
	WebhookSecret string
	// Entitlement is the entitlement id that means Pro. Everything else a subscriber holds
	// is somebody else's product.
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

// Enabled reports whether the store provider can do its job: verify a delivery and read a
// subscriber's state, and know which entitlement it is looking for.
//
// The entitlement is required rather than assumed even though it has a default, for the same
// reason Stripe's price list is: without a name to match, every sync would derive "no Pro"
// and quietly downgrade every store subscriber — a failure that looks exactly like nobody
// having bought anything.
func (c RevenueCatConfig) Enabled() bool {
	return c.APIKey != "" && c.WebhookSecret != "" && c.Entitlement != ""
}

// RevenueCat is the App Store and Google Play, behind the shared engine.
//
// It has no counterpart to Stripe's checkout, portal, prices or receipts, and that absence is
// the product rather than a gap: a store subscription is bought, changed, cancelled and
// refunded inside the store, where we have no API and no business having one. What is left is
// exactly the engine — accept a delivery, record it, re-read the subscriber, write one column.
type RevenueCat struct {
	*engine
	cfg RevenueCatConfig
}

// NewRevenueCat constructs the store provider. It never fails; an unconfigured one reports
// itself disabled and refuses every operation with ErrDisabled.
func NewRevenueCat(cfg RevenueCatConfig, q *db.Queries) *RevenueCat {
	return NewRevenueCatWithBase(cfg, q, revenuecatAPIBaseURL)
}

// NewRevenueCatWithBase is NewRevenueCat pointed at a different base URL, for tests that
// stand a stub in front of it. It dials without the SSRF guard for the reason NewWithBase
// gives: the guard refuses the loopback address every stub listens on, and the production
// base URL is a constant rather than anything a caller supplies.
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

// account is one lookup and no database at all, because `app_user_id` IS our users.id.
//
// This is the single largest difference between the two providers, and the reason the seam is
// cut where it is. Stripe needs a stored customer id and two resolution routes because a
// first purchase arrives before any binding exists; here the identifier travelled outward
// from us in the first place.
// It still CONFIRMS the account exists, and that is not belt-and-braces. billing_events has a
// foreign key on user_id, so a delivery naming a numeric id we no longer have — an account
// deleted between purchase and delivery, or a sandbox app pointed at production — would fail
// the insert, answer 500, and be retried five times. After that RevenueCat may disable the
// endpoint, taking every other subscriber's deliveries with it. Returning false instead
// records the event with a NULL user, which is what "an event we could not attribute" already
// means here.
// It reads CustomerID when UserRef is empty, and that fallback is what makes the replay work.
// `applyPending` rebuilds an event from the stored row and can only fill UserRef from a
// non-NULL user_id — but a NULL user_id is precisely the row that still needs attributing.
// Without the fallback the replay resolved nobody, the worker read that as "nothing to apply"
// and STAMPED the row processed: a purchase marked done having conferred nothing, and never
// retried. For this provider both fields carry the same string, so reading either is reading
// `app_user_id`.
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

// bind is a no-op, and an honest one rather than a stub.
//
// There is no second identifier to remember: the reconciler reaches a subscriber by the same
// users.id the webhook carried. Nothing can be lost here, which is why nothing is stored.
func (p *revenuecatProvider) bind(context.Context, int64, Event) error { return nil }

// knows reports whether RevenueCat has ever been heard from about this account: a recorded
// delivery of theirs, or a store entitlement we already hold.
//
// It exists because the v1 subscribers endpoint CREATES the subscriber when the identifier is
// unknown — a read with a write's consequence. A pass that walked the user table without
// asking this first would register every account we have with the provider, permanently.
//
// An unreadable answer counts as "not known", which is the direction that cannot do damage:
// the cost is a sync that does not happen and will be retried.
func (p *revenuecatProvider) knows(ctx context.Context, userID int64) (bool, error) {
	known, err := p.q.HasRevenueCatFootprint(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("billing: looking for a RevenueCat footprint on user %d: %w", userID, err)
	}
	return known.Valid && known.Bool, nil
}

// reach reads the subscriber's current entitlements and reduces the configured one to how far
// it extends.
//
// It does not ask knows() itself. Whether an unknown account may be asked about depends on WHO
// is asking, and only the caller knows that — see engine.SyncUser and engine.SyncCaller.
func (p *revenuecatProvider) reach(ctx context.Context, userID int64) (time.Time, error) {
	body, err := p.subscriberBody(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	return revenuecatReach(body, p.cfg.Entitlement, time.Now().UTC())
}

func (p *revenuecatProvider) store(ctx context.Context, userID int64, until pgtype.Timestamptz) error {
	return p.q.SetProUntilRevenueCat(ctx, db.SetProUntilRevenueCatParams{Until: until, ID: userID})
}

// dueSoon reaches the accounts whose store entitlement is about to lapse. The candidate set is
// the accounts that hold one, which is also the set reach() is allowed to ask about.
func (p *revenuecatProvider) dueSoon(ctx context.Context, from, to time.Time, max int32) ([]int64, error) {
	return p.q.ListSubscribersNearProExpiryRevenueCat(ctx, db.ListSubscribersNearProExpiryRevenueCatParams{
		FromTime: stamp(from),
		ToTime:   stamp(to),
		MaxRows:  max,
	})
}

// subscriberBody fetches one subscriber, whole, as received.
//
// The identifier is a PATH SEGMENT and is escaped as one. It is a users.id today, so escaping
// changes nothing — which is exactly when an unescaped interpolation gets written and then
// stops being harmless the day the identifier's shape changes.
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

	// Bounded, because the body is decided by the provider and an unbounded read is a way for
	// one unlucky response to become this worker's memory ceiling. A subscriber carries a
	// handful of entitlements and their dates; a megabyte is orders of magnitude more room
	// than that needs and still small enough to be harmless.
	body, err := io.ReadAll(io.LimitReader(resp.Body, subscriberBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("billing: reading GET %s: %w", path, err)
	}
	return body, nil
}

// subscriberBodyLimit caps how much of a subscriber response is read.
const subscriberBodyLimit = 1 << 20

// revenuecatSubscriber is the reply shape, trimmed to what decides a plan. Everything else
// RevenueCat knows — products, stores, receipts, the management URL — stays with RevenueCat,
// which is its source of truth.
// The date fields are json.RawMessage rather than *string so that ABSENT and NULL stay
// distinguishable. That distinction decides money: a present-but-null expiry means a
// non-expiring entitlement and confers Pro, while an object carrying neither key is an
// entitlement we cannot read — a renamed field, a shape we do not know — and must confer
// nothing. Decoded into pointers, `{}` and `{"expires_date":null}` are the same value, and
// the first would silently grant a plan nobody bought.
type revenuecatSubscriber struct {
	Subscriber struct {
		Entitlements map[string]struct {
			ExpiresDate            json.RawMessage `json:"expires_date"`
			GracePeriodExpiresDate json.RawMessage `json:"grace_period_expires_date"`
		} `json:"entitlements"`
	} `json:"subscriber"`
}

// revenuecatReach reduces a subscriber's entitlements to the single instant
// users.pro_until_revenuecat holds: how far the configured entitlement still stands.
//
// The zero time with no error means this provider confers nothing — the entitlement is absent,
// or the subscriber holds only somebody else's product. That is NOT the same as the account
// being free: another source may still confer, which is what the derived column is for.
//
// An entitlement whose reach cannot be read returns an error and the zero time, deliberately
// together: failing closed costs a subscriber one support message, and failing open costs the
// revenue and hides that it is doing so.
func revenuecatReach(body []byte, entitlement string, now time.Time) (time.Time, error) {
	var raw revenuecatSubscriber
	if err := json.Unmarshal(body, &raw); err != nil {
		return time.Time{}, fmt.Errorf("billing: decoding the subscriber: %w", err)
	}

	ent, held := raw.Subscriber.Entitlements[entitlement]
	if !held {
		return time.Time{}, nil
	}

	// NEITHER KEY PRESENT is an entitlement we cannot read, not a lifetime one. We sell no
	// non-expiring product, so the shape that reaches the horizon below should only ever be a
	// deliberate dashboard grant — while `{}` is what a renamed field or an unfamiliar shape
	// looks like, and granting a month of Pro for it fails open in the one direction that
	// hides itself.
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

	// Present and explicitly null on both — the provider's way of saying "this does not
	// expire". See revenuecatLifetimeHorizon for why that becomes a date the reconciler
	// renews rather than a sentinel.
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

// parseRevenueCatDate reads one ISO 8601 instant, and reports whether the field was present
// and explicitly null.
//
// That second return is the whole point. An ABSENT field and a NULL one both yield the zero
// time, and only one of them means "does not expire"; collapsing them is how a shape we do not
// understand becomes a month of free Pro.
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

// parseRevenueCatEvent reads the webhook envelope: `{"api_version":…, "event":{…}}`.
//
// It requires an id and nothing else, for the reason Stripe's parser does: the id is the only
// field idempotency depends on, and demanding more would turn every event type RevenueCat
// adds into a rejected delivery. The event's own claims about entitlement — `expiration_at_ms`,
// `entitlement_ids` — are deliberately not read. A webhook says something changed; what it
// changed to is read from the provider.
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
		// Without an id there is no idempotency key, so a redelivery would be recorded and
		// applied a second time. Refusing makes the provider retry, which is the safe
		// direction to fail in.
		return Event{}, errors.New("billing: event has no id")
	}

	// CustomerID and UserRef are the same string here, and both are filled on purpose. The
	// first is what billing_events records as the subject; the second is what account()
	// resolves. For Stripe they are two different identifiers; for RevenueCat they are one,
	// and pretending otherwise would only hide that fact from whoever reads the table.
	return Event{
		ID:         fields.ID,
		Type:       fields.Type,
		CustomerID: fields.AppUserID,
		UserRef:    fields.AppUserID,
		Payload:    envelope.Event,
	}, nil
}

// refuseRedirect stops the SECRET key travelling anywhere it was not addressed to.
//
// Go's default policy follows up to ten hops and carries the Authorization header to the
// original host and its subdomains — and it permits an HTTPS request to be redirected to
// plain HTTP. safehttp re-dials every hop through the SSRF guard, which stops an internal
// address, but the guard says nothing about a PUBLIC host reading a header meant for
// api.revenuecat.com. That header is `Bearer sk_…`, the key that can grant and revoke a plan.
//
// So no hop at all. This is not a general HTTP client: it calls one documented endpoint that
// does not redirect, so a redirect here is a misconfiguration or an attack, and neither is
// worth following with a credential in hand.
func refuseRedirect(req *http.Request, _ []*http.Request) error {
	return fmt.Errorf("billing: refusing a redirect to %s — the RevenueCat key travels to one host only", req.URL.Host)
}
