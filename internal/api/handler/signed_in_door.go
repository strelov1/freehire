package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// signedInDoor is a second address for the whole /api/v1 surface, open only to a
// caller holding a real credential: /api/v1/me/api/<rest> is served as /api/v1/<rest>.
//
// It exists because of the edge, not the app. Cloudflare challenges traffic from the
// hosting and VPN networks the catalogue-enumeration fleet rents, and a CLI or an agent
// cannot solve a challenge — so a person on a commercial VPN got a bare 403 from every
// public read, the job search above all. The exemption the edge can afford is a PATH
// whose every route refuses an anonymous caller, because then it hands the fleet
// nothing; /api/v1/me/* has been that path since the rule first carved it out. An
// exemption by "carries an Authorization header" cannot be afforded — the public reads
// accept a junk Bearer with a 200 — so the credential has to be checked here, by the
// server, before anything behind the door runs.
//
// Rewriting the path and letting routing continue, rather than restarting it, is
// deliberate: c.RestartRouting would replay the app-wide middleware, counting and
// logging every door request twice. The target route keeps every gate of its own, so
// the door widens nothing — a cookie-only route still refuses a key, a scoped key is
// still refused here. It does not nest: routing only moves forward, and the door sits
// before every /api/v1 route, so /me/api/me/api/... finds no route.
const signedInDoor = "/api/v1/me/api"

// mountSignedInDoor registers the door on app. It must be mounted BEFORE the /api/v1
// routes, which it hands the rewritten request to.
func mountSignedInDoor(app fiber.Router, gate fiber.Handler) {
	app.Use(signedInDoor, func(c *fiber.Ctx) error {
		rest := strings.TrimPrefix(c.Path(), signedInDoor)
		if rest == "" {
			return fiber.ErrNotFound
		}
		// app.Use matches a bare string prefix, so /api/v1/me/apix lands here too.
		if rest[0] != '/' {
			return c.Next()
		}
		c.Path("/api/v1" + rest)
		return gate(c)
	})
}
