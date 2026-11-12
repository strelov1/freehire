package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// codeCrossSiteRefusal is the machine-readable code on the gate's refusals. It
// rides the repo's standard coded-error envelope ({"error", "code"}), and the
// SPA classifies on this code — not on the bare status — so an unrelated 403
// is never mistaken for the gate's answer.
const codeCrossSiteRefusal = "cross_site_refused"

// Session-issuing routes are browser forms reached same-origin by the SPA (dev
// reaches them through the Vite proxy). SameSite=Lax governs whether an EXISTING
// cookie rides along; it says nothing about a NEW cookie a response issues, so a
// top-level cross-site form POST to /auth/login would otherwise store an
// attacker-supplied session in the victim's browser (login CSRF).
//
// requireSameOrigin binds these routes to the request's origin. A present Origin
// decides: allowed (auth.BrowserOrigins) passes, anything else refuses. With
// Origin absent, a present Referer decides the same way. When BOTH are absent
// the request presents the headerless non-browser shape (curl, the CLI, the
// documented JSON API) and is accepted — unless its own Sec-Fetch-Site header
// says another site initiated it, which refuses.
//
// Known limit: a request carrying none of the three headers is indistinguishable
// from the non-browser shape and is accepted; the policy explicitly accepts that
// missing-metadata shape rather than claiming it cannot occur.
//
// Every refusal is 403 with code cross_site_refused BEFORE the handler runs, so
// a refused login never verifies credentials and never sets a cookie.
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

// requestIsCrossSite reports whether the request's Sec-Fetch-Site header says
// another site initiated the request. An absent or unknown value is not
// cross-site; the header and its value live as locals so the gate owns its
// declarations outright (a sibling patch declares its own Fetch-Metadata
// constants at package level).
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
