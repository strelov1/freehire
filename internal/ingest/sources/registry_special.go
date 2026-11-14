package sources

import (
	"os"
)

// registerSpecialTransports adds the providers All cannot list in its plain reg call: the keyed
// sources a credential in the environment gates, and the adapters that need a transport of
// their own (a cookie session, a long timeout, a TLS fingerprint). It follows All's rule: on the
// taxonomy path (c == nil) every provider registers, transport-free.
func registerSpecialTransports(registry map[string]Source, c HTTPClient) {
	// usajobs, reed, whatjobs, adzuna and arc are the keyed sources: each needs a credential read from
	// the environment, never from a board file. The credential gates the CRAWL registry only — a
	// host without it must not register a provider whose every board would fail to authenticate,
	// so its board file fails config validation before a request goes out. The taxonomy path
	// (c == nil, see Taxonomy) registers all four regardless, empty credential included.
	// Conflating the two classified whatjobs — a CPC reseller of first-party ATS postings — as
	// an ATS on every keyless host, so none of its copies was ever suppressed.
	if key := os.Getenv("USAJOBS_API_KEY"); c == nil || key != "" {
		registry["usajobs"] = NewUSAJobs(c, key)
	}
	if key := os.Getenv("REED_API_KEY"); c == nil || key != "" {
		registry["reed"] = NewReed(c, key)
	}
	// Arc's credential is a signed-in browser session: its complete remote-jobs list answers
	// only a request carrying that session's Cookie header (see arc.go). The listing and the
	// public job pages share one paced client, since the session is a person's account.
	if session := os.Getenv(arcSessionEnv); c == nil || session != "" {
		if c == nil {
			registry["arc"] = NewArc(nil, session)
		} else {
			registry["arc"] = NewArc(pacedArc(c), session)
		}
	}
	// France Travail's credential is an OAuth2 client pair issued to an application registered
	// on francetravail.io; both must be set, and the transport must answer the search's 204.
	if id, secret := os.Getenv("FRANCE_TRAVAIL_CLIENT_ID"), os.Getenv("FRANCE_TRAVAIL_CLIENT_SECRET"); c == nil || (id != "" && secret != "") {
		ft, ok := c.(francetravailHTTP)
		if c == nil || ok {
			registry["francetravail"] = NewFranceTravail(ft, id, secret)
		}
	}
	// Adzuna's credential is an app_id/app_key pair rather than a single key; both must be set.
	if appID, appKey := os.Getenv("ADZUNA_APP_ID"), os.Getenv("ADZUNA_APP_KEY"); c == nil || (appID != "" && appKey != "") {
		registry["adzuna"] = NewAdzuna(c, appID, appKey)
	}
	// whatjobs' credential is a publisher id rather than an API key, and it is per-country — one
	// account per market, each under its own provider name (whatjobsMarkets). Rather than one
	// WHATJOBS_<CC>_PUBLISHER_ID variable per market (which would keep growing as markets are
	// added), every account's id lives in the single WHATJOBS_PUBLISHER_IDS variable, a
	// comma-separated "code:id" list keyed by whatjobsMarket.code (e.g. "us:7065,br:7076"). All
	// configured accounts share ONE in-flight cap: the feed rate-limits the pipeline's parallel
	// board crawl by simultaneity (8 of 10 boards 429'd on the first prod run) though it serves
	// sequential requests fine, and that trigger reads as edge-wide rather than per-account, so
	// every provider must contend for the same semaphore rather than each getting its own budget.
	// On the taxonomy path there is nothing to wrap.
	whatjobsIDs := parseWhatJobsPublisherIDs(os.Getenv("WHATJOBS_PUBLISHER_IDS"))
	var wjGetter JSONGetter
	if c != nil {
		wjGetter = limitedWhatJobsGetter(c)
	}
	for _, m := range whatjobsMarkets {
		id := whatjobsIDs[m.code]
		if c == nil {
			registry[m.provider] = NewWhatJobsMarket(nil, id, m.code)
		} else if id != "" {
			registry[m.provider] = NewWhatJobsMarket(wjGetter, id, m.code)
		}
	}
	// taleo and aijobs both need a cookie-persisting session (a searchjobs POST authorized
	// by a careersection GET's session cookie; a CSRF-protected listing POST authorized by
	// a plain GET's csrftoken cookie) rather than the shared jar-less client — see
	// cookieSessionSource.
	registry["taleo"] = cookieSessionSource[taleoHTTP](c, NewTaleo)
	// dayforce needs the same: its listing POST is authorized by a double-submit CSRF pair,
	// so the token a bootstrap GET mints must ride back as BOTH a header and the cookie the
	// jar-less shared client would drop.
	registry["dayforce"] = cookieSessionSource[dayforceHTTP](c, NewDayforce)
	// peoplesoft's result list grows by a form postback PeopleTools accepts only inside the
	// session the search page's GET opened. See peoplesoft.go.
	registry["peoplesoft"] = cookieSessionSource[peoplesoftHTTP](c, NewPeopleSoft)
	// brassring's JSON API answers only inside the session its home page opens: a cookie plus an
	// anti-forgery token and an encrypted session value embedded in that page. See brassring.go.
	// Unpaced: the platform documents no request limit and the detail fan-out is already narrow.
	registry["brassring"] = cookieSessionSource[brassringHTTP](c, NewBrassring)
	// Lesta's listing answers with a 307 to itself that sets a session cookie; without a jar
	// the redirect loops until the client gives up.
	registry["lesta"] = cookieSessionSource[lestaHTTP](c, NewLesta)
	aijobsMaxNewPerRun := aijobsMaxNewPerRunFromEnv()
	registry["aijobs"] = cookieSessionSource[aijobsHTTP](c, func(h aijobsHTTP) Source {
		return NewAijobs(h, aijobsMaxNewPerRun)
	})
	// Civil Service Jobs answers every path with a proof-of-work check whose verdict is a
	// session cookie. See civilservicejobs.go.
	registry["civilservicejobs"] = cookieSessionSource[csjHTTP](c, NewCivilServiceJobs)
	// Empregos.com.br's search API answers up to 15,000 whole postings at once (~35 MB, ~12 s),
	// past the standard 15s client timeout. See empregos.go.
	if c == nil {
		registry["empregos"] = NewEmpregos(nil)
	} else {
		registry["empregos"] = NewEmpregos(NewLongTimeoutClient())
	}
	// DigitalRecruiters (Cegid, France): board = the careers HOST, almost always the employer's
	// own domain, read through the vendor's public JSON API at one request a second. A 1,000-posting
	// listing page measured 6-33 s to answer (2026-10-28), past the standard 15s client timeout.
	// See digitalrecruiters.go.
	if c == nil {
		registry["digitalrecruiters"] = NewDigitalrecruiters(nil)
	} else {
		registry["digitalrecruiters"] = NewDigitalrecruiters(NewLongTimeoutClient())
	}
	// Prospective (JobCloud, Switzerland): board = a MEDIUM id, read through the vendor's public
	// JSON API at one request a second, each new job's employer from its page's JobPosting. A
	// listing page of 1,000 measured 66 s (2026-10-29), so pages are 200 on the long-timeout
	// client. See prospective.go.
	if c == nil {
		registry["prospective"] = NewProspective(nil, nil)
	} else {
		lc := NewLongTimeoutClient()
		registry["prospective"] = NewProspective(lc, lc)
	}
	// HumanBit's listing page is a single, unpaginated ~5.6 MB payload that measured ~18s to
	// fetch — past the standard 15s client timeout, which made every crawl fail
	// intermittently with "context deadline exceeded" in production. Swap in the
	// long-timeout transport only when there is a real client to serve; the taxonomy path
	// (c == nil, e.g. FilterableProviders) must stay transport-free, per the invariant
	// Taxonomy documents.
	if c == nil {
		registry["humanbit"] = NewHumanBit(nil)
	} else {
		registry["humanbit"] = NewHumanBit(NewLongTimeoutClient())
	}
	// Jobboardly: hosted niche job boards, board = tenant subdomain. A board's whole catalogue
	// is one unpaginated /jobs.json rendered on request, and the largest board measured
	// (remotejobs: 1,723 postings, 31 MB) took ~28s to its first byte — past the standard 15s
	// client timeout, the HumanBit case again. See jobboardly.go.
	if c == nil {
		registry["jobboardly"] = NewJobboardly(nil)
	} else {
		registry["jobboardly"] = NewJobboardly(NewLongTimeoutClient())
	}
	// HR-Manager: Danish ATS, board = customer alias, one job-portal API request per board with
	// every advertisement body inline. The largest customer (663 postings) is a 12 MB answer
	// that took 5s from the prod host and 25s from a residential line — past the standard 15s
	// timeout, the HumanBit case again. See hrmanager.go.
	if c == nil {
		registry["hrmanager"] = NewHRManager(nil)
	} else {
		registry["hrmanager"] = NewHRManager(NewLongTimeoutClient())
	}
	// Tenstreet: US trucking carriers' Pulse pages, board = numeric company id, hydrating each
	// new posting from its page. Posting pages run to ~1.8 MB and some stall for 30 s or more,
	// so the adapter rides the long-timeout transport, paced at the rate measured as refused
	// nothing (see tenstreetRequestInterval).
	if c == nil {
		registry["tenstreet"] = NewTenstreet(nil)
	} else {
		registry["tenstreet"] = NewTenstreet(pacedHTMLGetter(NewLongTimeoutClient(), tenstreetRequestInterval, tenstreetRequestBurst))
	}
	// Paradox: frontline-hiring ATS, board = "<host>/<site>". The listing API spends ~50 ms per
	// row, so a 100-row page took up to 11 s — too close to the standard 15 s timeout — and the
	// API meters a per-IP request budget, so the run is paced. See paradox.go.
	if c == nil {
		registry["paradox"] = NewParadox(nil)
	} else {
		registry["paradox"] = NewParadox(pacedParadoxGetter(NewLongTimeoutClient()))
	}
	// HireClick serves its pages uncompressed at ~80-160 KB/s per connection while they weigh
	// 1-3 MB (the tenant logo inlined as base64, plus an ASP.NET view state), so a board page
	// measured 13-40s and a posting page ~11s — past the standard 15s timeout, the HumanBit
	// case again. Every request is also held under one in-flight cap (limitedHireClick): the
	// origin is one small pipe that degraded under a default-fan-out crawl. See hireclick.go.
	if c == nil {
		registry["hireclick"] = NewHireClick(nil)
	} else {
		registry["hireclick"] = NewHireClick(limitedHireClick(NewLongTimeoutClient()))
	}
	// HerKey's posting resource answers in 4-15 s, close enough to the standard 15 s timeout
	// that a share of every run would time out, so it rides the long-timeout transport, paced
	// (see herkeyRequestInterval).
	if c == nil {
		registry["herkey"] = NewHerKey(nil)
	} else {
		registry["herkey"] = NewHerKey(pacedHerKeyGetter(NewLongTimeoutClient()))
	}
	// Naukri: India's largest job board, enumerated by functional area (board) through the
	// search API its Android app calls, hydrating each new posting's body from the posting API.
	// The edge serves only the app's User-Agent and tarpits datacenter IPs, so the crawl is
	// really served once ApplyProxyEgress rebuilds it over the proxy (proxy.go). See naukri.go.
	if c == nil {
		registry["naukri"] = NewNaukri(nil)
	} else {
		registry["naukri"] = NewNaukri(pacedNaukriGetter(withUserAgent(c, naukriUserAgent)))
	}
	// Instahyre: an Indian technical-hiring platform, enumerated by company-size bucket (board)
	// through its JSON search, each new posting's body read from its public page. The pages sit
	// behind Cloudflare, so the crawl is really served once ApplyScraplingEgress rebuilds it over
	// the fetch service (scraplingtier.go); this plain build is what runs without one.
	if c == nil {
		registry["instahyre"] = NewInstahyre(nil, nil)
	} else {
		registry["instahyre"] = pacedInstahyre(c, c)
	}
	// hirist.tech and iimjobs.com: one Indian job platform on two sites, enumerated by category
	// (board) through the API their web app calls, hydrating each new posting's body from the
	// posting API. Every request of a run is paced to one a second. See hirist.go.
	if c == nil {
		registry["hirist"] = NewHirist(nil)
		registry["iimjobs"] = NewIIMJobs(nil)
	} else {
		registry["hirist"] = NewHirist(pacedHiristGetter(c))
		registry["iimjobs"] = NewIIMJobs(pacedHiristGetter(c))
	}
	// engage (en-gage.net): a Japanese ATS, enumerated by occupation category (board) through the
	// cross-tenant search its own site runs on; a search row carries the whole posting, so no
	// detail requests. Paced to one page a second, over a cookie client so the load balancer keeps
	// a walk on one backend node. See engage.go.
	registry["engage"] = cookieSessionSource(c, func(g JSONGetter) Source { return NewEngage(pacedEngageGetter(g)) })
	// マイナビ転職 (Mynavi): a Japanese job board, enumerated by engineer-edition occupation code
	// (board), each new posting's body read from its page's schema.org JobPosting. The origin
	// refuses datacenter IPs, so the crawl is really served once ApplyProxyEgress rebuilds it
	// over the proxy (proxy.go). Paced either way. See mynavi.go.
	registry["mynavi"] = NewMynavi(pacedHTMLGetter(c, mynaviRequestInterval, mynaviRequestBurst))
	// Shine.com: an Indian general job board, enumerated from its job sitemaps, each new
	// posting's facts read from its page's schema.org JobPosting. See shine.go.
	registry["shine"] = NewShine(c)
	// Poslovi Infostud: Serbia's largest general job board, enumerated from its server-rendered
	// search pages, each new posting's body and salary read from its page. See infostud.go.
	registry["infostud"] = NewInfostud(c)
	// SpainJobs.io: an English-language board of Spanish employers' career-page postings,
	// enumerated from its job sitemaps, each new posting read from its page. See spainjobs.go.
	registry["spainjobs"] = NewSpainJobs(c)
	// Infoempleo: a Spanish general job board, enumerated from its paginated listing, each new
	// posting's body read from its page's schema.org JobPosting. See infoempleo.go.
	registry["infoempleo"] = NewInfoempleo(c)
	// Bumeran: Argentina's largest general job board, one posting pool with its sibling ZonaJobs,
	// enumerated from the web app's search API, each new posting's body read from its posting API.
	// See bumeran.go.
	registry["bumeran"] = NewBumeran(c)
	// MojPosao: Alma Career's job board, board = market domain (mojposao.hr Croatia's largest
	// commercial board, mojposao.ba Bosnia's), enumerated from the web app's search API, each new
	// posting's body read from its posting API. See mojposao.go.
	registry["mojposao"] = NewMojPosao(c)
	// Choisir le service public: the French civil service's own job board, enumerated from its
	// paginated offer list (head plus a moving refresh window), each new posting's body read from
	// its page. See servicepublic.go.
	registry["servicepublic"] = NewServicePublic(c)
	// Free-Work: France's IT-only job board (freelance missions and employment), enumerated from
	// its public API, each new posting's body read from its own resource. See freework.go.
	registry["freework"] = NewFreeWork(c)
	// SwissDevJobs: Switzerland's IT-only job board, enumerated from its public jobsLight feed, each
	// new posting's body read from its page's server-rendered state. See swissdevjobs.go.
	registry["swissdevjobs"] = NewSwissDevJobs(c)
	// Cadremploi: France's job board for managers and professionals, enumerated from its paged
	// online-postings sitemap, each new posting's body read from its page's schema.org JobPosting.
	// Behind Cloudflare: crawled through the Scrapling tier (scraplingProviders). See cadremploi.go.
	registry["cadremploi"] = NewCadremploi(c)
	// JobTeaser: the European careers platform of business and engineering schools, one board per
	// posting language, enumerated from its job-ads sitemaps, each new posting's body read from its
	// page's schema.org JobPosting. Pages behind Cloudflare: read through the Scrapling tier
	// (scraplingProviders). See jobteaser.go.
	registry["jobteaser"] = NewJobTeaser(c, c)
	// ITJobs.pt: Portugal's IT-only job board, enumerated from its paginated listing, each new
	// posting's body read from its page's schema.org JobPosting. See itjobs.go.
	registry["itjobs"] = NewITJobs(c)
	// Feina Activa: the Catalan public employment service's job board, read in full from its
	// own JSON listing and streamed page by page. See feinaactiva.go.
	registry["feinaactiva"] = NewFeinaActiva(c)
	// MojeDelo: Slovenia's largest general job board, enumerated from its SPA's own JSON search,
	// each new posting's body read from the posting's API resource. See mojedelo.go.
	registry["mojedelo"] = NewMojeDelo(c)
	// Optius: a Slovenian general job board, walked through its paginated HTML listing one
	// request at a time (it answers 429 to concurrency), each new posting's body read from its
	// page. See optius.go.
	registry["optius"] = NewOptius(c)
	// Turijobs: the Spanish hospitality board, enumerated from its active-offers sitemap; its
	// posting pages need the Scrapling tier (see scraplingProviders), so over this plain client
	// only the sitemap is read. See turijobs.go.
	registry["turijobs"] = NewTurijobs(c, c)
	// Taleez: a French ATS with no per-customer listing, enumerated from its job sitemap, each
	// new posting read from its page's schema.org JobPosting. See taleez.go.
	registry["taleez"] = NewTaleez(c, c)
	// Dualoo: a Swiss ATS for SMEs and municipalities, enumerated from its per-customer sitemaps,
	// each new posting read from its page's schema.org JobPosting. See dualoo.go.
	registry["dualoo"] = NewDualoo(c, c)
	// Refline: a Swiss ATS strong in the public sector, enumerated from the sitemaps robots.txt
	// names, each job read from one publication's schema.org JobPosting. See refline.go.
	registry["refline"] = NewRefline(c, c, c)
	// SAPO Emprego: a Portuguese general job board, enumerated from its offers sitemap, each new
	// posting's facts read from its page's schema.org JobPosting. See sapoemprego.go.
	registry["sapoemprego"] = NewSapoEmprego(c)
	// Pracuj.pl: Poland's largest general job board, enumerated from its server-rendered search
	// in category slices, each new posting's body read from its offer page's __NEXT_DATA__.
	// Behind Cloudflare: crawled through the Scrapling tier (scraplingProviders). See pracuj.go.
	registry["pracuj"] = NewPracuj(c)
	// Net-Empregos: a Portuguese general job board, enumerated from its sitemap and RSS feeds,
	// each new posting's body read from its page's schema.org JobPosting. See netempregos.go.
	registry["netempregos"] = NewNetEmpregos(c)
	// HZZ: the Croatian Employment Service's job exchange, enumerated from its county RSS feeds,
	// each new vacancy's employer and body read from its print page. The site answers nothing
	// without the session cookie its first response sets, hence the cookie client. See hzz.go.
	registry["hzz"] = cookieSessionSource[hzzGetter](c, NewHZZ)
	// posao.hr: a Croatian general job board, walked through its county pages, each new
	// posting's body read from its page; the employment-service vacancies it republishes are
	// HZZ's own and skipped. The display language lives in the session, hence the cookie
	// client. See posaohr.go.
	registry["posaohr"] = cookieSessionSource[posaoHRGetter](c, NewPosaoHR)
	// ESS: the Employment Service of Slovenia's job search, read through the public JSON API its
	// search page calls, each new vacancy's body from the detail endpoint. A listing page of 500
	// takes ~8 s to answer, near the standard 15s client timeout. See ess.go.
	if c == nil {
		registry["ess"] = NewESS(nil)
	} else {
		registry["ess"] = NewESS(NewLongTimeoutClient())
	}
	// NSZ: Serbia's National Employment Service's job search, its server-rendered pages read
	// newest first, each new vacancy's body from its page, all transliterated into Latin
	// script as the site's own "LAT" switch does in the browser. See nsz.go.
	registry["nsz"] = NewNSZ(c)
	// meta/uber/gusto are NOT served by the shared client: Meta's edge 400s the default Go
	// TLS+HTTP/2 fingerprint and Uber's and Gusto's Cloudflare edges challenge it, so all three
	// need the shared Chrome-fingerprint transport (fingerprintHTTP, also used by the
	// bayt/gulftalent aggregators, by jobleads — whose search edge rejects the plain Go
	// fingerprint the same way and serves a Chrome one 200, keyless — and by startupjobs' and
	// gradcracker's Cloudflare-challenged pages, and by darwinbox's careers API). Build it only when there
	// is a real client to serve (the c == nil marker/listing path — e.g. FilterableProviders —
	// must stay transport-free, so these register with a nil client there; Provider()/boardless()
	// never touch it). If the deterministic transport build ever fails, all nine are left
	// unregistered so config validation fails fast on their board entries, rather than
	// registering a client guaranteed to be rejected by the edge.
	if c == nil {
		registry["meta"] = NewMetaCareers(nil)
		registry["bayt"] = NewBayt(nil)
		registry["gulftalent"] = NewGulfTalent(nil)
		registry["uber"] = NewUber(nil)
		registry["gusto"] = NewGusto(nil)
		// jobleads must register on the taxonomy path too: classification (AggregatorProviders,
		// the source facet) reads this registry, and an aggregator it does not know is never
		// suppressed in the cross-source dedup.
		registry["jobleads"] = NewJobleads(nil)
		// startupjobs likewise: it is an aggregator, and the dedup pass must know it as one.
		registry["startupjobs"] = NewStartupJobs(nil, nil, nil, 0)
		registry["gradcracker"] = NewGradcracker(nil, 0)
		registry["darwinbox"] = NewDarwinbox(nil)
	} else if fp, err := newFingerprintHTTP(); err == nil {
		registry["meta"] = NewMetaCareers(fp)
		registry["bayt"] = NewBayt(fp)
		registry["gulftalent"] = NewGulfTalent(fp)
		registry["uber"] = NewUber(fp)
		registry["gusto"] = NewGusto(fp)
		// The search and detail paths are both paced through one limiter per run, wired here
		// so every board competes for the same bucket (see pacedJobleadsPoster).
		registry["jobleads"] = NewJobleads(pacedJobleadsPoster(fp))
		// startup.jobs splits across both transports: its pages sit behind a Cloudflare
		// challenge only the fingerprint passes (paced, since that edge counts requests per
		// IP), while its sitemap CDN and its public Algolia index have no Cloudflare in front
		// and are read over the shared client.
		registry["startupjobs"] = NewStartupJobs(pacedStartupJobsSite(fp), c, c, 0)
		// Every gradcracker request, listing and posting page alike, shares one limiter: the
		// edge counts requests per IP (see gradcrackerRequestInterval).
		registry["gradcracker"] = NewGradcracker(pacedGradcracker(fp), 0)
		// Darwinbox's careers API sits behind Cloudflare, which 403s the plain Go fingerprint.
		registry["darwinbox"] = NewDarwinbox(fp)
	}
}
