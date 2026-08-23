package llmkey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeQueries is an in-memory stand-in for the generated query surface, holding what is
// stored per account and letting a test fail any one of the three statements.
type fakeQueries struct {
	mu     sync.Mutex
	stored map[int64]string
	// storedIDs holds the gateway's own identifier beside the secret, because the two
	// are written and cleared as one credential and a test that tracked only the secret
	// could not tell a revocable credential from an unrevocable one.
	storedIDs map[int64]string

	// commitDuringMint is the other transaction landing while we hold a freshly minted
	// key: the first read saw nothing, and by the time we try to claim, the winner's
	// credential is already committed. This is the real interleaving, reproduced without
	// putting a seam in the production type.
	commitDuringMint string

	getErr, claimErr, clearErr error
	claims                     int
}

func newFakeQueries() *fakeQueries {
	return &fakeQueries{stored: map[int64]string{}, storedIDs: map[int64]string{}}
}

func (f *fakeQueries) GetUserLLMKey(_ context.Context, id int64) (db.GetUserLLMKeyRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return db.GetUserLLMKeyRow{}, f.getErr
	}
	return db.GetUserLLMKeyRow{LlmKey: f.stored[id], LlmKeyID: f.storedIDs[id]}, nil
}

// ClaimUserLLMKey mirrors the real statement: it writes only while the account has none,
// and reports no rows when somebody else got there first.
func (f *fakeQueries) ClaimUserLLMKey(_ context.Context, arg db.ClaimUserLLMKeyParams) (db.ClaimUserLLMKeyRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims++
	if f.claimErr != nil {
		return db.ClaimUserLLMKeyRow{}, f.claimErr
	}
	if f.commitDuringMint != "" {
		f.stored[arg.ID] = f.commitDuringMint
		f.storedIDs[arg.ID] = keyID(f.commitDuringMint)
	}
	if f.stored[arg.ID] != "" {
		return db.ClaimUserLLMKeyRow{}, pgx.ErrNoRows
	}
	f.stored[arg.ID] = arg.LlmKey.String
	f.storedIDs[arg.ID] = arg.LlmKeyID.String
	return db.ClaimUserLLMKeyRow{LlmKey: arg.LlmKey.String, LlmKeyID: arg.LlmKeyID.String}, nil
}

func (f *fakeQueries) ClearUserLLMKey(_ context.Context, arg db.ClearUserLLMKeyParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.clearErr != nil {
		return f.clearErr
	}
	if f.stored[arg.ID] == arg.LlmKey.String {
		delete(f.stored, arg.ID)
		delete(f.storedIDs, arg.ID)
	}
	return nil
}

// keyID is the gateway id a test expects beside a given secret. The real gateway hands
// back two unrelated opaque strings; deriving one from the other here keeps an assertion
// about which credential was blocked readable — "vk-loser" beside "sk-loser" — without
// pretending the production code may assume any such relationship.
func keyID(secret string) string { return "vk-" + strings.TrimPrefix(secret, "sk-") }

// routedGateway answers a create with successive secrets and records every deletion and
// block BY ID, so a test can prove an abandoned key was cleaned up (or, for a refused
// live credential, merely retired) rather than orphaned.
type routedGateway struct {
	mu       sync.Mutex
	mints    []string
	minted   int
	deleted  []string
	blocked  []string
	mintFail bool
}

