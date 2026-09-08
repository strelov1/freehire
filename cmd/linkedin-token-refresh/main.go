// Command linkedin-token-refresh keeps the daily digest's LinkedIn credential alive. Schedule
// it once a day.
//
// LinkedIn access tokens last 60 days. When the application is entitled to a programmatic
// refresh token this worker exchanges it for a fresh one a fortnight before expiry and nobody
// notices; when it is not — the expected case outside the Marketing Developer Platform — it
// cannot renew anything, and its job is instead to say so while there is still a fortnight to
// act. Both paths matter: a channel that stops publishing because its credential quietly died
// is indistinguishable from a quiet day.
//
// HOW THE WARNING IS SEEN. Not through a red unit: this fleet already runs ~37 permanently
// failing ingest units, so a non-zero exit repeated daily for two weeks teaches an operator to
// ignore exactly the thing it is trying to say. It publishes a gauge instead — the seconds
// left on the credential — which an alert rule can read and which describes the situation
// continuously rather than as a boolean. A non-zero exit is reserved for the credential being
// ALREADY dead, where the channel is broken now rather than soon.
//
// Needs DATABASE_URL and the four LINKEDIN_* values; without them it is a no-op that never
// opens the pool, which is how this ships before the channel is configured. PROM_TEXTFILE_DIR
// is optional: unset, the run still renews and still logs, it simply publishes nothing.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/engage/linkedinauth"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// textfileName is the node_exporter collector file this worker owns. One file per worker, so a
// run that fails to publish cannot blank another worker's series.
const textfileName = "freehire_social_token.prom"

func main() { worker.Main(run) }

func run() int {
	// Gated before Bootstrap: an unconfigured deployment has no credential to renew and must
	// not open a pool to discover that. The predicate is config's, so this worker and
	// cmd/social-digest cannot disagree about whether the channel exists.
	cfg := config.Load()
	if !cfg.LinkedInDigestConfigured() {
		log.Print("linkedin-token-refresh: the LinkedIn channel is not configured, nothing to renew")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	store := linkedinauth.NewPostgresStore(db.New(pool))
	renewer := linkedinauth.NewRenewer(linkedinauth.Credentials{
		ClientID:     cfg.LinkedInClientID,
		ClientSecret: cfg.LinkedInClientSecret,
		RedirectURI:  cfg.LinkedInRedirectURI,
	}, store, nil, time.Now)

	outcome, err := renewer.Run(ctx)
	if err != nil {
		log.Printf("linkedin-token-refresh: %v", err)
		return 1
	}

	publish(outcome)

	switch outcome.State {
	case linkedinauth.StateMissing:
		log.Print("linkedin-token-refresh: no credential stored — run cmd/linkedin-auth to sign in")
		return 0
	case linkedinauth.StateRenewed:
		log.Printf("linkedin-token-refresh: renewed; the access token now expires %s",
			outcome.ExpiresAt.Format(time.RFC3339))
		return 0
	case linkedinauth.StateWarn:
		log.Printf("linkedin-token-refresh: ATTENTION: %s", outcome.Reason)
		return 0
	case linkedinauth.StateExpired:
		log.Printf("linkedin-token-refresh: %s", outcome.Reason)
		return 1
	default:
		log.Printf("linkedin-token-refresh: healthy; the access token expires %s",
			outcome.ExpiresAt.Format(time.RFC3339))
		return 0
	}
}

// publish writes the gauges an alert rule reads. Best-effort by design: failing to publish a
// measurement must not fail the renewal that already happened, and the next run overwrites the
// file wholesale anyway.
func publish(o linkedinauth.Outcome) {
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		return
	}
	if err := worker.WriteTextfile(dir, textfileName, render(o, time.Now().UTC())); err != nil {
		log.Printf("linkedin-token-refresh: %v", err)
	}
}

// render is the published contract. Seconds-remaining rather than an expiry timestamp, because
// an alert asks "how long have we got" and a rule written against an absolute time has to
// subtract the clock itself — in a query language where that is easy to get wrong once and
// then trust forever.
//
// A missing credential publishes a NEGATIVE remaining time rather than zero or nothing at all.
// Nothing at all would make an unsigned-in channel look identical to a broken exporter, and
// zero would look like a credential that expired this instant. Both hide the state that
// actually needs a person.
func render(o linkedinauth.Outcome, now time.Time) string {
	var b strings.Builder
	b.WriteString("# HELP freehire_social_token_expires_in_seconds Seconds until a social channel's stored access token expires; negative when none is stored.\n")
	b.WriteString("# TYPE freehire_social_token_expires_in_seconds gauge\n")
	fmt.Fprintf(&b, "freehire_social_token_expires_in_seconds{channel=%q} %.0f\n",
		linkedinauth.Channel, secondsUntil(o.ExpiresAt, now))

	b.WriteString("# HELP freehire_social_token_grant_expires_in_seconds Seconds until the grant behind the token runs out, after which a person must sign in again; negative when there is no refresh token.\n")
	b.WriteString("# TYPE freehire_social_token_grant_expires_in_seconds gauge\n")
	fmt.Fprintf(&b, "freehire_social_token_grant_expires_in_seconds{channel=%q} %.0f\n",
		linkedinauth.Channel, secondsUntil(o.RefreshExpiresAt, now))

	b.WriteString("# HELP freehire_social_token_renewable Whether the stored credential can be renewed without a person.\n")
	b.WriteString("# TYPE freehire_social_token_renewable gauge\n")
	fmt.Fprintf(&b, "freehire_social_token_renewable{channel=%q} %d\n",
		linkedinauth.Channel, boolGauge(!o.RefreshExpiresAt.IsZero()))

	return b.String()
}

// secondsUntil is the remaining life of a deadline, and -1 for a deadline that does not exist.
func secondsUntil(deadline, now time.Time) float64 {
	if deadline.IsZero() {
		return -1
	}
	return deadline.Sub(now).Seconds()
}

func boolGauge(b bool) int {
	if b {
		return 1
	}
	return 0
}
