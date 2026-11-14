package sources

import (
	"os"
)

// Taxonomy assembles the registry for classification rather than crawling: every adapter this
// binary knows about, with its markers, whatever credentials the environment holds. Callers
// that ask what KIND of source a provider is — the source facet, the status page's provider
// kind, the cross-source dedup pass's aggregator set — use this, so their answers do not vary
// by host. It carries no transport, so nothing reached through it may Fetch; crawlers call All
// with a real client and get the credential-gated registry instead.
func Taxonomy() map[string]Source { return All(nil) }

// All assembles the registered adapters into a provider-keyed registry, sharing one
// HTTP client across them. Adding a platform is a new adapter plus one line here.
// A nil client builds the transport-free taxonomy registry — call Taxonomy for that.
func All(c HTTPClient) map[string]Source {
	// SEEK, JobStreet and JobsDB share the same frontend/GraphQL infrastructure and our
	// crawl egresses from one IP, so their detail requests must compete for ONE token bucket
	// and share one ban state and run budget (seekDetailNetwork). Two independent networks would
	// silently double the measured safe rate. The proxy lane is attached by ApplyProxyEgress.
	seekNetworkDetail := newSeekDetailNetwork(pacedSeekPoster(c), nil)
	// Alfa-Bank, Positive Technologies and Tochka Bank serve certificates issued by the
	// Russian Trusted Root CA, which no standard trust store carries, so they fail the TLS
	// handshake on the shared client. They share one client that trusts that extra root —
	// see NewRussianTrustedRootClient for why the scope is these adapters and not the fleet.
	russianRoot := NewRussianTrustedRootClient()
	registry := reg(
		NewGreenhouse(c),
		NewLever(c),
		NewAshby(c),
		// Rate-paced (pacedWorkableGetter): all ~4k boards share one metered widget host, and
		// unpaced the run 429s after ~1,500 boards. See pacer.go.
		NewWorkable(pacedWorkableGetter(c)),
		NewWorkableMarketplace(c),
		NewRecruitee(c),
		// Rate-paced (pacedSmartRecruitersGetter): all 5,204 boards share one metered API host,
		// and unpaced the run's own aggregate volume 429s nearly all of them. See pacer.go.
		NewSmartRecruiters(pacedSmartRecruitersGetter(c)),
		NewISmartRecruit(c),
		// Keka: Indian HR/ATS SaaS, board = subdomain; one request to the platform's own
		// job-listing API returns every open posting, fully detailed.
		NewKeka(c),
		NewHireHive(c),
		NewBeehire(c),
		NewInploi(c),
		NewSeamlessHiring(c),
		NewGupy(c),
		// AppOne (Paychex): board = job-portal slug; one listing request per portal, bodies
		// hydrated from the posting API for new postings only. Paced: its edge refuses a fast
		// client with a 403 (see pacer.go).
		NewAppOne(pacedAppOneGetter(c)),
		NewSolides(c),
		NewPersonio(c),
		NewPeopleForce(c),
		NewCatsone(c),
		NewOpencats(c),
		NewOdoo(c),
		NewTalentLyft(c),
		NewPinpoint(c),
		NewRippling(c),
		NewBambooHR(c),
		NewWorkday(c),
		NewHuntflow(c),
		// FriendWork: Russian ATS, board = career-portal token; one keyless JSON request returns
		// the whole list with bodies. Run sequentially at a measured pace (see pacer.go).
		NewFriendWork(limitedFriendWorkGetter(c)),
		// Potok: Russian ATS, board = tenant subdomain; one server-rendered listing page, bodies
		// hydrated from posting pages for new postings only.
		NewPotok(limitedPotokGetter(c)),
		NewInhire(c),
		NewHiBob(c),
		NewGem(c),
		NewSuccessFactors(c),
		NewSelfRecruit(c),
		// Paced: its per-posting detail fan-out fired ~37k requests in 10 minutes and Teamtailor
		// 403'd nearly half the fleet (see teamtailorRequestInterval).
		NewTeamtailor(pacedTeamtailorGetter(c)),
		NewHurma(c),
		NewICIMS(c),
		// careerspage is rate-paced (pacedHTMLGetter); the proxied path paces it too.
		NewCareerPage(pacedHTMLGetter(c, careerspageRequestInterval, careerspageRequestBurst)),
		NewCleverstaff(c),
		NewNorthstone(c),
		NewBriefHQ(c),
		NewDjinni(c),
		NewTalentAdore(c),
		NewLoxo(c),
		NewHireology(c),
		// Rate-paced: api.higherme.com 403s an IP over a per-window budget (see pacer.go).
		NewHigherMe(pacedHigherMeGetter(c)),
		NewIsolvedHire(c),
		NewApplicantPro(c),
		NewPaylocity(c),
		NewJibe(c),
		// Rate-paced (pacedPhenomGetter): all ~380 boards share Phenom People's platform
		// infrastructure, and the run's own aggregate volume 403s a large fraction of them
		// unpaced. See pacer.go.
		NewPhenom(pacedPhenomGetter(c)),
		NewAvature(c),
		NewComeet(c),
		NewCornerstone(c),
		NewRadancy(c),
		NewJazzHR(c),
		NewWPYoast(c),
		NewBreezy(c),
		NewHerp(c),
		NewTrinetHire(c),
		// Rate-paced (gohirePacedGetter): the platform answers 429 to a modest burst and keeps
		// refusing for minutes. See gohire.go.
		NewGoHire(gohirePacedGetter(c)),
		NewHrmos(c),
		NewJoin(pacedJoinGetter(c)),
		NewRapyd(c),
		NewCareerPlug(c),
		NewHRMDirect(c),
		// Rate-paced (pacedIdealTraitsGetter): every board shares one host that refuses an IP
		// for minutes once its budget is spent. See pacer.go.
		NewIdealTraits(pacedIdealTraitsGetter(c)),
		NewAppliTrack(c),
		// Paced per board to robots.txt's Crawl-delay (each tenant is its own host); see
		// applicantstack.go.
		NewApplicantStack(c),
		NewPaycom(c),
		NewPaycor(c),
		NewLuxoft(c),
		NewEPAM(c),
		// Rate-paced (pacedADPGetter): all ~2,800 boards share one ADP host, and the run's own
		// aggregate volume 429s most of them unpaced. See pacer.go.
		NewADP(pacedADPGetter(c)),
		// ADP MyJobs: ADP's OTHER career-site product, addressed by site slug rather than by
		// Workforce Now's cid+ccId pair, so a company on it is unreachable through NewADP above.
		// Rate-paced on a limiter of its OWN, not the one above — see pacedADPMyJobsGetter for
		// why sharing would be an assertion nothing has measured.
		NewADPMyJobs(pacedADPMyJobsGetter(c)),
		NewITechArt(c),
		NewVention(c),
		// Detail hydration is rate-paced (pacedHTMLGetter) to hold the run's request rate
		// under ClinchTalent's per-IP AWS-WAF challenge window; the sitemap fetch is a single
		// request and needs no pacing.
		NewClinch(c, pacedHTMLGetter(c, clinchRequestInterval, clinchRequestBurst)),
		NewOracle(c),
		NewEightfold(c),
		NewFreshteam(c),
		NewSoftgarden(c),
		// TalentBrew (Radancy): white-labelled onto each employer's own host, so the board is the host
		// (plus locale path). Unpaced: the few requests made while probing four boards drew no limit
		// and the platform documents none; detail hydration runs under the shared 8-worker bound.
		NewTalentbrew(c),
		// Recruiter (recruiter.co.kr): Saramin's career-site builder. One listing POST per board
		// plus one detail GET per open posting against a shared API host; boards are small
		// (42 postings on the largest measured) and no limit was observed, so it is unpaced.
		NewRecruiterCoKR(c),
		// Abacus Umantis: board is the careers host (recruitingapp-<n>.umantis.com or a customer's
		// vanity domain). Unpaced: tenants are separate hosts, each small (tens of postings), so the
		// run's load never concentrates on one origin the way a shared-host ATS does. See umantis.go.
		NewUmantis(c),
		NewBetterteam(c),
		// White-labelled ATS: the board is the full careers host, because the same tenant is
		// served under the vendor's domain and under each reseller's.
		NewHiringThing(c),
		// Intervieweb (Zucchetti): the board is the careers host. Unpaced: the listing is one
		// landing page plus one POST per further page (12 for the largest sample board) and
		// bodies are hydrated for new postings only; no rate limit was observed or documented.
		NewIntervieweb(c),
		// HCS Hiring: symplr's US healthcare career centers, <tenant>.hcshiring.com. A keyless
		// JSON listing ten rows a page; the body is hydrated per new posting. See hcshiring.go.
		NewHCSHiring(c),
		NewEarcu(c),
		NewPageUp(c),
		NewNeogov(c),
		NewDeel(c),
		NewScalis(c),
		NewVouch(c),
		NewRecruitingSolutions(c),
		// UKG Pro Recruiting's edge 403s the project's bare User-Agent on every path (the
		// listing API included) while serving a recognised crawler string — see crawlerUASource —
		// and the adapter re-fetches every posting's detail page per run, so it is paced too
		// (pacedUKGHTTP, one bucket for the listing POST and the detail GETs).
		crawlerUASource(c, func(h ukgHTTP) Source { return NewUKG(pacedUKGHTTP(h)) }),
		// UKG Ready (formerly Kronos Workforce Ready) — a different UKG product from the one
		// NewUKG crawls, on its own regional pod hosts.
		NewUKGReady(c),
		// Workstream: the hourly/frontline ATS (restaurants, retail, care homes), board = the
		// employer's eight-hex career-site id. Rate-paced on every path: its AWS API Gateway
		// origin meters by request rate per IP (see workstreamRequestInterval).
		NewWorkstream(pacedHTMLGetter(c, workstreamRequestInterval, workstreamRequestBurst)),
		// jobappnetwork (talentReef): another hourly/frontline-leaning ATS, board = the
		// employer's numeric client id from its own public apply-link URL shape.
		NewJobAppNetwork(c),
		NewSenior(c),
		NewTrakstar(c),
		// gethired: board = the tenant subdomain on easyapply.co, one RSS feed per board.
		NewGetHired(c),
		// taleobe: Taleo Business Edition — NOT Taleo Enterprise ("taleo" below), board =
		// "<ORG>:<cws>", one RSS feed per board reached through the platform's dispatcher.
		NewTaleoBE(c),
		NewFactorial(c),
		// gr8people: another vendor served under two marketing domains (gr8people.com and
		// workgr8.com — see atsboard's matching two rows), board = the tenant's whole host.
		NewGr8People(c),
		// werecruit: board = "<locale>/<tenant>", the locale is load-bearing (see AGENTS.md).
		NewWerecruit(c),
		NewZoho(c),
		NewTraffit(c),
		NewErecruiter(c),
		// Recruitify (Poland): board = the <label>.recruitify.ai portal. One API host answers every
		// portal by Origin, so the provider shares one pace (see recruitifyRequestInterval).
		NewRecruitify(pacedRecruitifyGetter(c)),
		NewQuickin(c),
		NewSpark(c),
		NewMindsight(c),
		NewEnlizt(c),
		NewJobvite(c),
		NewBullhorn(c),
		NewManatal(c),
		NewJobscore(c),
		NewCrelate(c),
		// Ashby boards whose public Posting API is disabled, served via the embed GraphQL.
		NewAshbyGraphQL(c),
		// VC talent network, board = portfolio company slug: listed only for the early-stage
		// companies that host their application there instead of on an ATS of their own.
		NewSpeedrun(c),
		// Multi-company aggregators (boardless): one global feed, company per posting.
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
		// Paced (pacedDoverGetter): an unpaced multi-board run drew 22 429s from 31 boards
		// within ~2s (see doverRequestInterval's incident note in pacer.go).
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
		NewRemoteOK(c),
		NewJobicy(c),
		// devglobaljobs rates list and detail requests at 120 req/min per IP; paced
		// (pacedDevGlobalJobsGetter) so pagination and detail hydration share the bucket.
		NewDevGlobalJobs(pacedDevGlobalJobsGetter(c)),
		NewWeWorkRemotely(c),
		// Cutshort: Indian hiring platform, one public syndication XML carrying every
		// posting's body and structured facets inline. City-expanded rows collapse on
		// partnerJobId. See cutshort.go.
		NewCutshort(c),
		NewNoDesk(c),
		NewCryptocurrencyJobs(c),
		NewJobspresso(c),
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
		NewWorkingNomads(c),
		NewPowerToFly(c),
		NewHimalayas(c),
		NewRemotive(c),
		NewRemotedotcom(c),
		// wellfound needs no fingerprint-spoofing transport of its own (unlike bayt/gulftalent
		// below) — its pages refuse every plain request, and the Scrapling tier serves them,
		// wired in scraplingProviders. Without SCRAPLING_FETCH_URL this entry still exists
		// (classification/dedup must know about it) but every crawl attempt simply 403s, the
		// same shape bayt/gulftalent already have.
		NewWellfound(c),
		NewRemotli(c),
		NewLandingJobs(c),
		NewTheMuse(c),
		NewNoFluffJobs(c),
		// Just Join IT (justjoin.it) and its non-IT sibling RocketJobs (rocketjobs.pl): one
		// platform, enumerated from each site's active-jobs sitemap, each new offer read from its
		// page's Next.js flight. See justjoin.go.
		NewJustJoin(c),
		NewRocketJobs(c),
		// Centralna Baza Ofert Pracy (oferty.praca.gov.pl): Poland's public job board, listed whole
		// from the portal's search API, whose rows carry the full offer. See cbop.go.
		NewCBOP(c),
		NewWantapply(c),
		NewInfoJobs(c),
		NewJobtech(c),
		NewJobnet(c),
		NewJobdanmark(c),
		NewJobindex(c),
		NewEmagine(c, limitedEmagineGetter(c)),
		NewTyomarkkinatori(c),
		NewLikeit(c),
		NewArbeitsagentur(c),
		// EURES: the EU's cross-border public employment portal, aggregating national PES
		// and partner-board feeds across ~31 EU/EFTA countries. Board-based (board =
		// country), aggregator-marked (see eures.go for the confirmed re-listing evidence).
		NewEures(c),
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
		// keyless API, enumerated by search keyword (board) because 0.4% of its 80k postings are
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
		// XING Jobs: the DACH professional network's job board, enumerated from its gzip job
		// sitemaps and hydrating each new posting from the Apollo cache its page server-renders.
		// See xing.go.
		NewXing(c),
		// jobs.ch / jobup.ch: JobCloud's two Swiss national boards, one posting pool enumerated from
		// both job sitemaps plus the newest date-sorted search pages, each new posting's body read
		// from its page's server-rendered state. See jobsch.go.
		NewJobsCh(c),
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
		// RU-domestic single-company adapters (boardless, except Yandex which selects
		// host+language by board).
		NewYandex(c),
		// Yandex Crowd: gig/support catalogue at crowd.yandex.ru, one JSON blob per landing page.
		NewYandexCrowd(c),
		NewOzon(c),
		NewAvito(c),
		// Kaspi.kz (Kazakhstan): the same sitemap + JobPosting ld+json shape as Avito.
		NewKaspi(c),
		// Halyk Bank (Kazakhstan): one HTML listing, then a page per vacancy. The path
		// number is a list position, not an id — see halyk.go.
		NewHalyk(c),
		NewX5(c),
		NewRWB(c),
		NewSber(c),
		// Uzum (Uzbekistan): sitemap + per-vacancy RSC flight; see uzum.go.
		NewUzum(c),
		NewAlfaBank(russianRoot),
		// Positive Technologies and Tochka Bank: their postings exist only on their own
		// Next.js careers sites, read from the server-rendered pages.
		NewPtsecurity(russianRoot),
		NewTochka(russianRoot),
		NewLamoda(c),
		NewKuper(c),
		NewAviasales(c),
		NewDodo(c),
		NewDomclick(c),
		NewMtslink(c),
		// www.tbank.ru serves a certificate chaining to the Russian Trusted Root CA too, and its
		// sitemaps are slow to serve, hence the longer timeout (see tbankTimeout).
		NewTBank(newRussianTrustedRootClient(tbankTimeout)),
		NewMTS(c),
		// MegaFon walks its listing city by city (the unfiltered feed's order is unstable and
		// deterministically hides ~14% of postings) and hydrates bodies; see megafon.go.
		NewMegafon(c),
		NewRostelecom(c),
		NewVK(c),
		NewTwoGIS(c),
		// Kaspersky: no public API; the listing's RSC flight carries the structured fields
		// and the posting page the body. See kaspersky.go.
		NewKaspersky(c),
		// Kontur: one listing page plus a JobPosting ld+json per vacancy. Proxied — see proxy.go.
		NewKontur(c),
		// Magnit Tech: the retail group's IT arm, a keyless JSON API (list → detail).
		NewMagnit(c),
		NewSelectel(c),
	)
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
	return registry
}
