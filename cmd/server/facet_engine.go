package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/search/search"
)

const (
	// facetEngineKey overrides the start mode at runtime: meili | shadow | facetd.
	// Absent means the start mode. `SET freehire:search:facet-engine meili` is the rollback.
	facetEngineKey      = "freehire:search:facet-engine"
	facetEnginePoll     = 10 * time.Second
	facetEngineGetLimit = 2 * time.Second
	facetEngineTimeout  = 150 * time.Millisecond
)

// facetModeSource holds the current facet mode for the router to read per request
// without any I/O; refresh updates it from Redis.
type facetModeSource struct {
	start   search.FacetMode
	cur     atomic.Value // search.FacetMode
	warned  atomic.Value // last invalid override already logged
	getFunc func(ctx context.Context) (string, error)
}

func newFacetModeSource(start search.FacetMode, get func(ctx context.Context) (string, error)) *facetModeSource {
	s := &facetModeSource{start: start, getFunc: get}
	s.cur.Store(start)
	return s
}

func (s *facetModeSource) Mode() search.FacetMode { return s.cur.Load().(search.FacetMode) }

// refresh applies the override. A Redis error keeps the last known mode; an absent key
// returns to the start mode; an invalid value is ignored and logged once per value.
func (s *facetModeSource) refresh(ctx context.Context) {
	v, err := s.getFunc(ctx)
	switch {
	case errors.Is(err, redis.Nil):
		s.cur.Store(s.start)
	case err != nil:
		slog.Warn("facet engine: reading the mode override failed, keeping the last mode", "err", err)
	default:
		m, perr := search.ParseFacetMode(v)
		if perr != nil {
			if s.warned.Load() != v {
				s.warned.Store(v)
				slog.Warn("facet engine: ignoring invalid override", "key", facetEngineKey, "err", perr)
			}
			return
		}
		s.cur.Store(m)
	}
}

func (s *facetModeSource) run(ctx context.Context) {
	t := time.NewTicker(facetEnginePoll)
	defer t.Stop()
	s.refresh(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refresh(ctx)
		}
	}
}

// facetEngineOptions reads FACETD_URL, FACETD_TIMEOUT and SEARCH_FACET_ENGINE. Without
// FACETD_URL it returns nothing: the router is not built and Meilisearch answers everything.
func facetEngineOptions(ctx context.Context, rdb *redis.Client) ([]search.Option, error) {
	url := os.Getenv("FACETD_URL")
	if url == "" {
		return nil, nil
	}
	start := search.FacetModeMeili
	if v := os.Getenv("SEARCH_FACET_ENGINE"); v != "" {
		m, err := search.ParseFacetMode(v)
		if err != nil {
			return nil, fmt.Errorf("SEARCH_FACET_ENGINE: %w", err)
		}
		start = m
	}
	timeout := facetEngineTimeout
	if v := os.Getenv("FACETD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("FACETD_TIMEOUT %q: want a positive duration like 150ms", v)
		}
		timeout = d
	}
	src := newFacetModeSource(start, func(ctx context.Context) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, facetEngineGetLimit)
		defer cancel()
		return rdb.Get(ctx, facetEngineKey).Result()
	})
	go src.run(ctx)
	// No client-wide timeout: the router bounds each call with a context.
	hc := &http.Client{Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: time.Second}).DialContext,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
	}}
	slog.Info("facet engine configured", "url", url, "start_mode", start, "timeout", timeout)
	return []search.Option{
		search.WithFacetEngine(search.NewFacetdClient(url, hc), src.Mode),
		search.WithFacetEngineTimeout(timeout),
	}, nil
}
