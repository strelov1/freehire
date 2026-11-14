package changehook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestSignAndVerify(t *testing.T) {
	body := []byte(`{"id":"1-2-3","type":"job.changes","changes":[]}`)
	now := time.Unix(1_800_000_000, 0)
	sig := Sign("whsec_x", now.Unix(), body)

	// The documented construction, written out: HMAC-SHA256(secret, "<ts>.<body>"), hex, "v1=".
	mac := hmac.New(sha256.New, []byte("whsec_x"))
	mac.Write([]byte("1800000000." + string(body)))
	if want := "v1=" + hex.EncodeToString(mac.Sum(nil)); sig != want {
		t.Fatalf("Sign = %q, want %q", sig, want)
	}
	ts := "1800000000"
	for _, tc := range []struct {
		name, secret, ts, sig string
		body                  []byte
		now                   time.Time
		want                  error
	}{
		{"valid", "whsec_x", ts, sig, body, now, nil},
		{"within tolerance", "whsec_x", ts, sig, body, now.Add(4 * time.Minute), nil},
		{"wrong secret", "whsec_y", ts, sig, body, now, ErrBadSignature},
		{"body changed", "whsec_x", ts, sig, append(body[:len(body)-1:len(body)-1], ' ', '}'), now, ErrBadSignature},
		{"timestamp changed", "whsec_x", "1800000001", sig, body, now, ErrBadSignature},
		{"replayed later", "whsec_x", ts, sig, body, now.Add(6 * time.Minute), ErrStale},
		{"garbage timestamp", "whsec_x", "soon", sig, body, now, ErrBadSignature},
	} {
		if got := Verify(tc.secret, tc.ts, tc.sig, tc.body, tc.now, DefaultTolerance); got != tc.want {
			t.Errorf("%s: Verify = %v, want %v", tc.name, got, tc.want)
		}
	}
}

var kinds3 = []string{"opened", "closed", "reopened"}

func row(id int64, kind, company string) db.ListJobChangesRow {
	return db.ListJobChangesRow{ID: id, Xid: 10, Kind: kind, CompanySlug: company, PublicSlug: "job-" + company,
		At: pgtype.Timestamptz{Time: time.Unix(1_800_000_000, 0), Valid: true}}
}

func TestBatchFiltersByCompanyAndKindKeepingOrder(t *testing.T) {
	rows := []db.ListJobChangesRow{row(1, "opened", "acme"), row(2, "closed", "other"), row(3, "closed", "acme"), row(4, "reopened", "acme")}

	all := Filter{Kinds: kinds3}.Batch(rows)
	if len(all) != 4 {
		t.Fatalf("no company filter: %d changes, want 4", len(all))
	}
	got := Filter{CompanySlugs: []string{"acme"}, Kinds: []string{"opened", "closed"}}.Batch(rows)
	if len(got) != 2 || got[0].Seq != 1 || got[1].Seq != 3 {
		t.Fatalf("filtered = %+v, want seq 1 then 3", got)
	}
	if got[1].ClosedReason != "" || got[1].CompanySlug != "acme" {
		t.Fatalf("change = %+v", got[1])
	}
	if n := len(Filter{CompanySlugs: []string{"nobody"}, Kinds: kinds3}.Batch(rows)); n != 0 {
		t.Fatalf("unmatched company delivered %d changes", n)
	}
}

func TestBackoffDoublesToTheCeiling(t *testing.T) {
	for n, want := range map[int64]time.Duration{0: 30 * time.Second, 1: time.Minute, 2: 2 * time.Minute, 6: 32 * time.Minute, 7: time.Hour, 24: time.Hour} {
		if got := backoff(n); got != want {
			t.Errorf("backoff(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestDeliveryIDIsStableForABatch(t *testing.T) {
	if a, b := deliveryID(7, row(9, "closed", "x")), deliveryID(7, row(9, "closed", "x")); a != b || a != "7-10-9" {
		t.Fatalf("ids %q %q", a, b)
	}
}
