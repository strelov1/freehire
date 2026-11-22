// Package sources holds the modular job-source adapters and the registry that maps
// a platform key to its adapter. Each adapter implements one ATS platform; adding a
// platform is a new file plus one line in All. The kernel they share lives in sources/kit.
package sources

import (
	"maps"
	"slices"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources/ats"
)

// Taxonomy assembles the registry for classification rather than crawling: every adapter this
// binary knows about, with its markers, whatever credentials the environment holds. Callers
// that ask what KIND of source a provider is — the source facet, the status page's provider
// kind, the cross-source dedup pass's aggregator set — use this, so their answers do not vary
// by host. It carries no transport, so nothing reached through it may Fetch; crawlers call All
// with a real client and get the credential-gated registry instead.
var taxonomy = sync.OnceValue(func() map[string]Source { return All(nil) })

// Taxonomy returns a private map copy; callers may change entries without poisoning later reads.
// Adapter values are shared classification-only objects and must not be mutated.
func Taxonomy() map[string]Source { return maps.Clone(taxonomy()) }

var taxonomyAggregators = sync.OnceValue(func() []string {
	return AggregatorProviders(taxonomy())
})

// TaxonomyAggregatorProviders returns the cached classification with a caller-owned slice.
func TaxonomyAggregatorProviders() []string { return slices.Clone(taxonomyAggregators()) }

// All assembles the registered adapters into a provider-keyed registry, sharing one
// HTTP client across them. Adding a platform is a new adapter plus one line here.
// A nil client builds the transport-free taxonomy registry — call Taxonomy for that.
func All(c HTTPClient) map[string]Source {
	// SEEK, JobStreet and JobsDB share the same frontend/GraphQL infrastructure and our
	// crawl egresses from one IP, so their detail requests must compete for ONE token bucket
	// and share one ban state and run budget (seekDetailNetwork). Two independent networks would
	// silently double the safe rate. The proxy lane is attached by ApplyProxyEgress.
	seekNetworkDetail := newSeekDetailNetwork(pacedSeekPoster(c), nil)
	registry := reg(slices.Concat(
		[]Source{
			ats.NewGreenhouse(c),
			NewHRsmart(c),
			ats.NewLever(c),
			ats.NewAshby(c),
			ats.NewCatchTalents(c),
			ats.NewTalentglade(c),
			ats.NewEmploymentHero(c),
			// Rate-paced (pacedWorkableGetter): all boards share one metered widget host, and
			// unpaced the run 429s. See pacer.go.
			NewWorkable(pacedWorkableGetter(c)),
			NewWorkableMarketplace(c),
			ats.NewRecruitee(c),
			// Rate-paced (pacedSmartRecruitersGetter): all boards share one metered API host,
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
			// the whole list with bodies. Run sequentially at a conservative pace (see pacer.go).
			NewFriendWork(limitedFriendWorkGetter(c)),
			// Potok: Russian ATS, board = tenant subdomain; one server-rendered listing page, bodies
			// hydrated from posting pages for new postings only.
			NewPotok(limitedPotokGetter(c)),
			NewInhire(c),
			NewHiBob(c),
			NewGem(c),
			NewSuccessFactors(c),
			NewSelfRecruit(c),
			// Paced: an unpaced per-posting detail fan-out drew Teamtailor 403s across the fleet (see teamtailorRequestInterval).
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
			// Rate-paced (pacedPhenomGetter): all boards share Phenom People's platform
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
			NewJoin(c),
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
			// Paced per board; see njoyn.go.
			NewNjoyn(c),
			NewPaycom(c),
			NewPaycor(c),
			NewLuxoft(c),
			NewEPAM(c),
			// Rate-paced (pacedADPGetter): all boards share one ADP host, and the run's own
			// aggregate volume 429s most of them unpaced. See pacer.go.
			NewADP(pacedADPGetter(c)),
			// ADP MyJobs: ADP's OTHER career-site product, addressed by site slug rather than by
			// Workforce Now's cid+ccId pair, so a company on it is unreachable through NewADP above.
			// Rate-paced on a limiter of its OWN, not the one above — see pacedADPMyJobsGetter for
			// why sharing would be unverified.
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
			// (plus locale path). Unpaced: no limit was observed or documented; detail hydration runs under the shared 8-worker bound.
			NewTalentbrew(c),
			// Recruiter (recruiter.co.kr): Saramin's career-site builder. One listing POST per board
			// plus one detail GET per open posting against a shared API host; boards are small
			// and no limit was observed, so it is unpaced.
			NewRecruiterCoKR(c),
			// Abacus Umantis: board is the careers host (recruitingapp-<n>.umantis.com or a customer's
			// vanity domain). Paced provider-wide: the tenant hosts share one origin that rate-limits
			// per client IP (see umantisRequestInterval). See umantis.go.
			NewUmantis(pacedHTMLGetter(c, umantisRequestInterval, umantisRequestBurst)),
			NewBetterteam(c),
			// White-labelled ATS: the board is the full careers host, because the same tenant is
			// served under the vendor's domain and under each reseller's.
			NewHiringThing(c),
			// Intervieweb (Zucchetti): the board is the careers host. Unpaced: the listing is one
			// landing page plus one POST per further page and
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
			ats.NewAshbyGraphQL(c),
			// VC talent network, board = portfolio company slug: listed only for the early-stage
			// companies that host their application there instead of on an ATS of their own.
			NewSpeedrun(c),
		},
		aggregatorSources(c, seekNetworkDetail),
		remoteBoardSources(c),
		ruCompanySources(c),
	)...)
	registerSpecialTransports(registry, c)
	return registry
}
