package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CookieName is the httpOnly cookie that carries the JWT. The SPA never reads
// it (it can't — HTTPOnly); the browser attaches it automatically on same-site
// requests, and RequireAuth reads it server-side.
const CookieName = "hire_token"

// BrowserOrigins returns the origins of the sites that serve the browser app and
// therefore the origins a same-origin request from it can name: the canonical
// frontend origin plus, for every configured cookie domain, its https apex. It is
// the ONE place that pair is turned into origins — the CORS middleware's allowlist
// (handler.go) and the request-origin gate on session-issuing routes
// (handler.requireSameOrigin) both consume this list, so the two policies cannot
// drift apart. Values are normalized the way cookie domains are written in config:
// a leading "." on a domain is dropped, a trailing "/" on the frontend origin is
// dropped, empty entries are skipped.
//
// It lives here rather than in the handler package because it is the cookie-domain
// configuration — the exact set of sites the session cookie is scoped to — read as
// origins.
func BrowserOrigins(frontendOrigin string, cookieDomains []string) []string {
	var origins []string
	if o := strings.TrimSuffix(strings.TrimSpace(frontendOrigin), "/"); o != "" {
		origins = append(origins, o)
	}
	for _, d := range cookieDomains {
		d = strings.TrimPrefix(strings.TrimSpace(d), ".")
		if d == "" {
			continue
		}
		origins = append(origins, "https://"+d)
	}
	return origins
}

// SetTokenCookie writes the auth cookie. SameSite=Lax (with a same-origin
// deployment) sends it on the app's own requests while blocking it on
// cross-site ones, which covers CSRF for the current endpoints. secure comes
// from config so dev (http://localhost) and HTTPS deployments both work. domain
// scopes the cookie: empty in dev (host-only cookie); ".freehire.me" in prod so
// freehire.me and apply.freehire.me send the same cookie (unified SSO).
func SetTokenCookie(c *fiber.Ctx, token string, ttl time.Duration, secure bool, domain string) {
	writeTokenCookie(c, token, time.Now().Add(ttl), secure, domain)
}

// ClearTokenCookie expires the auth cookie (logout). The browser only
// overwrites a cookie whose attributes match, so during a cookie-domain
// migration a `.freehire.me` clear can't remove a leftover host-only cookie
// (minted before COOKIE_DOMAIN was set). So when a Domain is configured, also
// emit a host-only expiry — clearing both scopes makes logout reliable
// regardless of which cookie the browser is still holding.
func ClearTokenCookie(c *fiber.Ctx, secure bool, domain string) {
	expired := time.Now().Add(-time.Hour)
	writeTokenCookie(c, "", expired, secure, domain)
	if domain != "" {
		// fasthttp dedups Set-Cookie by name, so a second `c.Cookie()` for the
		// same name would replace the first. Emit the host-only clear as a raw
		// header so both the `.freehire.me` cookie and any leftover host-only
		// cookie are expired.
		raw := CookieName + "=; Path=/; Expires=" + expired.UTC().Format(http.TimeFormat) + "; HttpOnly; SameSite=Lax"
		if secure {
			raw += "; Secure"
		}
		c.Response().Header.Add("Set-Cookie", raw)
	}
}

// writeTokenCookie is the single place the cookie's attributes are set, so set
// and clear can't drift apart (the browser only overwrites a cookie whose
// attributes match). A blank Domain omits the attribute entirely (host-only).
func writeTokenCookie(c *fiber.Ctx, value string, expires time.Time, secure bool, domain string) {
	c.Cookie(&fiber.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		Expires:  expires,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}
