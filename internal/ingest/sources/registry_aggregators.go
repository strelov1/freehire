package sources

import (
	"os"

	"github.com/strelov1/freehire/internal/ingest/sources/aggregators"
)

// aggregatorSources are the multi-company aggregators and national job boards, one
// registry entry each (see All). seekNetworkDetail is the detail network SEEK and
// JobStreet share.
func aggregatorSources(c HTTPClient, seekNetworkDetail *seekDetailNetwork) []Source {
	return []Source{
		// Multi-company aggregators (boardless): one global feed, company per posting.
		NewLinkedIn(c),
		NewTecla(c),
		NewTeamex(c),
		NewTopco(c),
		NewGetmatch(c),
		NewGetmanfred(c),
		// Joppy: Spain-only tech board with no search API, walked via its own sitemap directory —
		// every company's page carries that company's open postings in full, no detail request.
		NewJoppy(c),
		NewEchoJobs(c),
		NewHabrCareer(c),
		NewGeekjob(c),
		NewGetro(c),
		// LiveHire: talent platform behind employers like Flybuys, board = the career-site
		// segment (/careers/<segment>). The JSON API under /careers-api answers without the
		// Cloudflare challenge that fronts the HTML pages. Hydrating: the listing carries the
		// facets, the detail carries the body. See livehire.go.
		NewLiveHire(c),
		// Firefish: UK recruitment CRM behind employers like Arnold Clark, board = the tenant's
		// whole careers host (careers.arnoldclark.com or <tenant>.firefishsoftware.com). Every site
		// publishes one RSS document carrying every open vacancy whole, with body and structured
		// facets — one GET per board, no detail request. See firefish.go.
		NewFirefish(c),
		// Dover: boarded ATS (board = company slug), public JSON API with no bot protection on
		// the data endpoints — only the browser apply-submission flow gates behind Turnstile.
		// Paced (pacedDoverGetter): an unpaced multi-board run draws 429s (see
		// doverRequestInterval in pacer.go).
		NewDover(pacedDoverGetter(c)),
		NewJobylon(c),
		// Emply: Danish multi-tenant ATS, one career site per board (<slug>.career.emply.com or
		// a customer domain). One keyless JSON listing carries every vacancy with its body.
		// See emply.go.
		NewEmply(c),
		// Harri: hospitality ATS, board = a brand's career-page slug (harri.com/<slug>). A
		// keyless JSON search lists the brand tree's postings; bodies are hydrated per posting,
		// paced (pacedHarri). See harri.go.
		NewHarri(pacedHarri(c)),
		NewWorkAtAStartup(c),
		NewJobStash(c),
		NewArbeitnow(c),
		// devglobaljobs rates list and detail requests at 120 req/min per IP; paced
		// (pacedDevGlobalJobsGetter) so pagination and detail hydration share the bucket.
		NewDevGlobalJobs(pacedDevGlobalJobsGetter(c)),
		// Cutshort: Indian hiring platform, one public syndication XML carrying every
		// posting's body and structured facets inline. City-expanded rows collapse on
		// partnerJobId. See cutshort.go.
		NewCutshort(c),
		NewCryptocurrencyJobs(c),
		NewStartupAndVC(c),
		// Hacker News "Ask HN: Who is hiring?": the two newest monthly threads, read whole
		// through the Algolia HN API — one global feed, company per comment.
		NewHackerNews(c),
		browserUASource(c, NewFourDayWeek),
		NewFunctionalWorks(c),
		NewTheHub(c),
		NewCompleo(c),
		NewInstaffo(c),
		NewTechTree(c),
		NewGetonbrd(c),
		NewVagas(c),
		// GeekHunter: Brazilian tech-recruitment ATS, board = company slug; listing and detail
		// both come from parsing the page's own schema.org ld+json blocks.
		NewGeekHunter(c),
		// Recrutei: Brazilian multi-tenant ATS, board = tenant slug; listing is one POST to the
		// platform's own internal frontend API, detail comes from the page's ld+json block.
		NewRecrutei(c),
		// PyjamaHR: Indian multi-tenant ATS, board = tenant slug; listing and detail are both
		// plain keyless GETs to the platform's own internal frontend API (api.pyjamahr.com).
		NewPyjamahr(c),
		// RecruiterFlow: recruiting-agency board = agency slug; the whole listing is embedded
		// as a bare JS variable on the page, detail comes from the page's ld+json block.
		NewRecruiterflow(c),
		NewMyCareersFuture(c),
		NewCareersGov(c),
		NewPowerToFly(c),
		// wellfound needs no fingerprint-spoofing transport of its own (unlike bayt/gulftalent
		// below) — its pages refuse every plain request, and the Scrapling tier serves them,
		// wired in scraplingProviders. Without SCRAPLING_FETCH_URL this entry still exists
		// (classification/dedup must know about it) but every crawl attempt simply 403s, the
		// same shape bayt/gulftalent already have.
		NewWellfound(c),
		NewLandingJobs(c),
		NewTheMuse(c),
		aggregators.NewNoFluffJobs(c),
		// Just Join IT (justjoin.it) and its non-IT sibling RocketJobs (rocketjobs.pl): one
		// platform, enumerated from each site's active-jobs sitemap, each new offer read from its
		// page's Next.js flight. See justjoin.go.
		NewJustJoin(c),
		NewRocketJobs(c),
		// Centralna Baza Ofert Pracy (oferty.praca.gov.pl): Poland's public job board, listed whole
		// from the portal's search API, whose rows carry the full offer. See cbop.go.
		NewCBOP(c),
		NewWantapply(c),
		NewWantapplyCy(c),
		NewInfoJobs(c),
		aggregators.NewJobtech(c),
		aggregators.NewJobnet(c),
		NewJobdanmark(c),
		NewJobindex(c),
		NewEmagine(c, limitedEmagineGetter(c)),
		NewTyomarkkinatori(c),
		NewLikeit(c),
		NewArbeitsagentur(c),
		// EURES: the EU's cross-border public employment portal, aggregating national PES
		// and partner-board feeds across EU/EFTA countries. Board-based (board =
		// country), aggregator-marked (see eures.go for the confirmed re-listing evidence).
		aggregators.NewEures(c),
		// International single-company adapters (boardless).
		NewTelegramCareers(c),
		NewAmazon(c),
		NewGoogle(c),
		NewApple(c),
		// IBM: careers.ibm.com sits behind an AWS WAF challenge, so the catalogue — bodies
		// included — is read from IBM's own site-search API instead. See ibm.go.
		NewIBM(c),
		// Tencent: careers.tencent.com, the mainland-China catalogue (international hiring is
		// Tencent's Workday tenant), read through the site's Technology category and hydrated
		// from its posting API. See tencent.go.
		NewTencent(c),
		// Alibaba: the group's experienced-hire portal, every business under one search, read
		// through its Technology category behind a self-minted CSRF token. See alibaba.go.
		NewAlibaba(c),
		// Meituan: zhaopin.meituan.com, read through its Technology job family for experienced,
		// campus and intern postings alike. See meituan.go.
		NewMeituan(c),
		// Garena: one keyless listing POST plus a detail GET per posting, which alone holds
		// the responsibilities and requirements. See garena.go.
		NewGarena(c),
		// The Taiwanese hardware makers' own career sites, each a keyless endpoint the site's
		// own page reads. MediaTek and Foxconn list the duties and hydrate each new posting for
		// its requirements, Realtek lists no body at all, ASUS pages its listing five at a
		// time, and Quanta is one dump. See each adapter's file.
		NewMediaTek(c),
		NewRealtek(c),
		NewFoxconn(c),
		NewASUS(c),
		NewQuanta(c),
		// Dassault Systèmes: its Taleo section is not public, so the catalogue — bodies
		// included — is read from 3ds.com's own Exalead card search. See dassault.go.
		NewDassault(c),
		NewLumenalta(c),
		// Staffy: a single recruiting agency's own board, boardless like Lumenalta; both the
		// listing and detail pages are fully static server-rendered HTML.
		NewStaffy(c),
		NewOrion(c),
		// airswift's detail pages 429 under an unpaced burst (see airswiftRequestInterval);
		// the sitemap does not, so it stays on the shared client.
		NewAirswift(c, pacedHTMLGetter(c, airswiftRequestInterval, airswiftRequestBurst)),
		NewDataArt(c),
		// energyjobline's detail pages need the Googlebot UA (see crawlerUserAgent) and are
		// rate-paced (pacedHTMLGetter, see energyjoblineRequestInterval); the sitemap does
		// not, so it stays on the shared client.
		NewEnergyJobline(c, pacedHTMLGetter(newCrawlerUAClient(), energyjoblineRequestInterval, energyjoblineRequestBurst)),
		// Dynamite Jobs: remote-only, every function. The listing API is robots-disallowed, so
		// the sitemaps enumerate postings and each page's JobPosting ld+json carries the body.
		// Pages are rate-paced; the sitemaps stay on the shared client. See dynamitejobs.go.
		NewDynamiteJobs(c, pacedHTMLGetter(c, dynamitejobsRequestInterval, dynamitejobsRequestBurst)),
		// JettyCloud: its postings exist only on its own careers site — the employer runs no
		// ATS a crawl can reach, so this adapter is the only way the catalogue can carry it.
		NewJettyCloud(c),
		// 10ML: a Shopify storefront that publishes its openings only on its own careers page,
		// each detail page carrying a JobPosting ld+json. See tenml.go.
		NewTenML(c),
		NewOnstrider(c, os.Getenv("ONSTRIDER_REFERRAL_HANDLE")),
		NewAlignerr(c, os.Getenv("ALIGNERR_REFERRAL_CODE")),
		NewTalentHR(c),
		NewJobspage(c),
		// Rate-paced: every onlyfy site shares one platform edge that 429s the run's aggregate rate
		// (see onlyfyRequestInterval).
		NewOnlyfy(pacedHTMLGetter(c, onlyfyRequestInterval, onlyfyRequestBurst)),
		NewPolymer(c),
		NewFlatchr(c),
		NewDvinci(c),
		NewRexx(c),
		NewCoveto(limitedCovetoGetter(c)),
		NewConcludis(c),
		NewKenjo(c),
		// Concurrency-limited: one API host answers every Oorwin portal (see oorwinMaxInFlight).
		NewOorwin(limitedOorwinPoster(c)),
		NewEasyCruit(c),
		// Rate-paced: one origin serves every Homerun feed and vacancy page (see homerunRequestInterval);
		// the first refusal stops the run's Homerun traffic until ApplyProxyEgress gives it the
		// rotating lane (see homerunEgress).
		NewHomerun(pacedHomerunGetter(newHomerunEgress(c, nil))),
		NewHROffice(c),
		// CVWarehouse (Belgium/Portugal): board = companyGuid on jobpage.cvwarehouse.com. Paced: one
		// host serves every customer's pages (see cvwarehouseRequestInterval).
		NewCVWarehouse(pacedHTMLGetter(c, cvwarehouseRequestInterval, 1)),
		// Bizneo HR (Spain): board = the career-site host under bizneo.cloud; bodies hydrated for
		// new postings only. Paced: every site is served by one edge (see bizneoRequestInterval).
		NewBizneo(pacedHTMLGetter(c, bizneoRequestInterval, 1)),
		// ePreselec (Spain, Adevinta): board = the <label>.epreselec.com site; Imperva-fronted, so
		// on the crawl host it is rebuilt over the Scrapling tier (scraplingtier.go).
		NewEPreselec(c),
		// Viterbit (Spain): board = the <label>.viterbit.site career site; Cloudflare-challenged, so
		// on the crawl host it is rebuilt over the Scrapling tier (scraplingtier.go).
		NewViterbit(c),
		// Talent Clue (Spain): board = the customer's 32-hex client id the platform's widget reads
		// its listing with; bodies hydrated for new postings only, at robots.txt's 10 s crawl delay.
		NewTalentclue(c),
		// Velora HR (Spain): board = the numeric career-site id its public API is keyed by; the
		// listing carries every body, so a crawl is two requests per board.
		NewVelorahr(c),
		// Talentsoft / Cegid Talent (France): board = the careers HOST (<tenant>.talent-soft.com,
		// <tenant>.profils.org or the employer's own domain); paced per site and across the vendor,
		// bodies hydrated for new postings only.
		NewTalentsoft(c),
		NewGreytHR(c),
		// Concurrency-limited: one origin serves every helixjobs iid (see helixjobsMaxInFlight).
		NewHelixjobs(limitedHelixjobsGetter(c)),
		NewTalentstorm(c),
		NewMicro1(c, os.Getenv("MICRO1_REFERRAL_CODE")),
		NewBairesDev(c, os.Getenv("BAIRESDEV_REFERRER_CODE")),
		// RU federal open-data aggregator: board-based, sharded per region (board = OKATO code).
		// Concurrency-limited: its gov API degrades under the pipeline's board concurrency (500s +
		// read-timeouts), so its in-flight requests are capped to a gentle few.
		NewTrudvsem(limitedTrudvsemGetter(c)),
		// hh.ru: multi-company aggregator, enumerated by professional_role (board), reading the
		// server-rendered search page's embedded state.
		NewHH(c),
		// SolidJobs: Polish job board, multi-company aggregator enumerated by division (board),
		// same keyword-as-board shape as whatjobs.
		NewSolidJobs(c),
		// SchoolSpring: the US K-12 education job board, multi-company aggregator over one central
		// keyless API, enumerated by search keyword (board) because few of its postings are
		// technical and the platform's own category taxonomy cannot select them. Hydrating.
		NewSchoolSpring(c),
		// SEEK: the dominant AU/NZ job board, multi-company aggregator enumerated by ICT
		// subclassification (board) with the market in region, reading its frontend search API and
		// hydrating descriptions from its GraphQL endpoint. Keyless.
		// Its GraphQL detail endpoint meters by a per-IP request budget, so only that path is
		// rate-paced; the search listing stays on the bare client.
		NewSeek(c, seekNetworkDetail),
		NewJobStreet(c, seekNetworkDetail),
		// Japan Dev: curated Japan technology aggregator. One public sitemap lists posting URLs;
		// detail pages carry structured Nuxt SSR state and official ATS apply links when available.
		NewJapanDev(c),
		// sitemapldjson: board is a sitemap URL (plain XML or .xml.gz, index walked
		// to a fixed depth). Region is an optional URL glob. Pages with no JobPosting
		// are skipped. Hydrating, with a per-run cap on new detail fetches.
		NewSitemapLDJSON(c),
		// Get in IT: German entry-level IT board. The public sitemap names the whole catalogue;
		// posting pages carry a schema.org JobPosting. Paced, see getinit.go.
		NewGetInIT(c),
		// Berlin Startup Jobs: Berlin startup board. post-sitemap.xml names the whole catalogue;
		// posting pages carry a schema.org JobPosting. Paced, see berlinstartupjobs.go.
		NewBerlinStartupJobs(c),
		// DSML Kazakhstan: the Kazakh data/ML community board. sitemap.xml names the whole
		// catalogue; posting pages carry a schema.org JobPosting. Paced, see dsml.go.
		NewDSML(c),
		// ITviec: Vietnam's IT-only board. One server-rendered listing names the whole
		// catalogue; posting pages carry a schema.org JobPosting. Paced, see itviecRequestInterval.
		NewITviec(pacedHTMLGetter(c, itviecRequestInterval, itviecRequestBurst)),
		// Jumpit: Saramin's IT-only board for South Korea. One JSON listing request names the whole
		// catalogue; a per-posting detail carries the body. Paced, see jumpitRequestInterval.
		NewJumpit(pacedJumpitGetter(c)),
		// WORK180: board of women-endorsed employers, enumerated by regional site (board =
		// au/uk/us) through each region's jobs sitemap; posting pages carry a schema.org
		// JobPosting. Pages are paced to robots.txt's Crawl-delay, see work180RequestInterval.
		NewWORK180(c, pacedHTMLGetter(c, work180RequestInterval, work180RequestBurst)),
		// Python.org: the Python Software Foundation's own job board. One server-rendered listing
		// names the whole catalogue; posting pages carry the body. See pythonorg.go.
		NewPythonOrg(c),
		// Torre: Latin-American talent marketplace. The sitemap index names the whole live
		// catalogue; posting pages carry a schema.org JobPosting. See torre.go.
		NewTorre(c),
		// OCC Mundial: Mexico's general job board, enumerated by the site's own job category
		// (board) and hydrating each new posting from the JSON its split view loads. The search
		// pages refuse the project's bare User-Agent, hence the browser-compatible form of it.
		// See occ.go.
		compatibleUASource(c, NewOCC),
		// Catho: Brazil's largest general job board, enumerated from its job sitemaps and
		// hydrating each new posting from its page's ld+json. Posting pages refuse the
		// project's bare User-Agent, hence the browser-compatible form of it. See catho.go.
		compatibleUASource(c, NewCatho),
		// Computrabajo: the general job board in each Spanish-speaking Latin American country,
		// board = the country subdomain ("ar"), walked province by province under its 500-page
		// pager cap, each new posting's body read from its page. Its edge refuses the project's
		// bare User-Agent. See computrabajo.go.
		compatibleUASource(c, NewComputrabajo),
		// Cake: Taiwan's tech-leaning job board, enumerated through its search sliced by its own
		// facets and hydrating each new posting from the detail API. The crawl hosts are
		// Cloudflare-challenged, so it egresses through the rotating proxy (proxy.go). See cake.go.
		NewCake(c),
		// 104 Job Bank: Taiwan's largest job board, enumerated by the platform's own area code
		// (board) and read from its search, whose rows carry the whole posting. Both crawl hosts
		// are refused by Cloudflare, so production reaches it through the Scrapling tier
		// (scraplingtier.go). See job104.go.
		NewJob104(c, c),
		// Glints: the Southeast Asian job platform (Indonesia, Vietnam, Singapore, Malaysia),
		// enumerated from its job sitemaps and scoped to the technical postings by title; each new
		// posting's page carries the rest. The site refuses a plain client, so production reaches
		// it through the Scrapling tier (scraplingtier.go). See glints.go.
		NewGlints(c),
		// XING Jobs: the DACH professional network's job board, enumerated from its gzip job
		// sitemaps and hydrating each new posting from the Apollo cache its page server-renders.
		// See xing.go.
		NewXing(c),
		// jobs.ch / jobup.ch: JobCloud's two Swiss national boards, one posting pool enumerated from
		// both job sitemaps plus the newest date-sorted search pages, each new posting's body read
		// from its page's server-rendered state. See jobsch.go.
		NewJobsCh(c),
		NewClearanceJobs(c),
		NewCVOnline(c),
		pacedCVbankas(c),
		// onapply: a German ATS whose vendor-wide job board exposes one keyless search index
		// over every tenant, each hit carrying the whole posting. See onapply.go.
		NewOnapply(c),
		// Workwise: a German recruiting platform whose one posting sitemap names the whole
		// catalogue; each new posting is hydrated from its page's ld+json. See workwise.go.
		NewWorkwise(c),
		// Dutch public boards: the central government (werkenvoornederland), the municipalities
		// (gemeentebanen) and the universities and research institutes (academictransfer). Each
		// is one posting sitemap; every new posting is hydrated from its page's ld+json.
		NewWerkenVoorNederland(c),
		NewGemeentebanen(c),
		NewAcademicTransfer(c),
		NewJobsACUK(c),
		pacedCharityJob(c),
		// CHECK-IN (mein-check-in.de): a German applicant portal of savings banks, cooperative
		// banks and councils; a board is the tenant slug and each new position is hydrated from
		// its page's microdata. See meincheckin.go.
		NewMeinCheckIn(c),
		// InfoJobs Brasil: a general Brazilian board, one board per federative unit plus
		// "todo-brasil". There is no sitemap: each state listing is walked through the site's own
		// pager, which needs a session cookie, and each new posting hydrated from its page. See
		// infojobsbr.go.
		cookieSessionSource(c, NewInfoJobsBR),
		// Tech in Asia: Southeast Asian startup board. One public Algolia index holds the whole
		// catalogue, bodies included; the search key is read from the site's app bundle. The
		// site's edge refuses any self-identifying User-Agent on paths robots.txt allows, hence
		// the browser string. See techinasia.go.
		browserUASource(c, NewTechInAsia),
		// Golang Jobs: niche board of Go developer postings. A client-rendered shell over a
		// Supabase table read through PostgREST with the public anon key, both read from the
		// site's app bundle on every run. See golangjobs.go.
		NewGolangJobs(c),
		// Welcome to the Jungle: European multi-company board, enumerated by WTTJ's own
		// profession sub-category (board) over its public Algolia index, hydrating each new
		// posting's body and apply URL from the posting API. See wttj.go.
		NewWTTJ(c),
		// Kalibrr: Southeast Asian multi-company board, enumerated by country market (board)
		// and always read through its "IT and Software" job function. One JSON search request
		// per board carries every posting with its body. See kalibrr.go.
		NewKalibrr(c),
		// WUZZUF: Egypt's general multi-company board, enumerated by the platform's own job
		// role (board) from the site's search pages — behind a Cloudflare challenge, so the
		// Scrapling tier's browser reads them (scraplingtier.go). See wuzzuf.go.
		NewWuzzuf(c),
		// Feishu Jobs: ByteDance's Feishu Hire ATS, one tenant careers host per board
		// (jobs.bytedance.com, <tenant>.jobs.feishu.cn), read through the tenant's R&D
		// category. The edge admits only a macOS browser string. See feishujobs.go.
		macBrowserUASource(c, NewFeishuJobs),
		// MokaHR: Chinese multi-tenant ATS, one tenant careers site (<orgId>/<siteId>) per
		// board; app.mokahr.com encrypts its listing, hire-r1 (region "r1") does not. See
		// mokahr.go.
		NewMokaHR(c),
		// BeeSite: milch & zucker's German corporate career portals, one <name>.app.beesite.de
		// search host per board, hydrating bodies the search omits from each posting page. See
		// beesite.go.
		NewBeeSite(c),
		// Eploy: UK ATS, one careers host per board; the vacancy list is the host's own
		// /live-jobs.xml sitemap, the body the vacancy page. See eploy.go.
		NewEploy(c),
		// HiringRoom: LatAm ATS, one <tenant>.hiringroom.com portal per board, listed through the
		// portal's own JSON pager and hydrated from each vacancy page. See hiringroom.go.
		NewHiringRoom(c),
		// Pandapé: InfoJobs Brasil's ATS, one <tenant>.pandape.infojobs.com.br career site per
		// board, listed through the site's own JSON pager and hydrated from each vacancy page.
		// One origin serves every tenant, so the adapter paces the whole run. See pandape.go.
		NewPandape(c),
		// Pandapé LatAm: the same ATS sold by Computrabajo across Spanish-speaking Latin America,
		// one <tenant>.pandape.computrabajo.com career site per board, a separate tenant
		// namespace from the Brazilian one. Its edge answers 403 to "freehire/0.1" and serves
		// the compatible form of the same identity. See pandape.go.
		compatibleUASource(c, NewPandapeLatam),
		// Built In: US-centred tech board over many employers. Each tech category's last-24h
		// window is walked; posting pages carry a schema.org JobPosting. Coverage-gated and
		// paced, see builtinRequestInterval.
		NewBuiltIn(pacedHTMLGetter(c, builtinRequestInterval, builtinRequestBurst)),
		// Dice: US job board over many employers. The last-24h window of its own search page is
		// cut into slices under the search's 750-result ceiling; posting pages carry a schema.org
		// JobPosting. Coverage-gated and paced, see diceRequestInterval and dice.go.
		NewDice(pacedHTMLGetter(c, diceRequestInterval, diceRequestBurst)),
		// Levtech Freelance: a Japanese IT freelance project board, boardless. The whole open
		// catalogue is the search's leading ~63 pages (closed projects follow it); each new
		// posting's body is read from its page's schema.org JobPosting. Paced, see levtech.go.
		NewLevtech(pacedHTMLGetter(c, levtechRequestInterval, levtechRequestBurst)),
		// EDJOIN: California's K-12 education board, multi-company aggregator enumerated by
		// job type (board) over one central index, hydrating bodies from each posting page's
		// schema.org block. The board is a job type and not a district on purpose — see
		// edjoin.go.
		NewEdjoin(c),
		// Profession.hu: Hungary's dominant job board, multi-company aggregator enumerated
		// by the platform's own category sitemap (board) and hydrating each posting page.
		// The board is a category and not an employer on purpose — the two IT categories
		// are the technical slice of a general-population board. See profession.go.
		// It gets a client of its own: the platform mishandles a connection it has
		// already answered on, which fails the crawl outright and — worse — sometimes
		// answers with a truncated sitemap instead. See singleUseConnSource.
		singleUseConnSource(c, NewProfession),
	}
}
