//go:build integration

// The guided setup over a real grant and a real Postgres: a CV-less account is sent to add a
// CV, adds one as text through the website's upload use case, and cannot replace it from the
// chat. Run with: go test -tags=integration ./internal/api/handler/ -run TestMCPAccountSetup
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// setupFixture is a signed-in MCP account server over a real Postgres, with the CV store wired
// as production wires it (the per-user upload lock included) over in-memory blobs.
type setupFixture struct {
	userID int64
	blobs  *lockedResumeBlobs
	store  *resume.Store
	url    string
	token  string
}

// slowCheckRepo widens the window between the presence check and the pointer write, the one two
// first uploads would both slip through without the upload lock: the check inside the lock is
// slowed, so only the lock keeps the second upload from reading "no CV" too.
type slowCheckRepo struct{ resume.Repository }

func (r slowCheckRepo) WithUploadLock(ctx context.Context, userID int64, fn func(resume.Repository) error) error {
	return r.Repository.WithUploadLock(ctx, userID, func(locked resume.Repository) error {
		return fn(slowGet{locked})
	})
}

type slowGet struct{ resume.Repository }

func (r slowGet) Get(ctx context.Context, userID int64) (db.GetUserResumeRow, error) {
	row, err := r.Repository.Get(ctx, userID)
	time.Sleep(100 * time.Millisecond)
	return row, err
}

// lockedResumeBlobs is fakeResumeBlobs made safe for concurrent callers.
type lockedResumeBlobs struct {
	mu sync.Mutex
	*fakeResumeBlobs
}

func (b *lockedResumeBlobs) Put(ctx context.Context, key, ct string, r io.Reader, n int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fakeResumeBlobs.Put(ctx, key, ct, r, n)
}

func (b *lockedResumeBlobs) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fakeResumeBlobs.Get(ctx, key)
}

func (b *lockedResumeBlobs) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fakeResumeBlobs.Delete(ctx, key)
}

func (b *lockedResumeBlobs) List(ctx context.Context, prefix string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fakeResumeBlobs.List(ctx, prefix)
}

// count is how many objects the bucket holds: one per stored CV, so anything more is an orphan.
func (b *lockedResumeBlobs) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.objs)
}

// current is the stored CV as its pointer names it.
func (b *lockedResumeBlobs) current(t *testing.T, store *resume.Store, userID int64) string {
	t.Helper()
	data, err := store.File(context.Background(), userID)
	if err != nil {
		t.Fatalf("reading the stored CV: %v", err)
	}
	return string(data)
}

