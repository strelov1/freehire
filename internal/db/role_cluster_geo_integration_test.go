//go:build integration

// Integration tests for RoleClusterGeo: the per-cluster counterpart of the
// whole-catalogue RoleClusterGeoAll, asked by the incremental index writers so a push
// widens a collapsed canon instead of replacing the reindex's union with the canon's own
// narrow geography. The union, the open-rows-only scope and the empty-value filtering are
// SQL behaviours verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// clusterRow is one posting of a shared role, in its own geography.
func clusterRow(externalID, fingerprint string, countries, regions, cities []string) UpsertJobParams {
	p := withFingerprint(externalID, "Senior Full Stack Engineer", fingerprint)
	p.Countries = countries
	p.Regions = regions
	p.Cities = cities
	return p
}

// fpText matches the generated param type, which mirrors the jobs.role_fingerprint column.
func fpText(fp string) pgtype.Text {
	return pgtype.Text{String: fp, Valid: true}
}

func TestRoleClusterGeo_UnionsTheOpenRowsOfOneCluster(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	const fp = "fp-fullstack"
	rows := []UpsertJobParams{
		clusterRow("c-1", fp, []string{"de"}, []string{"eu"}, []string{"Düsseldorf"}),
		clusterRow("c-2", fp, []string{"pl"}, []string{"eu"}, []string{"Kraków"}),
		clusterRow("c-3", fp, []string{"at"}, []string{"eu"}, []string{"Wien"}),
		// A different role in the same company must not bleed into the union.
		clusterRow("other-1", "fp-backend", []string{"us"}, []string{"north_america"}, []string{"Austin"}),
	}
	for _, p := range rows {
		if _, err := q.UpsertJob(ctx, p); err != nil {
			t.Fatalf("upsert %s: %v", p.ExternalID, err)
		}
	}

	geo, err := q.RoleClusterGeo(ctx, RoleClusterGeoParams{CompanySlug: "acme", RoleFingerprint: fpText(fp)})
	if err != nil {
		t.Fatalf("RoleClusterGeo: %v", err)
	}
	if want := []string{"at", "de", "pl"}; !slices.Equal(geo.Countries, want) {
		t.Errorf("countries = %v, want %v", geo.Countries, want)
	}
	if want := []string{"eu"}; !slices.Equal(geo.Regions, want) {
		t.Errorf("regions = %v, want %v", geo.Regions, want)
	}
	if want := []string{"Düsseldorf", "Kraków", "Wien"}; !slices.Equal(geo.Cities, want) {
		t.Errorf("cities = %v, want %v", geo.Cities, want)
	}
}

func TestRoleClusterGeo_ExcludesClosedRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	const fp = "fp-closed"
	open := clusterRow("open-1", fp, []string{"de"}, []string{"eu"}, []string{"Düsseldorf"})
	closed := clusterRow("closed-1", fp, []string{"pl"}, []string{"eu"}, []string{"Kraków"})
	for _, p := range []UpsertJobParams{open, closed} {
		if _, err := q.UpsertJob(ctx, p); err != nil {
			t.Fatalf("upsert %s: %v", p.ExternalID, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now() WHERE external_id = $1`, closed.ExternalID); err != nil {
		t.Fatalf("close row: %v", err)
	}

	geo, err := q.RoleClusterGeo(ctx, RoleClusterGeoParams{CompanySlug: "acme", RoleFingerprint: fpText(fp)})
	if err != nil {
		t.Fatalf("RoleClusterGeo: %v", err)
	}
	// Only the open row contributes, so merging this back is a self-union no-op — the
	// closed row's Kraków must not resurrect a city the role is no longer open in.
	if want := []string{"de"}; !slices.Equal(geo.Countries, want) {
		t.Errorf("countries = %v, want %v (the closed row must not contribute)", geo.Countries, want)
	}
	if want := []string{"Düsseldorf"}; !slices.Equal(geo.Cities, want) {
		t.Errorf("cities = %v, want %v (the closed row must not contribute)", geo.Cities, want)
	}
}

func TestRoleClusterGeo_SingletonReturnsItsOwnGeographyAndUnknownIsEmpty(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := q.UpsertJob(ctx, clusterRow("solo-1", "fp-solo",
		[]string{"de"}, []string{"eu"}, []string{"Düsseldorf"})); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// A singleton answers with its own geography — a self-union, so merging it changes
	// nothing. Callers skip the query for a singleton anyway; what matters here is that the
	// query always answers, so no caller has to tell "no cluster" apart from a failure.
	t.Run("singleton returns its own geography", func(t *testing.T) {
		geo, err := q.RoleClusterGeo(ctx, RoleClusterGeoParams{CompanySlug: "acme", RoleFingerprint: fpText("fp-solo")})
		if err != nil {
			t.Fatalf("RoleClusterGeo: %v", err)
		}
		if want := []string{"de"}; !slices.Equal(geo.Countries, want) {
			t.Errorf("countries = %v, want %v", geo.Countries, want)
		}
	})

	for _, tc := range []struct{ name, slug, fp string }{
		{"unknown fingerprint", "acme", "fp-nothing-here"},
		{"unknown company", "nobody", "fp-solo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			geo, err := q.RoleClusterGeo(ctx, RoleClusterGeoParams{CompanySlug: tc.slug, RoleFingerprint: fpText(tc.fp)})
			if err != nil {
				t.Fatalf("RoleClusterGeo must answer even for an unknown cluster: %v", err)
			}
			if len(geo.Countries) != 0 || len(geo.Regions) != 0 || len(geo.Cities) != 0 {
				t.Errorf("expected an empty union, got countries=%v regions=%v cities=%v",
					geo.Countries, geo.Regions, geo.Cities)
			}
		})
	}
}

// A fingerprint that never clusters must not be answered by accident.
func TestRoleClusterGeo_EmptyFingerprintNeverClusters(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	for _, id := range []string{"blank-1", "blank-2"} {
		p := clusterRow(id, "", []string{"de"}, []string{"eu"}, []string{"Düsseldorf"})
		p.RoleFingerprint = pgtype.Text{}
		if _, err := q.UpsertJob(ctx, p); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}

	geo, err := q.RoleClusterGeo(ctx, RoleClusterGeoParams{CompanySlug: "acme", RoleFingerprint: fpText("")})
	if err != nil {
		t.Fatalf("RoleClusterGeo: %v", err)
	}
	if len(geo.Countries) != 0 || len(geo.Cities) != 0 {
		t.Errorf("unfingerprinted rows clustered: countries=%v cities=%v", geo.Countries, geo.Cities)
	}
}
