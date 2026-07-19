//go:build integration

// Integration tests for the employee-referral queries — offer moderation, the
// company-pool request model, active-request dedup, and the per-day cap are all
// constraint/ON CONFLICT semantics that only a real Postgres verifies. Run with:
// go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func TestReferralOffers(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			"TRUNCATE referral_requests, referral_offers, cvs, users, jobs, companies RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}

	t.Run("create offer starts pending; duplicate rejected", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "ref@example.test")
		insertCompany(t, pool, "acme", "acme")

		off, err := q.CreateReferralOffer(ctx, CreateReferralOfferParams{
			UserID: uid, CompanySlug: "acme", ProofObjectKey: "s3://proof/1.pdf",
		})
		if err != nil {
			t.Fatalf("create offer: %v", err)
		}
		if off.Status != "pending" {
			t.Errorf("status = %q, want pending", off.Status)
		}

		if _, err := q.CreateReferralOffer(ctx, CreateReferralOfferParams{
			UserID: uid, CompanySlug: "acme", ProofObjectKey: "s3://proof/2.pdf",
		}); err == nil {
			t.Error("duplicate offer for (user, company) should violate unique, got nil")
		}
	})

	t.Run("approve makes company referral-eligible", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "ref@example.test")
		mod := insertUser(t, pool, "mod@example.test")
		insertCompany(t, pool, "acme", "acme")
		off, err := q.CreateReferralOffer(ctx, CreateReferralOfferParams{
			UserID: uid, CompanySlug: "acme", ProofObjectKey: "s3://proof/1.pdf",
		})
		if err != nil {
			t.Fatalf("create offer: %v", err)
		}

		has, err := q.CompanyHasApprovedReferrer(ctx, "acme")
		if err != nil {
			t.Fatalf("has approved: %v", err)
		}
		if has {
			t.Error("company should not be eligible while offer is pending")
		}

		decided, err := q.DecideReferralOffer(ctx, DecideReferralOfferParams{
			ID: off.ID, Status: "approved", DecidedBy: pgtype.Int8{Int64: mod, Valid: true},
		})
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if decided.Status != "approved" || !decided.DecidedBy.Valid || decided.DecidedBy.Int64 != mod {
			t.Errorf("decided = %+v, want approved by %d", decided, mod)
		}

		has, err = q.CompanyHasApprovedReferrer(ctx, "acme")
		if err != nil {
			t.Fatalf("has approved: %v", err)
		}
		if !has {
			t.Error("company should be eligible after approval")
		}

		// A second decision on an already-decided offer matches no row.
		if _, err := q.DecideReferralOffer(ctx, DecideReferralOfferParams{
			ID: off.ID, Status: "rejected", DecidedBy: pgtype.Int8{Int64: mod, Valid: true},
		}); err == nil {
			t.Error("re-deciding a decided offer should match no row (pgx.ErrNoRows)")
		}
	})
}

