//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// The verification stack substitutes only the external board response; normalization,
// persistence and search enqueueing use the real ingest path.
type verificationBoard struct{}

func (verificationBoard) GetJSON(_ context.Context, _ string, dst any) error {
	jobs := make([]map[string]any, 0, 3)
	for i, title := range []string{"Backend Engineer", "Frontend Engineer", "Product Designer"} {
		jobs = append(jobs, map[string]any{
			"id": i + 1, "title": title,
			"absolute_url": fmt.Sprintf("https://boards.greenhouse.io/freehire-verification/jobs/%d", i+1),
			"updated_at":   time.Now().UTC().Format(time.RFC3339),
			"location":     map[string]string{"name": "Berlin, Germany"},
			"content":      fmt.Sprintf("<p>Join our team as a %s. Build accessible software products with Go, TypeScript and PostgreSQL. Work with product colleagues to design reliable systems and improve the customer experience.</p>", title),
		})
	}
	body, err := json.Marshal(map[string]any{"jobs": jobs})
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func TestVerificationSeed(t *testing.T) {
	dsn := os.Getenv("VERIFY_SEED_DATABASE_URL")
	if dsn == "" {
		t.Skip("only run by the isolated verification stack")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || !strings.HasPrefix(u.Path, "/verify_") {
		t.Fatal("verification seeding requires a loopback verify_ database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	runner := pipeline.Runner{
		Registry: map[string]sources.Source{"greenhouse": sources.NewGreenhouse(verificationBoard{})},
		Store:    newDBStore(pool, 1, nil, nil, defaultSeenPolicy()),
	}
	stats, err := runner.Run(ctx, []sources.CompanyEntry{{Provider: "greenhouse", Board: "freehire-verification", Company: "Freehire Verification"}})
	if err != nil || stats.Total().Ingested != 3 {
		t.Fatalf("seed: stats=%+v error=%v", stats.Total(), err)
	}
}
