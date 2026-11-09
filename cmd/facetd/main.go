// Command facetd is the in-memory facet engine that answers facet counts, totals and
// distinct counts beside Meilisearch (openspec/changes/facetd). Unlike the run-once
// workers it is long-lived (Restart=always), serves HTTP on localhost, and needs no
// DATABASE_URL: it is fed by search-drain (/write) and by `reindex -facetd-only` or
// FACETD_URL (/rebuild/*).
//
// Env: FACETD_ADDR (default 127.0.0.1:8095), FACETD_SNAPSHOT_PATH (empty disables
// snapshots), FACETD_SNAPSHOT_INTERVAL (default 5m), FACETD_WAL_PATH (default
// FACETD_SNAPSHOT_PATH + ".wal"; no snapshot path means no log). Without a loadable snapshot it
// starts not-ready (503) and the API falls back to Meilisearch until a rebuild commits.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() { os.Exit(run()) }

func run() int {
	addr := envOr("FACETD_ADDR", "127.0.0.1:8095")
	interval := 5 * time.Minute
	if v := os.Getenv("FACETD_SNAPSHOT_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			log.Printf("facetd: bad FACETD_SNAPSHOT_INTERVAL %q", v)
			return 1
		}
		interval = d
	}
	srv := NewServer(os.Getenv("FACETD_SNAPSHOT_PATH"))
	if v := os.Getenv("FACETD_WAL_PATH"); v != "" && srv.snapshotPath != "" {
		srv.walPath = v
	}
	if err := srv.LoadSnapshot(); err != nil {
		log.Printf("facetd: NOT READY: no usable snapshot (%v); answering 503 until a rebuild commits", err)
	} else {
		log.Print("facetd: snapshot loaded, ready")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := srv.Snapshot(true); err != nil {
					log.Printf("facetd: periodic snapshot: %v", err)
				}
			}
		}
	}()
	log.Printf("facetd: listening on %s (no auth: keep it on localhost)", addr)

	code := 0
	select {
	case err := <-errc:
		log.Printf("facetd: %v", err)
		code = 1
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("facetd: shutdown: %v", err)
	}
	if err := srv.Snapshot(true); err != nil {
		log.Printf("facetd: final snapshot: %v", err)
		code = 1
	}
	return code
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
