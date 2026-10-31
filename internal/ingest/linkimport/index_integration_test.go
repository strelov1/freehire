//go:build integration

package linkimport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// Fail only the closure read; the preceding cluster and vector reads remain real SQL.
type closureFailureDB struct {
	db.DBTX
	fail   bool
	failed bool
}

func (d *closureFailureDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.fail && strings.Contains(sql, "-- name: DuplicateClosureGeoFor :many") {
		d.failed = true
		return nil, errors.New("closure read unavailable")
	}
	return d.DBTX.Query(ctx, sql, args...)
}

func TestIndex_ClosureReadFailureKeepsIndexedGeography(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, description, is_tech, countries)
		VALUES ('greenhouse', 'acme:closure', 'https://acme.test/jobs/closure', 'Go Engineer', 'Acme', 'acme', 'go-engineer-acme',
		        'Build distributed Go services.', true, ARRAY['US']) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, duplicate_of_role, countries)
		VALUES ('greenhouse', 'acme:duplicate', 'https://acme.test/jobs/duplicate', 'Go Engineer', 'go-engineer-acme-duplicate', $1, ARRAY['CA'])`, id); err != nil {
		t.Fatal(err)
	}
	row, err := db.New(pool).GetJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	pushes := make(chan []search.JobDocument, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var docs []search.JobDocument
		if err := json.NewDecoder(r.Body).Decode(&docs); err != nil {
			t.Errorf("decode index push: %v", err)
		}
		pushes <- docs
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"taskUid":1,"status":"enqueued","type":"documentAdditionOrUpdate"}`))
	}))
	defer srv.Close()
	store := &closureFailureDB{DBTX: pool}
	im := &Importer{q: db.New(store), idx: search.NewClient(srv.URL, "test-key")}
	saved := db.UpsertJobRow{Job: row}

	// The ordinary push must preserve the duplicate's country, establishing that this
	// row is indexable and that a narrow replacement would lose real indexed coverage.
	im.index(ctx, saved)
	if len(pushes) != 1 {
		t.Fatalf("initial pushes = %d, want 1", len(pushes))
	}
	docs := <-pushes
	if len(docs) != 1 || !slices.Contains(docs[0].Countries, "US") || !slices.Contains(docs[0].Countries, "CA") {
		t.Fatalf("initial push did not include both countries: %+v", docs)
	}

	store.fail = true
	im.index(ctx, saved)
	if !store.failed {
		t.Fatal("did not reach the failing closure query")
	}
	if len(pushes) != 0 {
		t.Fatalf("closure failure submitted %d replacement(s), overwriting the union with the row's geography", len(pushes))
	}

	store.fail = false
	im.index(ctx, saved)
	if len(pushes) != 1 {
		t.Fatalf("recovered pushes = %d, want 1", len(pushes))
	}
}
