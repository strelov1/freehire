// Command linkedin-auth grants, records and inspects the credential the daily digest posts to
// LinkedIn with. Run by hand, by an operator, roughly once every sixty days — or once a year
// if LinkedIn issued this application a refresh token.
//
//	./linkedin-auth -status              # what is stored, and when it dies
//	./linkedin-auth -url                 # print the address to open in a browser
//	./linkedin-auth -code AQT...         # exchange the code that browser handed back
//	./linkedin-auth -token AQV... -expires-in 5184000
//	                                     # store a token minted in the Developer Portal instead
//
// WHY THIS IS A COMMAND AND NOT A ROUTE. The sign-in is one person, on one host, a handful of
// times a year. A route would mean a callback handler, a state cookie, an admin authorization
// rule and a page — a login flow for an audience of one — and it would put the ability to
// re-point our company page's publisher behind whatever that route's authorization turned out
// to be. A command needs SSH to the production host, which is the authorization we already have.
//
// The -token form exists because the LinkedIn Developer Portal has its own token generator,
// which needs no redirect URL at all. That is the shortest path to a first token, and the flow
// it skips (-url then -code) is the one that also yields a refresh token when the application
// is entitled to one.
//
// Needs DATABASE_URL and the four LINKEDIN_* values.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/engage/linkedinauth"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

var (
	showURL   = flag.Bool("url", false, "print the authorization URL to open in a browser")
	code      = flag.String("code", "", "exchange this authorization code for a token and store it")
	rawToken  = flag.String("token", "", "store this access token directly (from the Developer Portal token generator)")
	expiresIn = flag.Int64("expires-in", 0, "seconds the -token value remains valid (defaults to LinkedIn's 60 days)")
	status    = flag.Bool("status", false, "report the stored credential and when it expires")
)

// defaultExpiresIn is LinkedIn's documented access-token lifetime, used when -token is stored
// without an explicit one. A guess, and deliberately not a longer one: a stored expiry that
// overshoots the real one turns the renewal warning into a warning that arrives after the
// channel has already stopped.
const defaultExpiresIn = 60 * 24 * 60 * 60

func main() {
	flag.Parse()
	worker.Main(run)
}

func run() int {
	cfg := config.Load()
	if !cfg.LinkedInDigestConfigured() {
		log.Print("linkedin-auth: LINKEDIN_CLIENT_ID, LINKEDIN_CLIENT_SECRET, LINKEDIN_REDIRECT_URI " +
			"and LINKEDIN_ORGANIZATION_ID must all be set")
		return 1
	}
	creds := linkedinauth.Credentials{
		ClientID:     cfg.LinkedInClientID,
		ClientSecret: cfg.LinkedInClientSecret,
		RedirectURI:  cfg.LinkedInRedirectURI,
	}

	// -url is answered before the database is opened. It reads nothing and writes nothing, and
	// an operator printing the sign-in address on a laptop should not need DATABASE_URL.
	if *showURL {
		fmt.Println(creds.AuthorizeURL(time.Now().UTC().Format("20060102150405")))
		fmt.Println()
		fmt.Println("Open that address as a LinkedIn member who administers the company page.")
		fmt.Println("You will be redirected to " + cfg.LinkedInRedirectURI + "?code=...")
		fmt.Println("Copy the code out of the address bar and run: linkedin-auth -code <that value>")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	store := linkedinauth.NewPostgresStore(db.New(pool))

	switch {
	case *status:
		return reportStatus(ctx, store)
	case *code != "":
		return exchange(ctx, creds, store, *code)
	case *rawToken != "":
		return storeRaw(ctx, store, *rawToken, *expiresIn)
	}

	log.Print("linkedin-auth: nothing to do — pass -status, -url, -code or -token (see -help)")
	return 1
}

// exchange turns an authorization code into a stored credential, and says plainly whether a
// refresh token came with it. That one line is the answer to the question this whole feature's
// design hangs on — whether the token can be renewed by a worker or must be re-granted by a
// person — and it is knowable only from this response.
func exchange(ctx context.Context, creds linkedinauth.Credentials, store linkedinauth.Store, code string) int {
	tok, err := creds.Exchange(ctx, nil, code)
	if err != nil {
		log.Printf("linkedin-auth: %v", err)
		return 1
	}
	if err := store.Save(ctx, tok); err != nil {
		log.Printf("linkedin-auth: store credential: %v", err)
		return 1
	}
	log.Printf("linkedin-auth: stored a credential expiring %s, scope %q",
		tok.ExpiresAt.Format(time.RFC3339), tok.Scope)
	if tok.Renewable() {
		log.Printf("linkedin-auth: a refresh token WAS issued (valid until %s) — "+
			"cmd/linkedin-token-refresh will renew this without you",
			tok.RefreshExpiresAt.Format(time.RFC3339))
	} else {
		log.Print("linkedin-auth: NO refresh token was issued, which is expected outside the " +
			"Marketing Developer Platform — this must be repeated before it expires, and " +
			"cmd/linkedin-token-refresh will warn a fortnight ahead")
	}
	return 0
}

// storeRaw records a token minted elsewhere. Its expiry is not knowable from the token itself,
// so it is taken from the flag and defaulted rather than inferred — the alternative is storing
// no expiry, which would disable every warning this feature has.
func storeRaw(ctx context.Context, store linkedinauth.Store, token string, seconds int64) int {
	if seconds <= 0 {
		seconds = defaultExpiresIn
	}
	tok := linkedinauth.Token{
		AccessToken: token,
		ExpiresAt:   time.Now().UTC().Add(time.Duration(seconds) * time.Second),
		Scope:       linkedinauth.Scope,
	}
	if err := store.Save(ctx, tok); err != nil {
		log.Printf("linkedin-auth: store credential: %v", err)
		return 1
	}
	log.Printf("linkedin-auth: stored a credential expiring %s, with no refresh token — "+
		"it must be replaced by hand before then", tok.ExpiresAt.Format(time.RFC3339))
	return 0
}

// reportStatus prints what is stored WITHOUT printing the credential. An operator needs the
// dates and whether a renewal is possible; the token itself would only ever be read into a
// shell history or a screenshot.
func reportStatus(ctx context.Context, store linkedinauth.Store) int {
	tok, ok, err := store.Load(ctx)
	if err != nil {
		log.Printf("linkedin-auth: %v", err)
		return 1
	}
	if !ok {
		log.Print("linkedin-auth: no credential stored — the LinkedIn channel is not signed in")
		return 0
	}
	left := time.Until(tok.ExpiresAt).Round(time.Hour)
	log.Printf("linkedin-auth: access token expires %s (%s from now), scope %q",
		tok.ExpiresAt.Format(time.RFC3339), left, tok.Scope)
	if tok.Renewable() {
		log.Printf("linkedin-auth: renewable — refresh token expires %s",
			tok.RefreshExpiresAt.Format(time.RFC3339))
	} else {
		log.Print("linkedin-auth: NOT renewable — no refresh token; a person must sign in again before expiry")
	}
	return 0
}
