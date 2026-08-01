package llmkey

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
)

// fakeQueries is an in-memory stand-in for the generated query surface, holding what is
// stored per account and letting a test fail any one of the three statements.
type fakeQueries struct {
	mu     sync.Mutex
	stored map[int64]string

	// commitDuringMint is the other transaction landing while we hold a freshly minted
	// key: the first read saw nothing, and by the time we try to claim, the winner's
	// credential is already committed. This is the real interleaving, reproduced without
	// putting a seam in the production type.
	commitDuringMint string

	getErr, claimErr, clearErr error
	claims                     int
}

func newFakeQueries() *fakeQueries { return &fakeQueries{stored: map[int64]string{}} }

func (f *fakeQueries) GetUserLLMKey(_ context.Context, id int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.stored[id], nil
}

// ClaimUserLLMKey mirrors the real statement: it writes only while the account has none,
// and reports no rows when somebody else got there first.
func (f *fakeQueries) ClaimUserLLMKey(_ context.Context, arg db.ClaimUserLLMKeyParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims++
	if f.claimErr != nil {
		return "", f.claimErr
	}
	if f.commitDuringMint != "" {
		f.stored[arg.ID] = f.commitDuringMint
	}
	if f.stored[arg.ID] != "" {
		return "", pgx.ErrNoRows
	}
	f.stored[arg.ID] = arg.LlmKey.String
	return arg.LlmKey.String, nil
}

func (f *fakeQueries) ClearUserLLMKey(_ context.Context, arg db.ClearUserLLMKeyParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.clearErr != nil {
		return f.clearErr
	}
	if f.stored[arg.ID] == arg.LlmKey.String {
		delete(f.stored, arg.ID)
	}
	return nil
}

// routedGateway answers /key/generate with successive secrets and records every deletion,
// so a test can prove an abandoned key was cleaned up rather than orphaned.
type routedGateway struct {
	mu       sync.Mutex
	mints    []string
	minted   int
	deleted  []string
	mintFail bool
}

