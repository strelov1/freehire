package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// StateCookieName carries the CSRF state from the start redirect to the provider callback.
const StateCookieName = "hire_oauth_state"

// ReturnCookieName remembers where to send the browser after sign-in.
const ReturnCookieName = "hire_oauth_return"

// PlatformCookieName remembers a sign-in started by the mobile app (`?platform=mobile`),
// so the callback ends with a deep link carrying a one-time code instead of a cookie.
const PlatformCookieName = "hire_oauth_platform"

// PlatformMobile is the only recognized platform value; anything else is the web flow.
const PlatformMobile = "mobile"

// stateTTL bounds how long a started sign-in stays completable.
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

// SetStateCookieNamed is SetStateCookie under a caller-chosen name, for flows that must
// not clobber the sign-in state cookie (e.g. Gmail connect in another tab).
func SetStateCookieNamed(c *fiber.Ctx, name, state string, secure bool) {
	writeCookie(c, name, state, time.Now().Add(stateTTL), secure)
}

// ClearStateCookieNamed removes a named state cookie.
func ClearStateCookieNamed(c *fiber.Ctx, name string, secure bool) {
	writeCookie(c, name, "", time.Now().Add(-time.Hour), secure)
}

// SetReturnCookie remembers a (pre-validated) return path for the callback.
func SetReturnCookie(c *fiber.Ctx, path string, secure bool) {
	writeCookie(c, ReturnCookieName, path, time.Now().Add(stateTTL), secure)
}

// ClearReturnCookie removes the return cookie.
func ClearReturnCookie(c *fiber.Ctx, secure bool) {
	writeCookie(c, ReturnCookieName, "", time.Now().Add(-time.Hour), secure)
}

// SetPlatformCookie records the initiating platform for the callback to read.
func SetPlatformCookie(c *fiber.Ctx, platform string, secure bool) {
	writeCookie(c, PlatformCookieName, platform, time.Now().Add(stateTTL), secure)
}

// ClearPlatformCookie removes the platform cookie.
func ClearPlatformCookie(c *fiber.Ctx, secure bool) {
	writeCookie(c, PlatformCookieName, "", time.Now().Add(-time.Hour), secure)
}

// writeCookie is the one place these sign-in cookies get their attributes, so set and
// clear match.
//
// SameSite is None when secure: Apple's callback is a cross-site POST (form_post), which
// never carries a Lax cookie and would fail with a false "state mismatch". Browsers
// reject None without Secure, so insecure dev keeps Lax.
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

// SafeReturnPath validates a post-login return target. Only same-origin relative paths
// pass, preventing an open redirect; anything else (absolute, "//host", non-rooted,
// unparseable) collapses to "/". The query is kept.
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
