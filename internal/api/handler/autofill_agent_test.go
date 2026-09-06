package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/autofillagent"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
)

// stubValidKey authenticates exactly one full-scope key; any other hash is unknown,
// mirroring the real db layer's ErrNoRows. Mirrors internal/identity/auth's own fakeKeyAuth.
type stubValidKey struct {
	hash   string
	userID int64
}

func (s stubValidKey) AuthenticateAPIKey(_ context.Context, hash string) (auth.APIKeyIdentity, error) {
	if hash == s.hash {
		return auth.APIKeyIdentity{UserID: s.userID, Scope: auth.ScopeFull}, nil
	}
	return auth.APIKeyIdentity{}, pgx.ErrNoRows
}

// autofillRunApp mounts /me/autofill/run behind the same RequireAuthOrKey mw.key uses in
// production, on a handler with no DB. The refusal cases below reject before any query
// runs, so the nil autofillHandlers fields are never dereferenced.
func autofillRunApp(iss *auth.Issuer, keys auth.APIKeyAuthenticator) *fiber.App {
	app := fiber.New()
	h := &autofillHandlers{}
	app.Post("/api/v1/me/autofill/run", auth.RequireAuthOrKey(iss, testVersions, keys), h.RunAgentAutofill)
	return app
}

// RunAgentAutofill attaches to the caller's browsertools.Hub channel unconditionally —
// the same channel read_current_page reaches, but this one WRITES into whatever form the
// caller's extension currently has open. Hub is keyed by user id, not session id, so
// without this guard a website user (cookie) or a full-scope API key could trigger it
// against a page they never opened on this surface. See the
// confine-browse-preset-to-extension change.
func TestRunAgentAutofillRefusesNonExtensionAuth(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	const apiKey = "fhk_test-key"
	keys := stubValidKey{hash: auth.HashAPIKey(apiKey), userID: 7}

	cases := []struct {
		name   string
		cookie bool
		bearer string // "" = no Authorization header
	}{
		{"website cookie", true, ""},
		{"API key", false, apiKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/autofill/run", nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			resp, err := autofillRunApp(iss, keys).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusForbidden {
				t.Errorf("status = %d, want 403 (autofill must run only from the extension's own connection)", resp.StatusCode)
			}
		})
	}
}

// emptyProfileSources satisfies every reader the autofill profile is assembled from with
// "this candidate has stated nothing". The mapping tests below are about what a FAILED
// run answers, and the profile only has to exist for the run to start.
type emptyProfileSources struct{}

func (emptyProfileSources) BaseCV(context.Context, int64) (cv.Record, bool, error) {
	return cv.Record{}, false, nil
}

func (emptyProfileSources) Structured(context.Context, int64) (resumeextract.Structured, bool, error) {
	return resumeextract.Structured{}, false, nil
}

func (emptyProfileSources) GetUserByID(context.Context, int64) (db.GetUserByIDRow, error) {
	return db.GetUserByIDRow{Email: "candidate@example.test"}, nil
}

func (emptyProfileSources) Get(context.Context, int64) (screeninganswers.Answers, error) {
	return screeninganswers.Answers{}, nil
}

// extensionSocket stands in for the side panel on the other end of the relay: it reads
// the call off the wire and answers it, so a test can decide what the browser "reports"
// for a given tool. Answering inside Send is safe — Hub.Forward releases its lock before
// delivering, and the harness's own reply channel is buffered.
type extensionSocket struct {
	hub     *browsertools.Hub
	user    int64
	result  func(tool string) string // the raw JSON the tool call resolves to
	errText string                   // when set, the socket answers with an error frame instead
}