func (g *routedGateway) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch r.URL.Path {
		case "/key/generate":
			if g.mintFail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error":"boom"}`)
				return
			}
			secret := "sk-minted"
			if g.minted < len(g.mints) {
				secret = g.mints[g.minted]
			}
			g.minted++
			_, _ = io.WriteString(w, `{"key":"`+secret+`","token_id":"h"}`)
		case "/key/delete":
			var body struct {
				Keys []string `json:"keys"`
			}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			g.deleted = append(g.deleted, body.Keys...)
			_, _ = io.WriteString(w, `{"deleted_keys":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testResolver(t *testing.T, q Queries, g *routedGateway) *Resolver {
	t.Helper()
	return NewResolver(q, New(Config{BaseURL: g.server(t).URL, AdminKey: "sk-admin"}))
}

func TestForReturnsTheStoredCredentialWithoutMinting(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-already"
	g := &routedGateway{}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "sk-already" {
		t.Errorf("For = %q, want the stored credential", got)
	}
	if g.minted != 0 {
		t.Errorf("minted %d times, want none — an account that has a key must not get another", g.minted)
	}
}

func TestForMintsAndStoresOnFirstUse(t *testing.T) {
	q := newFakeQueries()
	g := &routedGateway{mints: []string{"sk-fresh"}}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "sk-fresh" {
		t.Errorf("For = %q, want the freshly minted credential", got)
	}
	if q.stored[7] != "sk-fresh" {
		t.Errorf("stored %q, want the credential persisted so the next call reuses it", q.stored[7])
	}
}

// Two concurrent first calls both mint. The database admits one; the loser must adopt the
// winner's credential AND revoke the one it minted, which would otherwise sit at the
// gateway forever, spending nothing and appearing in every listing.
func TestForLosesTheRaceAndRevokesItsOwnKey(t *testing.T) {
	q := newFakeQueries()
	q.commitDuringMint = "sk-winner"
	g := &routedGateway{mints: []string{"sk-loser"}}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "sk-winner" {
		t.Errorf("For = %q, want the credential that actually got stored", got)
	}
	if len(g.deleted) != 1 || g.deleted[0] != "sk-loser" {
		t.Errorf("deleted %v, want exactly the abandoned key", g.deleted)
	}
	if q.stored[7] != "sk-winner" {
		t.Errorf("stored %q, want the winner's credential untouched", q.stored[7])
	}
}

// Every failure is unattributed spend, never a failed call: the caller reads "" and falls
// back to the service credential.
func TestForFallsOpenWhenMintingFails(t *testing.T) {
	q := newFakeQueries()
	g := &routedGateway{mintFail: true}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "" {
		t.Errorf("For = %q, want \"\" so the caller spends on the service credential", got)
	}
	if q.stored[7] != "" {
		t.Errorf("stored %q after a failed mint, want nothing", q.stored[7])
	}
}

func TestForFallsOpenWhenTheStoreCannotBeRead(t *testing.T) {
	q := newFakeQueries()
	q.getErr = errors.New("connection refused")
	g := &routedGateway{}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "" {
		t.Errorf("For = %q, want \"\" — a database fault must not fail the user's call", got)
	}
	if g.minted != 0 {
		t.Errorf("minted %d times on an unreadable store, want none", g.minted)
	}
}

// A credential minted but not stored can never be found again, so it must be revoked
// immediately rather than left to spend under an account nobody can connect it to.
func TestForRevokesACredentialItCouldNotStore(t *testing.T) {
	q := newFakeQueries()
	q.claimErr = errors.New("deadlock detected")
	g := &routedGateway{mints: []string{"sk-unstorable"}}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "" {
		t.Errorf("For = %q, want \"\" — a credential we cannot store must not be used", got)
	}
	if len(g.deleted) != 1 || g.deleted[0] != "sk-unstorable" {
		t.Errorf("deleted %v, want the unstorable credential revoked", g.deleted)
	}
}

func TestNilResolverIsUnattributed(t *testing.T) {
	var r *Resolver
	if got := r.For(context.Background(), 7); got != "" {
		t.Errorf("For on an unconfigured resolver = %q, want \"\"", got)
	}
	r.Forget(context.Background(), 7, "sk-anything") // must not panic
}

// Forget is what a rejected credential triggers. Clearing the row comes first: that is
// what lets the very next call mint a working replacement, and the gateway delete is
// mopping up something it has usually already forgotten.
func TestForgetClearsTheRowAndRevokesTheKey(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-stale"
	g := &routedGateway{}
	r := testResolver(t, q, g)

	r.Forget(context.Background(), 7, "sk-stale")

	if q.stored[7] != "" {
		t.Errorf("stored %q, want the stale credential cleared so the next call re-mints", q.stored[7])
	}
	if len(g.deleted) != 1 || g.deleted[0] != "sk-stale" {
		t.Errorf("deleted %v, want the stale credential revoked", g.deleted)
	}
}

// Forgetting must not throw away a credential a concurrent call already replaced.
func TestForgetLeavesAReplacementAlone(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-replacement"
	g := &routedGateway{}
	r := testResolver(t, q, g)

	r.Forget(context.Background(), 7, "sk-stale")

	if q.stored[7] != "sk-replacement" {
		t.Errorf("stored %q, want the replacement kept", q.stored[7])
	}
}

// Guard the pgtype wrapping: an empty-string key must never be written as a stored value,
// because "" is the signal for "not minted" and storing it would strand the account.
func TestClaimNeverStoresAnEmptyCredential(t *testing.T) {
	q := newFakeQueries()
	g := &routedGateway{mints: []string{""}}
	r := testResolver(t, q, g)

	if got := r.For(context.Background(), 7); got != "" {
		t.Errorf("For = %q, want \"\"", got)
	}
	if _, present := q.stored[7]; present {
		t.Error("an empty credential was stored; \"\" is the not-minted signal and must stay unwritten")
	}
	if q.claims != 0 {
		t.Errorf("attempted %d claims for an empty credential, want none", q.claims)
	}
}
