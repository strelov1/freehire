// Command linkedin-token-refresh keeps the daily digest's LinkedIn credential alive. Schedule
// it once a day.
//
// LinkedIn access tokens last 60 days. When the application is entitled to a programmatic refresh token this worker exchanges it for a fresh one a fortnight before expiry. When it is not (the expected case outside the Marketing Developer Platform) it cannot renew anything and instead warns while a fortnight remains.
//
// The warning is not a red unit: a non-zero exit repeated daily teaches an operator to ignore it. It publishes a gauge of seconds left on the credential for an alert rule to read. A non-zero exit is reserved for a credential that is ALREADY dead.
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

// textfileName is the node_exporter collector file this worker owns. One file per worker, so a failed publish cannot blank another worker's series.
const textfileName = "freehire_social_token.prom"

func main() { worker.Main(run) }

func run() int {
	// Gated before Bootstrap: an unconfigured deployment has no credential to renew. The predicate is config's, so this worker and cmd/social-digest agree on whether the channel exists.
	cfg := config.Load()
	if !cfg.LinkedInDigestConfigured() {
		log.Print("linkedin-token-refresh: the LinkedIn channel is not configured, nothing to renew")
		return worker.Skip("LINKEDIN_DIGEST")
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
		return worker.Skip("LINKEDIN_CREDENTIAL")
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

// publish writes the gauges an alert rule reads. Best-effort: a failed publish must not fail a renewal that already happened, and the next run overwrites the file.
func publish(o linkedinauth.Outcome) {
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		return
	}
	if err := worker.WriteTextfile(dir, textfileName, render(o, time.Now().UTC())); err != nil {
		log.Printf("linkedin-token-refresh: %v", err)
	}
}

// render is the published contract. It emits seconds remaining rather than an expiry timestamp, so an alert rule needs no clock arithmetic.
//
// A missing credential publishes a NEGATIVE remaining time: nothing at all would look like a broken exporter, and zero like a credential that expired this instant.
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
