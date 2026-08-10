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
// provider callback. Most providers' callback is a top-level GET navigation,
// where Lax cookies are sent; Apple's is a cross-site POST, which needs
// SameSite=None instead — see writeCookie.
const StateCookieName = "hire_oauth_state"

// ReturnCookieName remembers where to send the browser after a successful
// sign-in, so signing in from a deep page returns there instead of the home
// page. It rides the same short-lived round-trip as the state cookie.
const ReturnCookieName = "hire_oauth_return"

// PlatformCookieName remembers that a sign-in was started by the mobile app
// (`?platform=mobile`), so the callback finishes with a custom-scheme deep link
// carrying a one-time code instead of setting the session cookie and redirecting
// to the web frontend. Rides the same short-lived round-trip.
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
	SetStateCookieNamed(c, StateCookieName, state, secure)
}

// ClearStateCookie removes the state cookie (the state is single-use).
func ClearStateCookie(c *fiber.Ctx, secure bool) {
	ClearStateCookieNamed(c, StateCookieName, secure)
}

// SetStateCookieNamed is SetStateCookie under a caller-chosen cookie name, for
// OAuth round-trips that must not clobber the sign-in state cookie (e.g. the
// Gmail connect flow, which a signed-in user can start mid-sign-in of another
// tab — one shared cookie name would overwrite the other flow's state).
func SetStateCookieNamed(c *fiber.Ctx, name, state string, secure bool) {
	writeCookie(c, name, state, time.Now().Add(stateTTL), secure)
}

// ClearStateCookieNamed removes a named state cookie (single-use, like the state).
func ClearStateCookieNamed(c *fiber.Ctx, name string, secure bool) {
	writeCookie(c, name, "", time.Now().Add(-time.Hour), secure)
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
//
// SameSite is None (not Lax) when secure, because Apple's callback arrives as
// a cross-site POST (response_mode=form_post) — browsers only attach a Lax
// cookie to a cross-site *top-level GET* navigation, never to a cross-site
// POST, so a Lax state cookie never reaches the server on Apple's callback
// and every Apple sign-in fails with a false "state mismatch". None is
// rejected by browsers unless Secure is set, so an insecure (http, dev-only)
// deployment keeps Lax instead of silently losing the cookie altogether;
// Google/GitHub/LinkedIn's GET callbacks are unaffected either way, since Lax
// already covers cross-site top-level GETs and None is a strict superset.
func writeCookie(c *fiber.Ctx, name, value string, expires time.Time, secure bool) {
	sameSite := fiber.CookieSameSiteLaxMode
	if secure {
		sameSite = fiber.CookieSameSiteNoneMode
	}
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: sameSite,
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
