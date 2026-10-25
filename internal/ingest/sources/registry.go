package sources

import (
	"os"
	"slices"
	"time"
)

// SelfClosingProviders returns the provider names in reg that manage their own closes and
// must be skipped by the post-run unseen sweep (see selfClosing). cmd/ingest consults this
// when deciding which providers to sweep.
func SelfClosingProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(selfClosing); ok {
			out = append(out, name)
		}
	}
	return out
}

// SelfCloses reports whether src manages its own closes from a change stream (see selfClosing).
// Such a source emits CHANGES rather than its listing, so nothing derived from one crawl's
// emissions — the quiet-board backoff's listing fingerprint, for one — describes what the board
// still lists.
func SelfCloses(src Source) bool {
	_, ok := src.(selfClosing)
	return ok
}

// FullCatalogProviders returns the provider names in reg that list their whole catalogue each
// run and so may be swept by source rather than by crawled company (see fullCatalog).
func FullCatalogProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(fullCatalog); ok {
			out = append(out, name)
		}
	}
	return out
}

// SweepGraceWindows returns the post-run sweep window each adapter in reg declares in place of
// the default — wider for a slice-crawled source that can't yet tell drift from removal, narrower
// for a full-board source whose unseen reading is already evidence (see sweepGrace). cmd/ingest
// consults this when computing a provider's cutoff; a provider absent from the map is swept on
// the default window.
func SweepGraceWindows(reg map[string]Source) map[string]time.Duration {
	out := make(map[string]time.Duration)
	for name, src := range reg {
		if s, ok := src.(sweepGrace); ok {
			out[name] = s.sweepGrace()
		}
	}
	return out
}

// FullBoardListingProviders returns, as a set, the provider names in reg whose adapter is
// registered as structurally proving it lists a board to completion (see fullBoardListing).
// cmd/ingest consults this as the sweep's fourth board-scope qualification gate: a provider
// absent from this set never contributes to the board-scoped close, however its crawl went.
func FullBoardListingProviders(reg map[string]Source) map[string]bool {
	out := make(map[string]bool)
	for name, src := range reg {
		if _, ok := src.(fullBoardListing); ok {
			out[name] = true
		}
	}
	return out
}

// AggregatorProviders returns the sorted provider names in reg that aggregate postings
// from many companies (see aggregator). The cross-source dedup pass uses this to tell an
// aggregator copy (which may be suppressed) from a first-party ATS posting (which wins).
func AggregatorProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(aggregator); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// BoardKeyedProviders returns the sorted provider names in reg whose adapter is addressed by
// a board id — every adapter that is not boardless. A caller that wants to say something about
// ONE board (cmd/harvest-boards validating a candidate) needs this: a boardless adapter serves
// the same catalogue whatever board it is handed, so it would answer for a board that does not
// exist, and confirm every candidate put to it.
func BoardKeyedProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, boardless := src.(boardless); !boardless {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// FilterableProviders returns the sorted provider keys the source facet offers.
func FilterableProviders() []string { return filterableProviders(Taxonomy()) }

// filterableProviders selects the source-facet provider keys from a registry. A
// single-company boardless platform is redundant with the company filter and excluded;
// a board-based platform or a multi-company aggregator stays listed.
func filterableProviders(registry map[string]Source) []string {
	var out []string
	for key, src := range registry {
		if _, isBoardless := src.(boardless); isBoardless {
			if _, isAggregator := src.(aggregator); !isAggregator {
				continue
			}
		}
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

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
		// Rate-paced (pacedPhenomGetter): all ~95 boards share Phenom People's platform
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
		NewBetterteam(c),
		// White-labelled ATS: the board is the full careers host, because the same tenant is
		// served under the vendor's domain and under each reseller's.
		NewHiringThing(c),
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
		NewUKG(c),
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
		NewWantapply(c),
		NewInfoJobs(c),
		NewJobtech(c),
		NewJobnet(c),
		NewJobdanmark(c),
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
		NewEasyCruit(c),
		// Rate-paced: one origin serves every Homerun feed and vacancy page (see homerunRequestInterval).
		NewHomerun(pacedHomerunGetter(c)),
		NewHROffice(c),
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
		// ITviec: Vietnam's IT-only board. One server-rendered listing names the whole
		// catalogue; posting pages carry a schema.org JobPosting. Paced, see itviecRequestInterval.
		NewITviec(pacedHTMLGetter(c, itviecRequestInterval, itviecRequestBurst)),
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
		// XING Jobs: the DACH professional network's job board, enumerated from its gzip job
		// sitemaps and hydrating each new posting from the Apollo cache its page server-renders.
		// See xing.go.
		NewXing(c),
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
	// usajobs, reed, whatjobs and adzuna are the keyed sources: each needs a credential read from
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

// cookieSessionSource builds a registry entry for an adapter whose API is session- or
// CSRF-cookie-authenticated (taleo, aijobs) and so cannot share the shared jar-less
// client. On a real crawl (c != nil) it hands build a fresh cookie-jar client; on the
// transport-free taxonomy path (c == nil, e.g. FilterableProviders) it hands build a true
// nil transport, since T is an interface type parameter and its zero value is nil —
// Provider()/marker assertions never touch it.
func cookieSessionSource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newCookieClient()).(T))
}

// singleUseConnSource builds a registry entry for an adapter whose platform mishandles a
// reused HTTP connection (profession) and so cannot share the pooled client. It mirrors
// cookieSessionSource exactly, including its nil-transport case: on the taxonomy path
// (c == nil) it hands build a true nil, since T is an interface type parameter whose zero
// value is nil and Provider()/marker assertions never touch it.
//
// The failure it answers is worth stating, because it does not present as a transport
// problem: over a pooled connection the platform answers the first request or two and then
// returns a bare EOF, and one of the answers it does give can be a truncated body — a
// sitemap that parses cleanly as a much shorter one. Pacing does not help and a retry
// cannot, since the retry reuses the same pool.
func singleUseConnSource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newSingleUseConnClient()).(T))
}

// browserUASource builds a registry entry for an adapter whose platform refuses any client
// that identifies itself, on a path that platform's own robots.txt allows (4dayweek). It
// mirrors cookieSessionSource exactly, including its nil-transport case for the taxonomy path.
// See browserUserAgent for the measurement, and for the line this does NOT cross.
func browserUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newBrowserUAClient()).(T))
}

// macBrowserUASource is browserUASource for an edge that admits only a macOS browser string
// (Feishu Hire). See macBrowserUserAgent.
func macBrowserUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newMacBrowserUAClient()).(T))
}

// compatibleUASource is browserUASource for an edge that admits the project's own identity once
// it is written in the browser-compatible form (OCC Mundial). See compatibleUserAgent.
func compatibleUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(NewCompatibleUAClient()).(T))
}

// reg indexes sources by provider key. A duplicate key means two adapters claim the
// same platform — a programming error — so it panics rather than silently dropping one.
func reg(sources ...Source) map[string]Source {
	m := make(map[string]Source, len(sources))
	for _, s := range sources {
		if _, dup := m[s.Provider()]; dup {
			panic("sources: duplicate provider " + s.Provider())
		}
		m[s.Provider()] = s
	}
	return m
}
