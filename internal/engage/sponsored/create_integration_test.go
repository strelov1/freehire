//go:build integration

package sponsored

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestCreate(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	_, jobID := seedCampaign(t, pool, 50, 0, StatusDraft)
	var owner int64
	if err := pool.QueryRow(ctx, `SELECT owner_user_id FROM sponsored_campaigns`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var closedID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, closed_at)
		 VALUES ('test', 'j2', 'https://example.test/j2', 'Old', 'co', 'co', 'j2', now()) RETURNING id`).Scan(&closedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, is_private)
		 VALUES ('test', 'j3', 'https://example.test/j3', 'Mine', 'co', 'co', 'j3', true)`); err != nil {
		t.Fatal(err)
	}
	svc := New(pool).WithMinCPC(10)
	cap5, cap20 := int64(5), int64(20)

	tests := []struct {
		name string
		slug string
		cpc  int64
		cap  *int64
		want error
	}{
		{"ok", "j1", 10, nil, nil},
		{"ok with cap", "j1", 10, &cap20, nil},
		{"below minimum", "j1", 9, nil, ErrInvalid},
		{"cap below cpc", "j1", 10, &cap5, ErrInvalid},
		{"closed job", "j2", 10, nil, ErrJobUnavailable},
		{"private job", "j3", 10, nil, ErrJobUnavailable},
		{"unknown job", "nope", 10, nil, ErrJobUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := svc.Create(ctx, owner, tc.slug, tc.cpc, tc.cap)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && (c.Status != string(StatusDraft) || c.OwnerUserID != owner || c.JobID != jobID || c.CpcCents != tc.cpc) {
				t.Fatalf("campaign = %+v", c)
			}
		})
	}
}