func (g *routedGateway) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		const keys = "/api/governance/virtual-keys"
		switch {
		// The policy read every mint begins with. Answering it here rather than in
		// each test keeps these cases about the race they exist to describe.
		case r.Method == http.MethodGet && r.URL.Path == keys+"/"+templateKey:
			_, _ = io.WriteString(w, templateBody)
		case r.Method == http.MethodPost && r.URL.Path == keys:
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
			_, _ = io.WriteString(w, `{"virtual_key":{"id":"`+keyID(secret)+`","value":"`+secret+`"}}`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, keys+"/"):
			g.deleted = append(g.deleted, strings.TrimPrefix(r.URL.Path, keys+"/"))
			_, _ = io.WriteString(w, `{"deleted":true}`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, keys+"/"):
			g.blocked = append(g.blocked, strings.TrimPrefix(r.URL.Path, keys+"/"))
			_, _ = io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testResolver(t *testing.T, q Queries, g *routedGateway) *Resolver {
	t.Helper()
	return NewResolver(q, configured(g.server(t).URL))
}

func TestForReturnsTheStoredCredentialWithoutMinting(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-already"
	q.storedIDs[7] = keyID("sk-already")
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
	if len(g.deleted) != 1 || g.deleted[0] != keyID("sk-loser") {
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
	if len(g.deleted) != 1 || g.deleted[0] != keyID("sk-unstorable") {
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
// Forget must BLOCK, not delete: a refused credential looks identical whether the gateway
// has simply forgotten it (the ordinary case) or a concurrent Revoke just blocked it for
// account deletion — and only the latter has spend history worth keeping. Deleting on that
// ambiguity would occasionally erase exactly the history Revoke blocked the key to preserve.
func TestForgetClearsTheRowAndBlocksTheKey(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-stale"
	q.storedIDs[7] = keyID("sk-stale")
	g := &routedGateway{}
	r := testResolver(t, q, g)

	r.Forget(context.Background(), 7, "sk-stale")

	if q.stored[7] != "" {
		t.Errorf("stored %q, want the stale credential cleared so the next call re-mints", q.stored[7])
	}
	if len(g.blocked) != 1 || g.blocked[0] != keyID("sk-stale") {
		t.Errorf("blocked %v, want the stale credential blocked", g.blocked)
	}
	if len(g.deleted) != 0 {
		t.Errorf("deleted %v, want nothing deleted — Forget must not erase spend history", g.deleted)
	}
}

// Forgetting must not throw away a credential a concurrent call already replaced.
func TestForgetLeavesAReplacementAlone(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-replacement"
	q.storedIDs[7] = keyID("sk-replacement")
	g := &routedGateway{}
	r := testResolver(t, q, g)

	r.Forget(context.Background(), 7, "sk-stale")

	if q.stored[7] != "sk-replacement" {
		t.Errorf("stored %q, want the replacement kept", q.stored[7])
	}
	// The hazard this branch guards is not the row but the gateway: blocking on the
	// strength of a stale credential's 401 would revoke a good key.
	if len(g.blocked) != 0 {
		t.Errorf("blocked %v, want nothing — the refusal was about a credential the row no longer holds", g.blocked)
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

// An unreadable store must stop Forget entirely, not merely stop it blocking. The row is
// the only place the gateway's id is written down: clearing it while the read failed
// would leave a live credential spending with nothing able to name it, permanently.
func TestForgetLeavesTheRowAloneWhenItCannotBeRead(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-stale"
	q.storedIDs[7] = keyID("sk-stale")
	q.getErr = errors.New("database down")
	g := &routedGateway{}
	r := testResolver(t, q, g)

	r.Forget(context.Background(), 7, "sk-stale")

	if q.stored[7] != "sk-stale" {
		t.Errorf("stored %q, want the credential left in place — the next refusal retries", q.stored[7])
	}
	if len(g.blocked) != 0 {
		t.Errorf("blocked %v, want nothing blocked on a read we could not trust", g.blocked)
	}
}

// A credential minted before the id column existed has nothing to block. Clearing the row
// is still right — that is what makes the next call mint a complete pair — but reaching
// for the gateway with an empty id would aim a request at no key at all.
func TestForgetClearsAnIdlessRowWithoutBlocking(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-pre-0119"
	g := &routedGateway{}
	r := testResolver(t, q, g)

	r.Forget(context.Background(), 7, "sk-pre-0119")

	if q.stored[7] != "" {
		t.Errorf("stored %q, want the row cleared so the next call mints a pair", q.stored[7])
	}
	if len(g.blocked) != 0 {
		t.Errorf("blocked %v, want nothing blocked — there is no id to name", g.blocked)
	}
}

// Revoke is why the id column exists: account deletion has to stop a credential spending,
// and this gateway will not accept the secret to do it.
func TestRevokeBlocksByIdAndLeavesTheRow(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-departing"
	q.storedIDs[7] = keyID("sk-departing")
	g := &routedGateway{}
	r := testResolver(t, q, g)

	if err := r.Revoke(context.Background(), 7); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if len(g.blocked) != 1 || g.blocked[0] != keyID("sk-departing") {
		t.Errorf("blocked %v, want exactly the departing account's credential, by id", g.blocked)
	}
	if len(g.deleted) != 0 {
		t.Errorf("deleted %v, want nothing deleted — a retired key stays legible in the listings", g.deleted)
	}
	// The row is about to go with the account's own cascade; clearing it here would only
	// cost a write.
	if q.stored[7] != "sk-departing" {
		t.Errorf("stored %q, want the row left to the deletion cascade", q.stored[7])
	}
}

// An account with no credential, and one credentialled before the id column existed, both
// have nothing to revoke. Neither is a fault.
func TestRevokeIsANoopWithoutAnId(t *testing.T) {
	for name, seed := range map[string]func(*fakeQueries){
		"never credentialled": func(*fakeQueries) {},
		"credentialled before 0119": func(q *fakeQueries) {
			q.stored[7] = "sk-pre-0119"
		},
	} {
		t.Run(name, func(t *testing.T) {
			q := newFakeQueries()
			seed(q)
			g := &routedGateway{}
			if err := testResolver(t, q, g).Revoke(context.Background(), 7); err != nil {
				t.Fatalf("Revoke: %v", err)
			}
			if len(g.blocked) != 0 {
				t.Errorf("blocked %v, want nothing", g.blocked)
			}
		})
	}
}

// A read that fails must not let account deletion report success having revoked nothing.
// Everywhere else in this package a failed read costs only attribution; here it would
// leave a live credential spending for an account that no longer exists, and deleting the
// row is what makes its id unrecoverable — so this is the last moment anyone can act.
func TestRevokeReportsAnUnreadableStore(t *testing.T) {
	q := newFakeQueries()
	q.stored[7] = "sk-departing"
	q.storedIDs[7] = keyID("sk-departing")
	q.getErr = errors.New("database down")
	g := &routedGateway{}

	if err := testResolver(t, q, g).Revoke(context.Background(), 7); err == nil {
		t.Fatal("Revoke must surface an unreadable store, not report a revocation it did not perform")
	}
	if len(g.blocked) != 0 {
		t.Errorf("blocked %v, want nothing", g.blocked)
	}
}
