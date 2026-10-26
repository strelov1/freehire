//go:build integration

// Integration tests for membership and the anonymous profile.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

const secretText = "Led the Initech billing rewrite"

func setup(t *testing.T) (*pgxpool.Pool, *buddy.Service) {
	t.Helper()
	pool := testdb.Pool(t)
	return pool, buddy.New(buddy.NewQueriesRepository(db.New(pool)), chat(pool))
}

// otherHandle is the other member's handle as a conversation draws them, empty when they
// are not shown.
func otherHandle(c messaging.Conversation) string {
	d, _ := c.Counterpart.Detail.(buddy.Counterpart)
	return d.Handle
}

// chat is the messaging core with the buddy persona, as the server builds it: what accepts,
// sends, lists, blocks and reports a buddy conversation.
func chat(pool *pgxpool.Pool) *messaging.Service {
	q := db.New(pool)
	return messaging.New(messaging.NewQueriesRepository(q), buddy.NewPersona(buddy.NewQueriesRepository(q)))
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedCV(t *testing.T, pool *pgxpool.Pool, id int64) {
	t.Helper()
	cv, _ := json.Marshal(map[string]any{
		"total_years": 7,
		"skills":      []string{"Go", "PostgreSQL"},
		"experience": []map[string]any{{
			"title": "Senior Backend Engineer", "company": "Initech", "current": true,
			"summary": secretText, "highlights": []string{secretText},
		}},
	})
	if _, err := pool.Exec(context.Background(), `UPDATE users SET resume_structured = $2 WHERE id = $1`, id, cv); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
}

func TestJoinMintsHandleAndRejoinKeepsIt(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	u := seedUser(t, pool, "a@example.test")

	m, err := svc.Join(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if m.Handle == "" || !m.Active {
		t.Fatalf("join = %+v", m)
	}
	if err := svc.Leave(ctx, u); err != nil {
		t.Fatal(err)
	}
	if st, _ := svc.Status(ctx, u); st.Member {
		t.Fatal("left member reports as member")
	}
	again, err := svc.Join(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if again.Handle != m.Handle || !again.Active {
		t.Fatalf("rejoin = %+v, want handle %q", again, m.Handle)
	}
	st, err := svc.Status(ctx, u)
	if err != nil || !st.Member || st.Handle != m.Handle {
		t.Fatalf("status = %+v, %v", st, err)
	}
}

func TestNonMemberIsRefused(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, b := seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test")
	mb, err := svc.Join(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, a); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("leave by non-member = %v", err)
	}
	if err := svc.Touch(ctx, a); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("touch by non-member = %v", err)
	}
	if _, err := svc.Profile(ctx, a, mb.Handle); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("profile by non-member = %v", err)
	}
	if st, err := svc.Status(ctx, a); err != nil || st.Member {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if _, err := svc.Join(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Profile(ctx, a, "no-such-handle-00"); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("unknown handle = %v", err)
	}
}

func TestLeaveClosesConversations(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, b, c := seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test"), seedUser(t, pool, "c@example.test")
	for _, u := range []int64{a, b, c} {
		if _, err := svc.Join(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	conv := func(from, to int64, status string) int64 {
		t.Helper()
		cv, err := chat(pool).Start(ctx, messaging.StartRequest{
			Kind: buddy.KindName, StarterID: from, StarterPersona: buddy.PersonaName,
			RecipientID: to, RecipientPersona: buddy.PersonaName, Body: "hi",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE conversations SET status = $2 WHERE id = $1`, cv.ID, status); err != nil {
			t.Fatal(err)
		}
		return cv.ID
	}
	pending, open, others := conv(a, b, "pending"), conv(c, a, "open"), conv(b, c, "open")

	if err := svc.Leave(ctx, a); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]string{pending: "closed", open: "closed", others: "open"} {
		var status string
		var closed bool
		if err := pool.QueryRow(ctx, `SELECT status, closed_at IS NOT NULL FROM conversations WHERE id = $1`, id).Scan(&status, &closed); err != nil {
			t.Fatal(err)
		}
		if status != want || closed != (want == "closed") {
			t.Errorf("conversation %d = %s (closed_at set: %v), want %s", id, status, closed, want)
		}
	}
}

func TestProfileCardCarriesNoCVText(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, b := seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test")
	seedCV(t, pool, b)
	if _, err := svc.Join(ctx, a); err != nil {
		t.Fatal(err)
	}
	mb, err := svc.Join(ctx, b)
	if err != nil {
		t.Fatal(err)
	}

	p, err := svc.Profile(ctx, a, mb.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if p.EmptyCard || p.Handle != mb.Handle || len(p.Card.Roles) != 1 || p.Card.Roles[0].Title != "Senior Backend Engineer" {
		t.Fatalf("profile = %+v", p)
	}
	raw, _ := json.Marshal(p)
	for _, leak := range []string{"Initech", secretText, "example.test"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("profile leaks %q: %s", leak, raw)
		}
	}
}

func TestEmptyCVGivesEmptyCard(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, b := seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test")
	if _, err := svc.Join(ctx, a); err != nil {
		t.Fatal(err)
	}
	mb, err := svc.Join(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Profile(ctx, a, mb.Handle)
	if err != nil || !p.EmptyCard {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	if st, _ := svc.Status(ctx, b); !st.EmptyCard {
		t.Fatalf("status = %+v, want empty card", st)
	}
}

func TestBuddyHandleDiffersFromTalentHandle(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	u := seedUser(t, pool, "a@example.test")
	const talent = "senior-backend-engineer-0001"
	if _, err := pool.Exec(ctx, `UPDATE users SET talent_handle = $2 WHERE id = $1`, u, talent); err != nil {
		t.Fatal(err)
	}
	m, err := svc.Join(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if m.Handle == talent {
		t.Fatalf("buddy handle %q equals the talent handle", m.Handle)
	}
}

func TestStaleGeographyFallsBackToBaseCountry(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	u := seedUser(t, pool, "a@example.test")
	if _, err := pool.Exec(ctx, `INSERT INTO user_profiles (user_id, skills, specializations, location_preferences) VALUES ($1, '{go}', '{backend}','{"base":{"country":"pl"}}')`, u); err != nil {
		t.Fatal(err)
	}
	// The geography was derived from an earlier upload than the CV now stored.
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_countries = '{de}', resume_structured_uploaded_at = '2026-01-01', resume_uploaded_at = '2026-02-01' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	m, err := svc.Join(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(m.Countries) != "[pl]" {
		t.Fatalf("countries = %v, want [pl]", m.Countries)
	}
}

func TestTouchRefreshesRankingKeys(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	u := seedUser(t, pool, "a@example.test")
	m, err := svc.Join(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if m.Category != "" {
		t.Fatalf("category before CV = %q", m.Category)
	}
	seedCV(t, pool, u)
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_countries = '{de}', resume_uploaded_at = '2026-01-01', resume_structured_uploaded_at = '2026-01-01' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	if err := svc.Touch(ctx, u); err != nil {
		t.Fatal(err)
	}
	var category *string
	var countries []string
	if err := pool.QueryRow(ctx, `SELECT category, countries FROM buddy_members WHERE user_id = $1`, u).Scan(&category, &countries); err != nil {
		t.Fatal(err)
	}
	if category == nil || *category == "" || fmt.Sprint(countries) != "[de]" {
		t.Fatalf("keys = %v %v", category, countries)
	}
}
