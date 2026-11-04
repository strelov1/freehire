//go:build integration

package accounts_test

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func registrations(t *testing.T) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "freehire_registrations_total" {
			return family.Metric[0].GetCounter().GetValue()
		}
	}
	return 0
}

func TestRegistrationMetricsExcludeOAuthLinksAndRolledBackCreates(t *testing.T) {
	pool := startPostgres(t)
	repo := oauthRepo(pool)
	ctx := context.Background()
	before := registrations(t)
	if _, err := repo.CreateUser(ctx, "password@example.test", "hash", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateUser(ctx, "password@example.test", "hash", false, nil); err == nil {
		t.Fatal("duplicate registration succeeded")
	}
	if _, err := repo.LinkOrCreateByEmail(ctx, "google", "identity-one", "oauth@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LinkOrCreateByEmail(ctx, "github", "identity-two", "oauth@example.test"); err != nil {
		t.Fatal(err)
	}
	// This creates a user inside the transaction, then hits an identity conflict.
	if _, err := repo.LinkOrCreateByEmail(ctx, "google", "identity-one", "rollback@example.test"); err == nil {
		t.Fatal("conflicting identity succeeded")
	}
	if got := registrations(t) - before; got != 2 {
		t.Fatalf("registrations = %v, want exactly two committed new accounts", got)
	}
}