func TestReferralRequests(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			"TRUNCATE referral_requests, referral_offers, cvs, users, jobs, companies RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}

	approvedOffer := func(t *testing.T, referrer int64, slug string) {
		t.Helper()
		mod := insertUser(t, pool, "mod-"+slug+"@example.test")
		off, err := q.CreateReferralOffer(ctx, CreateReferralOfferParams{
			UserID: referrer, CompanySlug: slug, ProofObjectKey: "s3://proof.pdf",
		})
		if err != nil {
			t.Fatalf("offer: %v", err)
		}
		if _, err := q.DecideReferralOffer(ctx, DecideReferralOfferParams{
			ID: off.ID, Status: "approved", DecidedBy: pgtype.Int8{Int64: mod, Valid: true},
		}); err != nil {
			t.Fatalf("approve: %v", err)
		}
	}

	t.Run("create request with original CV; active duplicate rejected", func(t *testing.T) {
		reset(t)
		seeker := insertUser(t, pool, "seeker@example.test")
		referrer := insertUser(t, pool, "ref@example.test")
		insertCompany(t, pool, "acme", "acme")
		approvedOffer(t, referrer, "acme")

		req, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "original",
			ContactEmail: text("seeker@example.test"), Note: "hi",
		})
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		if req.Status != "sent" {
			t.Errorf("status = %q, want sent", req.Status)
		}

		if _, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "original",
			ContactEmail: text("seeker@example.test"),
		}); err == nil {
			t.Error("second active request for (seeker, company) should violate partial unique")
		}
	})

	t.Run("referrer inbox lists sent requests for their companies; mark contacted", func(t *testing.T) {
		reset(t)
		seeker := insertUser(t, pool, "seeker@example.test")
		referrer := insertUser(t, pool, "ref@example.test")
		other := insertUser(t, pool, "other@example.test")
		insertCompany(t, pool, "acme", "acme")
		insertCompany(t, pool, "globex", "globex")
		approvedOffer(t, referrer, "acme")
		approvedOffer(t, other, "globex")

		req, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "original",
			ContactTelegram: text("@seeker"),
		})
		if err != nil {
			t.Fatalf("create request: %v", err)
		}

		inbox, err := q.ListIncomingReferralRequests(ctx, referrer)
		if err != nil {
			t.Fatalf("inbox: %v", err)
		}
		if len(inbox) != 1 || inbox[0].ID != req.ID {
			t.Fatalf("referrer inbox = %+v, want just request %d", inbox, req.ID)
		}
		if otherInbox, _ := q.ListIncomingReferralRequests(ctx, other); len(otherInbox) != 0 {
			t.Errorf("other referrer inbox = %d, want 0 (different company)", len(otherInbox))
		}

		marked, err := q.ResolveReferralRequest(ctx, ResolveReferralRequestParams{
			ID: req.ID, Status: "contacted", ActedBy: pgtype.Int8{Int64: referrer, Valid: true},
		})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if marked.Status != "contacted" || marked.ActedBy.Int64 != referrer {
			t.Errorf("marked = %+v, want contacted by %d", marked, referrer)
		}

		// Resolved leaves the pool, and the seeker may request again.
		if inbox, _ := q.ListIncomingReferralRequests(ctx, referrer); len(inbox) != 0 {
			t.Errorf("inbox after resolve = %d, want 0", len(inbox))
		}
		if _, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "original",
			ContactTelegram: text("@seeker"),
		}); err != nil {
			t.Errorf("re-request after resolution should succeed, got %v", err)
		}
	})

	t.Run("DB guards: contact required; original must not carry a cv_id", func(t *testing.T) {
		reset(t)
		seeker := insertUser(t, pool, "seeker@example.test")
		referrer := insertUser(t, pool, "ref@example.test")
		insertCompany(t, pool, "acme", "acme")
		approvedOffer(t, referrer, "acme")

		// No contact at all → contact CHECK violation.
		if _, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "original",
		}); err == nil {
			t.Error("request with no contact should violate contact CHECK")
		}

		// cv_kind='original' with a cv_id → cv_kind CHECK violation. (The "built
		// requires a cv_id" invariant is a domain-layer concern, not a DB CHECK,
		// so ON DELETE SET NULL can null a built request's cv_id without failing.)
		var cvID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`,
			seeker).Scan(&cvID); err != nil {
			t.Fatalf("insert cv: %v", err)
		}
		if _, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "original",
			CvID: pgtype.Int8{Int64: cvID, Valid: true}, ContactEmail: text("seeker@example.test"),
		}); err == nil {
			t.Error("original CV carrying a cv_id should violate cv_kind CHECK")
		}
	})

	t.Run("deleting a built request's CV nulls cv_id, keeps the request", func(t *testing.T) {
		reset(t)
		seeker := insertUser(t, pool, "seeker@example.test")
		referrer := insertUser(t, pool, "ref@example.test")
		insertCompany(t, pool, "acme", "acme")
		approvedOffer(t, referrer, "acme")

		var cvID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`,
			seeker).Scan(&cvID); err != nil {
			t.Fatalf("insert cv: %v", err)
		}
		req, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
			SeekerUserID: seeker, CompanySlug: "acme", CvKind: "built",
			CvID: pgtype.Int8{Int64: cvID, Valid: true}, ContactEmail: text("seeker@example.test"),
		})
		if err != nil {
			t.Fatalf("create built request: %v", err)
		}

		// The CV builder deletes the CV — the ON DELETE SET NULL must null cv_id
		// without tripping the cv_kind CHECK, so the request outlives the CV.
		if _, err := pool.Exec(ctx, `DELETE FROM cvs WHERE id = $1`, cvID); err != nil {
			t.Fatalf("delete cv: %v", err)
		}
		got, err := q.GetReferralRequest(ctx, req.ID)
		if err != nil {
			t.Fatalf("get request after cv delete: %v", err)
		}
		if got.CvID.Valid {
			t.Errorf("cv_id = %v, want NULL after CV deletion", got.CvID)
		}
		if got.CvKind != "built" {
			t.Errorf("cv_kind = %q, want built (unchanged)", got.CvKind)
		}
	})

	t.Run("per-day cap count", func(t *testing.T) {
		reset(t)
		seeker := insertUser(t, pool, "seeker@example.test")
		referrer := insertUser(t, pool, "ref@example.test")
		insertCompany(t, pool, "acme", "acme")
		insertCompany(t, pool, "globex", "globex")
		approvedOffer(t, referrer, "acme")
		approvedOffer(t, referrer, "globex")

		for _, slug := range []string{"acme", "globex"} {
			if _, err := q.CreateReferralRequest(ctx, CreateReferralRequestParams{
				SeekerUserID: seeker, CompanySlug: slug, CvKind: "original",
				ContactEmail: text("seeker@example.test"),
			}); err != nil {
				t.Fatalf("create request %s: %v", slug, err)
			}
		}

		n, err := q.CountReferralRequestsSince(ctx, CountReferralRequestsSinceParams{
			SeekerUserID: seeker, Since: pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true},
		})
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 2 {
			t.Errorf("count since epoch = %d, want 2", n)
		}
	})
}
