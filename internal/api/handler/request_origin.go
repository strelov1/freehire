package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// codeCrossSiteRefusal is the code on the gate's refusals ({"error", "code"} envelope); the
// SPA classifies on it, not on the bare 403.
const codeCrossSiteRefusal = "cross_site_refused"

// requireSameOrigin binds session-issuing routes to the request's origin. SameSite=Lax says
// nothing about a NEW cookie a response issues, so a cross-site form POST to /auth/login would
// otherwise store an attacker's session in the victim's browser (login CSRF).
//
// A present Origin decides: allowed (auth.BrowserOrigins) passes, anything else refuses. With
// Origin absent, a present Referer decides the same way. With BOTH absent the request is the
// headerless non-browser shape (curl, CLI, JSON API) and is accepted unless Sec-Fetch-Site says
// another site initiated it.
//
// Known limit: a browser request carrying none of the three headers is indistinguishable from
// that shape and is accepted.
//
// Every refusal is 403 cross_site_refused BEFORE the handler runs, so a refused login never
// verifies credentials or sets a cookie.
func (h *authHandlers) requireSameOrigin() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		if err := h.checkSameOrigin(c); err != nil {
			return err
		}
		return c.Next()
	}
}

// checkSameOrigin is the gate's verdict alone, for a handler that must apply it to
// one branch of its own (the consent screens' "switch account").
func (h *authHandlers) checkSameOrigin(c *fiber.Ctx) error {
	if origin := strings.TrimSpace(c.Get(fiber.HeaderOrigin)); origin != "" {
		if originAllowed(origin, h.browserOrigins) {
			return nil
		}
		return authError(fiber.StatusForbidden, codeCrossSiteRefusal, "cross-site request refused")
	}
	if referer := strings.TrimSpace(c.Get(fiber.HeaderReferer)); referer != "" {
		if refererAllowed(referer, h.browserOrigins) {
			return nil
		}
		return authError(fiber.StatusForbidden, codeCrossSiteRefusal, "cross-site request refused")
	}
	if requestIsCrossSite(c) {
		return authError(fiber.StatusForbidden, codeCrossSiteRefusal, "cross-site request refused")
	}
	return nil
}

// requestIsCrossSite reports whether Sec-Fetch-Site says another site initiated the
// request. An absent or unknown value is not cross-site.
func requestIsCrossSite(c *fiber.Ctx) bool {
	const headerSecFetchSite = "Sec-Fetch-Site"
	const valueCrossSite = "cross-site"
	return strings.EqualFold(strings.TrimSpace(c.Get(headerSecFetchSite)), valueCrossSite)
}

// originAllowed reports whether an Origin header value is one of the browser
// origins this deployment serves. An unparseable or host-only value never matches.
func originAllowed(origin string, browserOrigins []string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	for _, allowed := range browserOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

// refererAllowed reports whether a Referer header value starts with one of the
// browser origins this deployment serves. Prefix-matched on purpose: a Referer is
// a full URL (origin + path + query), and only its origin part carries site
// identity.
func refererAllowed(referer string, browserOrigins []string) bool {
	if referer == "" {
		return false
	}
	u, err := url.Parse(referer)
	if err != nil || u.Host == "" {
		return false
	}
	for _, allowed := range browserOrigins {
		if len(referer) >= len(allowed) && strings.EqualFold(referer[:len(allowed)], allowed) {
			// The next character must be the path separator (or the value ends
			// there), so https://evil.example-freehire.me cannot share the
			// https://freehire.me prefix.
			if len(referer) == len(allowed) || referer[len(allowed)] == '/' {
				return true
			}
		}
	}
	return false
}
