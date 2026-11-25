package sources

// registerLongTimeoutTransports adds the adapters whose answers outlast the standard 15s client
// timeout and so ride the long-timeout transport (registerSpecialTransports' slow group). It
// follows All's rule: on the taxonomy path (c == nil) every provider registers, transport-free.
func registerLongTimeoutTransports(registry map[string]Source, c HTTPClient) {
	// These adapters ride one long-timeout client, built once per registry and shared the
	// way the shared client is: a 429 holds back its own HOST only (hostbackoff.go), and these
	// are all different platforms, so one holds table costs nothing and saves ten transports.
	var longTimeout *Client
	if c != nil {
		longTimeout = NewLongTimeoutClient()
	}
	// Empregos.com.br's search API answers up to 15,000 whole postings at once (a very large
	// response), past the standard 15s client timeout. See empregos.go.
	if c == nil {
		registry["empregos"] = NewEmpregos(nil)
	} else {
		registry["empregos"] = NewEmpregos(longTimeout)
	}
	// DigitalRecruiters (Cegid, France): board = the careers HOST, almost always the employer's
	// own domain, read through the vendor's public JSON API at one request a second. A 1,000-posting
	// listing page can take tens of seconds, past the standard 15s client timeout.
	// See digitalrecruiters.go.
	if c == nil {
		registry["digitalrecruiters"] = NewDigitalrecruiters(nil)
	} else {
		registry["digitalrecruiters"] = NewDigitalrecruiters(longTimeout)
	}
	// Prospective (JobCloud, Switzerland): board = a MEDIUM id, read through the vendor's public
	// JSON API at one request a second, each new job's employer from its page's JobPosting. A
	// listing page of 1,000 can take over a minute, so pages are 200 on the long-timeout
	// client. See prospective.go.
	if c == nil {
		registry["prospective"] = NewProspective(nil, nil)
	} else {
		registry["prospective"] = NewProspective(longTimeout, longTimeout)
	}
	// HumanBit's listing page is a single, unpaginated multi-MB payload that can take ~18s to
	// fetch, past the standard 15s client timeout ("context deadline exceeded"). Swap in the
	// long-timeout transport only when there is a real client to serve; the taxonomy path
	// (c == nil, e.g. FilterableProviders) must stay transport-free, per the invariant
	// Taxonomy documents.
	if c == nil {
		registry["humanbit"] = NewHumanBit(nil)
	} else {
		registry["humanbit"] = NewHumanBit(longTimeout)
	}
	// Jobboardly: hosted niche job boards, board = tenant subdomain. A board's whole catalogue
	// is one unpaginated /jobs.json rendered on request, and a large board
	// can take ~28s to its first byte, past the standard 15s
	// client timeout, the HumanBit case again. See jobboardly.go.
	if c == nil {
		registry["jobboardly"] = NewJobboardly(nil)
	} else {
		registry["jobboardly"] = NewJobboardly(longTimeout)
	}
	// HR-Manager: Danish ATS, board = customer alias, one job-portal API request per board with
	// every advertisement body inline. The largest customer's answer is
	// ~12 MB and can take 25s on a slow line, past the standard 15s timeout, the HumanBit case again. See hrmanager.go.
	if c == nil {
		registry["hrmanager"] = NewHRManager(nil)
	} else {
		registry["hrmanager"] = NewHRManager(longTimeout)
	}
	// Tenstreet: US trucking carriers' Pulse pages, board = numeric company id, hydrating each
	// new posting from its page. Posting pages are large and some stall for 30 s or more,
	// so the adapter rides the long-timeout transport, paced at a rate that drew no refusals (see tenstreetRequestInterval).
	if c == nil {
		registry["tenstreet"] = NewTenstreet(nil)
	} else {
		registry["tenstreet"] = NewTenstreet(newTenstreetClient())
	}
	// Paradox: frontline-hiring ATS, board = "<host>/<site>". The listing API is slow per
	// row, so a 100-row page can take ~11 s, too close to the standard 15 s timeout — and the
	// API meters a per-IP request budget, so the run is paced. See paradox.go.
	if c == nil {
		registry["paradox"] = NewParadox(nil)
	} else {
		registry["paradox"] = NewParadox(pacedParadoxGetter(longTimeout))
	}
	// HireClick serves its pages uncompressed slowly while they weigh
	// 1-3 MB (the tenant logo inlined as base64, plus an ASP.NET view state), so a board page
	// can take 13-40s and a posting page ~11s, past the standard 15s timeout, the HumanBit
	// case again. Every request is also held under one in-flight cap (limitedHireClick): the
	// origin is one small pipe that degraded under a default-fan-out crawl. See hireclick.go.
	if c == nil {
		registry["hireclick"] = NewHireClick(nil)
	} else {
		registry["hireclick"] = NewHireClick(limitedHireClick(longTimeout))
	}
	// HerKey's posting resource answers in 4-15 s, close enough to the standard 15 s timeout
	// that a share of every run would time out, so it rides the long-timeout transport, paced
	// (see herkeyRequestInterval).
	if c == nil {
		registry["herkey"] = NewHerKey(nil)
	} else {
		registry["herkey"] = NewHerKey(pacedHerKeyGetter(longTimeout))
	}
	// ESS: the Employment Service of Slovenia's job search, read through the public JSON API its
	// search page calls, each new vacancy's body from the detail endpoint. A listing page of 500
	// is slow to answer, near the standard 15s client timeout. See ess.go.
	if c == nil {
		registry["ess"] = NewESS(nil)
	} else {
		registry["ess"] = NewESS(longTimeout)
	}
}
