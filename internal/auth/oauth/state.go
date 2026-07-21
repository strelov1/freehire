package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// StateCookieName carries the CSRF state between the start redirect and the
// provider callback. Lax is enough: the callback is a top-level GET
// navigation, on which Lax cookies are sent.
const StateCookieName = "hire_oauth_state"

// ReturnCookieName remembers where to send the browser after a successful
// sign-in, so signing in from a deep page returns there instead of the home
// page. It rides the same Lax, short-lived round-trip as the state cookie.
const ReturnCookieName = "hire_oauth_return"

// PlatformCookieName remembers that a sign-in was started by the mobile app
// (`?platform=mobile`), so the callback finishes with a custom-scheme deep link
// carrying a one-time code instead of setting the session cookie and redirecting
// to the web frontend. Rides the same Lax, short-lived round-trip.
const PlatformCookieName = "hire_oauth_platform"

// PlatformMobile is the only recognized platform value; anything else means the
// default web flow.
const PlatformMobile = "mobile"

// stateTTL bounds how long a started sign-in stays completable. Ten minutes
// covers a slow consent screen without leaving stale states around.
const stateTTL = 10 * time.Minute

// NewState returns a fresh URL-safe random state value.
func NewState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetStateCookie stores the state for the upcoming callback to verify.
func SetStateCookie(c *fiber.Ctx, state string, secure bool) {
	writeCookie(c, StateCookieName, state, time.Now().Add(stateTTL), secure)
}

// ClearStateCookie removes the state cookie (the state is single-use).
func ClearStateCookie(c *fiber.Ctx, secure bool) {
	writeCookie(c, StateCookieName, "", time.Now().Add(-time.Hour), secure)
}

// SetReturnCookie remembers a (pre-validated) return path for the callback.
func SetReturnCookie(c *fiber.Ctx, path string, secure bool) {
	writeCookie(c, ReturnCookieName, path, time.Now().Add(stateTTL), secure)
}

// ClearReturnCookie removes the return cookie (single-use, like the state).
func ClearReturnCookie(c *fiber.Ctx, secure bool) {
	writeCookie(c, ReturnCookieName, "", time.Now().Add(-time.Hour), secure)
}

// SetPlatformCookie records the initiating platform for the callback to read.
func SetPlatformCookie(c *fiber.Ctx, platform string, secure bool) {
	writeCookie(c, PlatformCookieName, platform, time.Now().Add(stateTTL), secure)
}

// ClearPlatformCookie removes the platform cookie (single-use, like the state).
func ClearPlatformCookie(c *fiber.Ctx, secure bool) {
	writeCookie(c, PlatformCookieName, "", time.Now().Add(-time.Hour), secure)
}

// writeCookie is the single place these short-lived sign-in cookies get their
// attributes, so set and clear can't drift apart (same pattern as the session
// cookie).
func writeCookie(c *fiber.Ctx, name, value string, expires time.Time, secure bool) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

// SafeReturnPath validates a post-login return target the SPA supplies. It
// accepts only same-origin relative paths so the open redirect can't bounce a
// user to an attacker's site; anything else (absolute URL, scheme-relative
// "//host", non-rooted, or unparseable) collapses to "/". The query is kept;
// scheme and host are never echoed back.
func SafeReturnPath(raw string) string {
	const fallback = "/"
	if raw == "" {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return fallback
	}
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return fallback
	}
	out := u.EscapedPath()
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}
