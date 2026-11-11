package handler

import "github.com/gofiber/fiber/v2"

// mountJobsContract registers the jobs routes through the contract, the way Register does.
// A middleware the test leaves nil passes the request on.
func mountJobsContract(app *fiber.App, h *jobsHandlers, mw middleware) {
	pass := func(c *fiber.Ctx) error { return c.Next() }
	for _, m := range []*fiber.Handler{&mw.optional, &mw.key, &mw.moderator} {
		if *m == nil {
			*m = pass
		}
	}
	h.register(app.Group("/api/v1"), mw, newContract(contractHandlers{jobsHandlers: h}))
}

// mountCompaniesContract registers the companies routes through the contract.
func mountCompaniesContract(app *fiber.App, h *companiesHandlers) {
	pass := func(c *fiber.Ctx) error { return c.Next() }
	h.register(app.Group("/api/v1"), middleware{optional: pass}, newContract(contractHandlers{companiesHandlers: h}))
}