func newSetupFixture(t *testing.T, email string) setupFixture {
	t.Helper()
	ctx := context.Background()
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, email)
	seedOAuthTestClient(t, queries, "client-mcp-setup", []string{"http://127.0.0.1:9999/callback"})
	version, err := queries.GetUserTokenVersion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	const token = "fhm_test-setup-token"
	expiresAt := time.Now().Add(time.Hour)
	if _, err := queries.CreateOAuthGrant(ctx, db.CreateOAuthGrantParams{
		UserID: userID, ClientID: "client-mcp-setup", AccessTokenHash: oauth2server.HashToken(token),
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		t.Fatal(err)
	}

	blobs := &lockedResumeBlobs{fakeResumeBlobs: newFakeResumeBlobs()}
	store := resume.New(blobs, slowCheckRepo{resume.NewQueriesRepository(queries).WithUploadLocks(pool)})
	profiles := userprofile.New(userprofile.NewQueriesRepository(queries))
	assistants := &assistantHandlers{
		queries: queries,
		resume:  &resumeHandlers{resume: store},
		profile: &profileHandlers{userProfile: profiles, resume: store},
		plans:   plan.NewStore(queries, pool, plan.DefaultConfig()),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountMCP(app.Group("/api/v1"), assistants, newMCPAppHandlers(nil, nil, queries, testMCPOrigin))
	server := httptest.NewServer(adaptor.FiberApp(app))
	t.Cleanup(server.Close)
	// The background derivation has no extractor here and records a failure; wait for it so
	// the goroutine is done with the database before the test tears it down.
	t.Cleanup(func() { waitForExtractOutcome(t, store, userID) })
	return setupFixture{userID: userID, blobs: blobs, store: store, url: server.URL + "/api/v1/mcp", token: token}
}

func (f setupFixture) connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{
			Endpoint:   f.url,
			HTTPClient: &http.Client{Transport: bearerRoundTripper{token: f.token}},
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func waitForExtractOutcome(t *testing.T, store *resume.Store, userID int64) {
	t.Helper()
	ctx := context.Background()
	if meta, err := store.Status(ctx, userID); err != nil || !meta.Present {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		row, err := store.StructuredRow(ctx, userID)
		if err == nil && resume.ResolveExtractStatus(row).Status == resume.ExtractStatusFailed {
			return
		}
		if time.Now().After(deadline) {
			t.Error("the background CV derivation never finished")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMCPAccountSetup_AddsAFirstCVAndRefusesAReplacement(t *testing.T) {
	ctx := context.Background()
	f := newSetupFixture(t, "mcp-setup@example.test")
	session, userID, store, blobs := f.connect(t), f.userID, f.store, f.blobs

	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res
	}
	status := func() setupStatus {
		t.Helper()
		res := call("get_setup_status", map[string]any{})
		if res.IsError {
			t.Fatalf("get_setup_status: %s", res.Content[0].(*mcp.TextContent).Text)
		}
		var out setupStatus
		if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	before := status()
	if before.NextStep.Tool != "add_cv" || before.Complete {
		t.Fatalf("a new account's setup = %+v, want next_step add_cv", before)
	}
	for _, name := range []string{"cv", "preferences", "alerts", "paid_plan", "agents_may_apply"} {
		stepNamed(t, before, name)
	}

	res := call("add_cv", map[string]any{"text": addCVText})
	if res.IsError {
		t.Fatalf("add_cv: %s", res.Content[0].(*mcp.TextContent).Text)
	}
	var added addCVResult
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &added); err != nil {
		t.Fatal(err)
	}
	if !added.Stored || len(added.Suggested.Skills) == 0 {
		t.Errorf("add_cv result = %+v, want stored with suggested skills", added)
	}
	if meta, err := store.Status(ctx, userID); err != nil || !meta.Present {
		t.Fatalf("stored CV status = %+v, %v; want present", meta, err)
	}
	waitForExtractOutcome(t, store, userID)
	if after := status(); after.NextStep.Tool == "add_cv" {
		t.Errorf("after add_cv, next_step still asks for a CV: %+v", after.NextStep)
	}

	res = call("add_cv", map[string]any{"text": addCVText + " Replaced."})
	if !res.IsError {
		t.Fatal("a second add_cv replaced the stored CV")
	}
	if got := blobs.current(t, store, userID); got != addCVText || blobs.count() != 1 {
		t.Errorf("stored CV = %q, want the first text", got)
	}
}

// Two first CVs at once: the upload lock makes the presence check and the write one step, so
// exactly one is stored and the other is refused as already having a CV.
func TestMCPAccountSetup_ConcurrentFirstCVsStoreExactlyOne(t *testing.T) {
	f := newSetupFixture(t, "mcp-setup-race@example.test")
	texts := []string{addCVText + " First.", addCVText + " Second."}
	sessions := []*mcp.ClientSession{f.connect(t), f.connect(t)}
	results := make([]*mcp.CallToolResult, len(texts))
	var wg sync.WaitGroup
	for i := range texts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := sessions[i].CallTool(context.Background(), &mcp.CallToolParams{
				Name: "add_cv", Arguments: map[string]any{"text": texts[i]},
			})
			if err != nil {
				t.Errorf("add_cv %d: %v", i, err)
				return
			}
			results[i] = res
		}(i)
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	stored := -1
	for i, res := range results {
		if res.IsError {
			continue
		}
		if stored >= 0 {
			t.Fatal("both concurrent add_cv calls stored a CV")
		}
		stored = i
	}
	if stored < 0 {
		t.Fatal("neither concurrent add_cv call stored a CV")
	}
	refused := results[1-stored].Content[0].(*mcp.TextContent).Text
	if !strings.Contains(refused, "already has a CV") {
		t.Errorf("refusal = %q, want the already-has-a-CV answer", refused)
	}
	if got := f.blobs.current(t, f.store, f.userID); got != texts[stored] || f.blobs.count() != 1 {
		t.Errorf("stored CV = %q, want the winner's text %q", got, texts[stored])
	}
}
