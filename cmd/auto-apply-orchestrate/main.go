// Command auto-apply-orchestrate serves the durable tailor-then-review Inngest function
// (internal/application/autoapplyorchestrate) that sequences one auto-apply queue entry's
// tailoring call and its candidate-review call, pausing between them for however long the
// candidate's decision takes — including across a restart of this very process. Unlike the
// run-once cron workers it is long-lived (an Inngest function must stay reachable between
// steps for the SDK's own callback protocol), so it serves HTTP until SIGTERM — mirrors
// cmd/mail-ingest's own shape.
//
// It needs no database: every call it makes goes out over plain HTTP to freehire's own API
// (internal/api/handler's two auto-apply routes) — see
// openspec/changes/auto-apply-inngest-orchestration/design.md on why not
// internal/platform/safehttp, and why not internal/candidate/cv or internal/ai/assistant
// directly.
//
// AUTO_APPLY_ORCHESTRATOR_SECRET / INNGEST_EVENT_KEY / INNGEST_SIGNING_KEY have no useful
// degraded mode here (unlike PostAutoApplyReview's own best-effort event publish): this
// worker either runs against a real, self-hosted Inngest instance or is not deployed, so a
// missing value fails startup loudly rather than serving a function that could never
// actually authenticate anywhere.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/inngest/inngestgo"

	"github.com/strelov1/freehire/internal/application/autoapplyorchestrate"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

// selfRegisterTimeout bounds the request that triggers this worker's own sync with the
// self-hosted Inngest server at startup — a local call between two systemd units on the
// same host, not a network hop that needs generosity.
const selfRegisterTimeout = 10 * time.Second

func run() int {
	cfg := config.Load()

	if cfg.AutoApplyOrchestratorSecret == "" || cfg.InngestEventKey == "" || cfg.InngestSigningKey == "" || cfg.InngestEventAPIURL == "" {
		log.Print("auto-apply-orchestrate: AUTO_APPLY_ORCHESTRATOR_SECRET / INNGEST_EVENT_API_URL / INNGEST_EVENT_KEY / INNGEST_SIGNING_KEY must all be set — nothing to serve")
		return 1
	}
	hireBaseURL := os.Getenv("HIRE_BASE_URL")
	if hireBaseURL == "" {
		hireBaseURL = "http://127.0.0.1:" + cfg.Port + "/api/v1"
	}

	registerURL := cfg.InngestEventAPIURL + "/fn/register"
	client, err := inngestgo.NewClient(inngestgo.ClientOpts{
		AppID:           "freehire-auto-apply-orchestrator",
		EventKey:        &cfg.InngestEventKey,
		SigningKey:      &cfg.InngestSigningKey,
		APIBaseURL:      &cfg.InngestEventAPIURL,
		EventAPIBaseURL: &cfg.InngestEventAPIURL,
		RegisterURL:     &registerURL,
	})
	if err != nil {
		log.Printf("auto-apply-orchestrate: new inngest client: %v", err)
		return 1
	}
	if _, err := autoapplyorchestrate.Register(client, autoapplyorchestrate.Config{
		HireBaseURL: hireBaseURL,
		Secret:      cfg.AutoApplyOrchestratorSecret,
	}); err != nil {
		log.Printf("auto-apply-orchestrate: register function: %v", err)
		return 1
	}

	mux := http.NewServeMux()
	// EnableUnauthedSync: this deployment is one internal, single-tenant, self-hosted
	// Inngest instance (never reachable from the public internet, per design.md) — the
	// restriction unauthed-sync guards against (an arbitrary caller registering functions
	// on someone else's Inngest Cloud account) does not apply here.
	mux.Handle("/api/inngest", client.ServeWithOpts(inngestgo.ServeOpts{
		EnableUnauthedSync: inngestgo.BoolPtr(true),
	}))

	addr := ":" + cfg.AutoApplyOrchestratePort
	srv := &http.Server{Addr: addr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	if err := selfRegister(addr); err != nil {
		log.Printf("auto-apply-orchestrate: self-registration with the Inngest server: %v", err)
		return 1
	}
	log.Printf("auto-apply-orchestrate: serving on %s, registered with %s", addr, cfg.InngestEventAPIURL)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("auto-apply-orchestrate: shutdown: %v", err)
			return 1
		}
		return 0
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("auto-apply-orchestrate: serve: %v", err)
			return 1
		}
		return 0
	}
}

// selfRegister triggers this worker's own sync with the Inngest server: a PUT to its own
// /api/inngest endpoint, which the SDK answers by pushing its function list out of band —
// the same handshake this package's own integration tests (and this session's earlier
// spike) verified against a real Inngest dev server. It retries briefly since the HTTP
// server above has just started serving in its own goroutine.
func selfRegister(addr string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, "http://127.0.0.1"+addr+"/api/inngest", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: selfRegisterTimeout}

	deadline := time.Now().Add(selfRegisterTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("self-registration PUT: status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}