func (s extensionSocket) Send(frame []byte) error {
	var call struct {
		ID   string `json:"id"`
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(frame, &call); err != nil {
		return err
	}
	answer := fmt.Sprintf(`{"id":%q,"result":%s}`, call.ID, s.result(call.Tool))
	if s.errText != "" {
		// The shape executor.ts produces for anything thrown on the browser's side.
		answer = fmt.Sprintf(`{"id":%q,"error":%q}`, call.ID, s.errText)
	}
	s.hub.Forward(s.user, browsertools.RoleExtension, []byte(answer))
	return nil
}

// A failed run used to be flattened into one 409 carrying the raw error text. Run makes
// two live model calls, so that answer told a candidate whose gateway had 5xx'd, or whose
// call hit our own 90-second deadline, that they had a state conflict — printed the
// internals verbatim in the extension's panel, and kept every one of those faults out of
// Sentry, since RenderError only reports what it classifies as a fault.
func TestRunAgentAutofillAnswersEachFailureAsWhatItIs(t *testing.T) {
	const userID = int64(7)
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	run := func(t *testing.T, attach func(hub *browsertools.Hub)) (int, map[string]any) {
		t.Helper()
		hub := browsertools.New()
		if attach != nil {
			attach(hub)
		}
		h := &autofillHandlers{
			profiles:     candidateprofile.NewAssembler(emptyProfileSources{}, emptyProfileSources{}, emptyProfileSources{}, emptyProfileSources{}),
			browserTools: hub,
		}
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		app.Post("/api/v1/me/autofill/run",
			auth.RequireAuthOrKey(iss, testVersions, stubValidKey{}), h.RunAgentAutofill)

		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/autofill/run", nil)
		req.Header.Set("Authorization", "Bearer "+token) // a session bearer: the extension's own connection
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}

	t.Run("no extension attached is a conflict naming what to do", func(t *testing.T) {
		status, body := run(t, nil)
		if status != fiber.StatusConflict {
			t.Fatalf("status = %d, want 409", status)
		}
		if body["error"] != browsertools.ErrNotConnected.Error() {
			t.Errorf("error = %v, want %q", body["error"], browsertools.ErrNotConnected)
		}
	})

	t.Run("a page with no form is a conflict, not a fault", func(t *testing.T) {
		status, body := run(t, func(hub *browsertools.Hub) {
			hub.Join(userID, browsertools.RoleExtension, extensionSocket{
				hub: hub, user: userID, result: func(string) string { return `{"fields":[]}` },
			})
		})
		if status != fiber.StatusConflict {
			t.Fatalf("status = %d, want 409", status)
		}
		if body["error"] != autofillagent.ErrNoFillableFields.Error() {
			t.Errorf("error = %v, want %q", body["error"], autofillagent.ErrNoFillableFields)
		}
	})

	t.Run("no model configured is unavailable, not a conflict", func(t *testing.T) {
		status, body := run(t, func(hub *browsertools.Hub) {
			hub.Join(userID, browsertools.RoleExtension, extensionSocket{
				hub: hub, user: userID,
				result: func(string) string { return `{"fields":[{"label":"Full name","type":"text"}]}` },
			})
		})
		if status != fiber.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", status)
		}
		if body["error"] != autofillagent.ErrUnavailable.Error() {
			t.Errorf("error = %v, want %q", body["error"], autofillagent.ErrUnavailable)
		}
	})

	// The most common failure of all, and the one the shared mapper must NOT take: the
	// extension answered, and what it said is that the browser could not do it. "No active
	// tab" and a content script Chrome has already discarded arrive here as the executor's
	// own sentence, and both are fixed by the person reading the panel. Answering them with
	// "internal server error" would hide the one thing they could act on and file a report
	// about a tab somebody closed.
	t.Run("what the browser reports is the browser's state, not our fault", func(t *testing.T) {
		for _, reported := range []string{
			"no active tab",
			"Could not establish connection. Receiving end does not exist.",
		} {
			t.Run(reported, func(t *testing.T) {
				status, body := run(t, func(hub *browsertools.Hub) {
					hub.Join(userID, browsertools.RoleExtension, extensionSocket{
						hub: hub, user: userID, errText: reported,
						result: func(string) string { return `{"fields":[]}` },
					})
				})
				if status != fiber.StatusConflict {
					t.Fatalf("status = %d, want 409 — the browser named a state the user can fix", status)
				}
				if msg, _ := body["error"].(string); !strings.Contains(msg, reported) {
					t.Errorf("error = %v, want it to carry %q so the panel can show it", body["error"], reported)
				}
			})
		}
	})

	t.Run("an unreadable reply is a fault, so it reaches the error inbox", func(t *testing.T) {
		status, body := run(t, func(hub *browsertools.Hub) {
			hub.Join(userID, browsertools.RoleExtension, extensionSocket{
				hub: hub, user: userID, result: func(string) string { return `"not a form"` },
			})
		})
		// The shared mapper's answer: 500 with the generic sentence, and — the point of
		// the branch — classify reporting it, which a *fiber.Error never is.
		if status != fiber.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", status)
		}
		if body["error"] != "internal server error" {
			t.Errorf("error = %v, want the generic message rather than our internals", body["error"])
		}
	})
}
