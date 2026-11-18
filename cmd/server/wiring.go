package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/api/handler"
	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/discordlink"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
	"github.com/strelov1/freehire/internal/search/suggest"
)

// buildDiscordLinker adapts discordlink.NewFromSettings to what the handler config takes.
// It returns the INTERFACE, not *discordlink.Service: a nil *Service in an interface field is
// not a nil interface, so the handler's "unconfigured means unmounted" check would mount the
// routes onto a service that panics on first use.
func buildDiscordLinker(cfg config.Settings, pool *pgxpool.Pool) handler.DiscordLinker {
	svc, ok := discordlink.NewFromSettings(cfg, db.New(pool))
	if !ok {
		return nil
	}
	return svc
}

// buildGmail wires the Connect-Gmail inbox from config: it needs the Google OAuth
// client (reused from sign-in) and the 32-byte token-encryption key. Any piece
// missing returns (nil, nil) — the feature stays off and the server runs unchanged.
func buildGmail(cfg config.Settings) (*gmailsync.Connector, *tokencrypt.Cipher) {
	g := cfg.OAuth["google"]
	if g.ClientID == "" || g.ClientSecret == "" || len(cfg.GmailTokenKey) != 32 {
		return nil, nil
	}
	cipher, err := tokencrypt.New(cfg.GmailTokenKey)
	if err != nil {
		// A configured-but-broken key must not silently disable the feature.
		log.Printf("gmail: token cipher init failed, Connect-Gmail disabled: %v", err)
		return nil, nil
	}
	return gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin), cipher
}

// startSuggestRefresh keeps the recognition set current in the background. The first load
// runs in the goroutine so an unreachable or not-yet-built dictionary never blocks boot; until
// it lands every query is one fragment (plain completion).
func startSuggestRefresh(ctx context.Context, svc *suggest.Service) {
	go func() {
		t := time.NewTicker(suggestRefreshInterval)
		defer t.Stop()
		for {
			if err := svc.Refresh(ctx); err != nil {
				log.Printf("suggest: refresh: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// loadOJCPSigner reads the manifest signing key. The signature's iss is the host the
// manifest is served from, which an agent compares against the domain it fetched it from.
func loadOJCPSigner(path, frontendOrigin string) (*ojcp.Signer, error) {
	origin, err := url.Parse(frontendOrigin)
	if err != nil || origin.Host == "" {
		return nil, fmt.Errorf("FRONTEND_ORIGIN %q names no host for the signature's iss", frontendOrigin)
	}
	key, err := os.ReadFile(path) //nolint:gosec // the operator-configured key path
	if err != nil {
		return nil, err
	}
	return ojcp.NewSigner(key, origin.Host)
}
