package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// deletedAccountVersions stands in for the token-version read against an account that
// no longer exists: the row is gone, so the lookup errors.
type deletedAccountVersions struct{}

func (deletedAccountVersions) GetUserTokenVersion(context.Context, int64) (int32, error) {
	return 0, pgx.ErrNoRows
}

// A token is a claim about who the caller is, not proof that they still exist.
// Sessions are stateless, so a cookie held on another device outlives the account it
// names — deletion has to be visible at the gate, not several queries later, where it
// would surface as a foreign-key 500 instead of a 401.
//
// No new machinery backs this: the revocation check already fails closed on an
// unreadable token version, and a deleted account is the extreme case of unreadable.
// The test pins that consequence so a later "tolerate a version-load hiccup" change
// cannot quietly re-admit deleted accounts.
func TestRequireAuth_RejectsTokenForDeletedAccount(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	resp, err := versionedApp(iss, deletedAccountVersions{}).Test(cookieRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status for a deleted account = %d, want 401", resp.StatusCode)
	}
}

// The cookie path of RequireAuthOrKey needs the same guarantee. The key path does not:
// api_keys cascade with the account, so a deleted user's key stops resolving on its own.
func TestRequireAuthOrKey_RejectsCookieForDeletedAccount(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	// No key matches, so the cookie is the only credential in play.
	app.Get("/protected", RequireAuthOrKey(iss, deletedAccountVersions{}, fakeKeyAuth{}), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	resp, err := app.Test(cookieRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status for a deleted account's cookie = %d, want 401", resp.StatusCode)
	}
}
