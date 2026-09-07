package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
)

// The public read API is unauthenticated and, until this existed, unbounded:
// every other limiter in this package guards an auth route, a write, or an LLM
// spend, and nginx applies its own limit_req to the SvelteKit pages and not to
// /api/. Both ceilings below sit above the highest per-IP minute measured on
// production over a 4.6-hour window, so they bound abuse without cutting off any
// caller that exists today.
//
// The split is by cost, not by path. `/agent/jobs/search` rehydrates every hit's
// full description from Postgres — ~833 KB and ~1.3s at limit=100, against
// ~123 KB and ~0.55s for the ordinary search — so one shared budget would have
// to be sized for it and would then throttle facet lookups seven times harder
// than their cost warrants.
const (
	// publicReadsPerMinute covers the cheap reads. 600/min is 10 r/s, deliberately
	// the same ceiling nginx already applies to the HTML pages, so the site has one
	// number to explain rather than two. Measured peak: 258/min.
	publicReadsPerMinute = 600

	// agentSearchPerMinute covers the one expensive read. Measured peak: 184/min,
	// held steadily by a single third-party client, so this leaves that caller room
	// to grow by two thirds before it ever sees a 429.
	agentSearchPerMinute = 300

	// suggestPerMinute covers the search box's completions. The split here is by
	// FREQUENCY rather than by cost: each call is cheap — a query against a
	// dictionary of tens of thousands of documents, not the 8M-document catalogue —
	// but the box issues one per settled keystroke, so a visitor composing a query
	// spends ten of these where the same visitor spends one ordinary read. Sharing
	// the public-read budget would mean typing a query throttles the page that
	// answers it.
	//
	// 1200/min is 20 r/s per caller, which is faster than anybody types and far
	// short of what a scraper would want from an endpoint that returns no postings.
	suggestPerMinute = 1200
)

// All three below are keyed by IP, and deliberately: on a PUBLIC read there is no
// authenticated caller to key by.
//
// They used to ask for user-or-IP and say so, and the branch was dead. Fiber runs a
// route's handlers in registration order, and every public read registers its limiter
// BEFORE the optional-auth gate (jobs.go, companies.go, search.go) or without a gate at
// all (geo.go, suggest.go) — there is no group-level auth, handler.go builds /api/v1 with
// no .Use. So `auth.UserID` read a local nothing had written yet and the key was always
// the IP form. Fifty signed-in colleagues behind one office NAT already shared one budget;
// the comment claiming otherwise was the only thing that changed.
//
// Moving the gate in front would have fixed three of the ten routes and put a database
// lookup ahead of throttling on the hottest reads on the site — OptionalAuth answers 503
// when a Bearer-key lookup fails, so it is not free — against a host where crawlers are
// most of the traffic. Deleting the unreachable branch is the smaller answer, and the
// allowance was never per-user in production anyway.
//
// The tests below mount the gate FIRST, so an authenticated caller really is
// authenticated when the limiter runs. That is what makes them a guard: they fail if
// anyone reintroduces a user-keyed budget here, whatever the ordering.

// publicReadLimiter throttles the cheap public reads as one shared budget.
func publicReadLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("publicread"), publicReadsPerMinute, time.Minute)
}

// agentSearchLimiter throttles the full-description search on its own budget, so
// exhausting it leaves the ordinary search endpoints serving.
func agentSearchLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("agentsearch"), agentSearchPerMinute, time.Minute)
}

// suggestLimiter throttles the search box's completions on their own budget, so a
// visitor typing quickly cannot exhaust the allowance the rest of the site reads on.
func suggestLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("suggest"), suggestPerMinute, time.Minute)
}
