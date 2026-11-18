package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CookieName is the httpOnly cookie that carries the JWT; only RequireAuth reads it.
const CookieName = "hire_token"

// BrowserOrigins returns the canonical frontend origin, cookie-domain HTTPS apexes
// and explicitly served HTTPS hosts. CORS and session writers share this policy.
// Served hosts are exact hostnames; wildcard and scheme/path entries are ignored.
func BrowserOrigins(frontendOrigin string, cookieDomains []string, servedHosts ...string) []string {
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
	for _, host := range servedHosts {
		host = strings.TrimSpace(host)
		if host == "" || strings.ContainsAny(host, "/*") {
			continue
		}
		origins = append(origins, "https://"+strings.ToLower(host))
	}
	return origins
}

// SetTokenCookie writes the auth cookie. SameSite=Lax blocks it on cross-site requests,
// covering CSRF for the current endpoints. secure comes from config for dev http. domain
// is empty in dev (host-only) and ".freehire.me" in prod for unified SSO.
func SetTokenCookie(c *fiber.Ctx, token string, ttl time.Duration, secure bool, domain string) {
	writeTokenCookie(c, token, time.Now().Add(ttl), secure, domain)
}

// ClearTokenCookie expires the auth cookie (logout). A `.freehire.me` clear cannot remove
// a leftover host-only cookie, so with a Domain configured it also emits a host-only expiry.
func ClearTokenCookie(c *fiber.Ctx, secure bool, domain string) {
	expired := time.Now().Add(-time.Hour)
	writeTokenCookie(c, "", expired, secure, domain)
	if domain != "" {
		// fasthttp dedups Set-Cookie by name, so the host-only clear goes out as a raw header.
		raw := CookieName + "=; Path=/; Expires=" + expired.UTC().Format(http.TimeFormat) + "; HttpOnly; SameSite=Lax"
		if secure {
			raw += "; Secure"
		}
		c.Response().Header.Add("Set-Cookie", raw)
	}
}

// writeTokenCookie is the one place the cookie attributes are set, so set and clear match
// (a browser only overwrites a cookie with matching attributes). A blank Domain is host-only.
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
