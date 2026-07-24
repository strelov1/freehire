package sources

import (
	"os"
	"slices"
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

// FilterableProviders returns the sorted provider keys the source facet offers.
// Passing a nil client is safe: Provider() and the marker assertions never touch the
// transport.
func FilterableProviders() []string { return filterableProviders(All(nil)) }

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

// All assembles the registered adapters into a provider-keyed registry, sharing one
// HTTP client across them. Adding a platform is a new adapter plus one line here.
func All(c HTTPClient) map[string]Source {
	registry := reg(
		NewGreenhouse(c),
		NewLever(c),
		NewAshby(c),
		NewWorkable(c),
		NewWorkableMarketplace(c),
		NewRecruitee(c),
		NewSmartRecruiters(c),
		NewISmartRecruit(c),
		NewGupy(c),
		NewSolides(c),
		NewPersonio(c),
		NewPeopleForce(c),
		NewCatsone(c),
		NewOdoo(c),
		NewTalentLyft(c),
		NewPinpoint(c),
		NewRippling(c),
		NewBambooHR(c),
		NewWorkday(c),
		NewHuntflow(c),
		NewInhire(c),
		NewGem(c),
		NewSuccessFactors(c),
		NewTeamtailor(c),
		NewHurma(c),
		NewICIMS(c),
		// careerspage is rate-paced (pacedCareerPageGetter); the proxied path paces it too.
		NewCareerPage(pacedCareerPageGetter(c)),
		NewCleverstaff(c),
		NewNorthstone(c),
		NewBriefHQ(c),
		NewDjinni(c),
		NewTalentAdore(c),
		NewLoxo(c),
		NewHireology(c),
		NewIsolvedHire(c),
		NewApplicantPro(c),
		NewApploi(c),
		NewPaylocity(c),
		NewJibe(c),
		NewPhenom(c),
		NewAvature(c),
		NewComeet(c),
		NewCornerstone(c),
		NewRadancy(c),
		NewJazzHR(c),
		NewWPYoast(c),
		NewBreezy(c),
		NewJoin(c),
		NewGlobalPayments(c),
		NewRapyd(c),
		NewCareerPlug(c),
		NewPaycom(c),
		NewLuxoft(c),
		NewEPAM(c),
		NewADP(c),
		NewITechArt(c),
		NewVention(c),
		// Detail hydration is rate-paced (pacedClinchGetter) to hold the run's request rate
		// under ClinchTalent's per-IP AWS-WAF challenge window; the sitemap fetch is a single
		// request and needs no pacing.
		NewClinch(c, pacedClinchGetter(c)),
		NewOracle(c),
		NewEightfold(c),
		NewFreshteam(c),
		NewSoftgarden(c),
		NewBetterteam(c),
		NewEarcu(c),
		NewPageUp(c),
		NewNeogov(c),
		NewDeel(c),
		NewVouch(c),
		NewRecruitingSolutions(c),
		NewUKG(c),
		NewSenior(c),
		NewTrakstar(c),
		NewFactorial(c),
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
		// Multi-company aggregators (boardless): one global feed, company per posting.
		NewTecla(c),
		NewTeamex(c),
		NewTopco(c),
		NewGetmatch(c),
		NewGetmanfred(c),
		NewHabrCareer(c),
		NewGeekjob(c),
		NewGetro(c),
		NewJobylon(c),
		NewWorkAtAStartup(c),
		NewJobStash(c),
		NewArbeitnow(c),
		NewRemoteOK(c),
		NewJobicy(c),
		NewWeWorkRemotely(c),
		NewJobspresso(c),
		NewStartupAndVC(c),
		NewFourDayWeek(c),
		NewFunctionalWorks(c),
		NewTheHub(c),
		NewGetonbrd(c),
		NewVagas(c),
		NewWantedKR(c),
		NewMyCareersFuture(c),
		NewWorkingNomads(c),
		NewPowerToFly(c),
		NewHimalayas(c),
		NewRemotive(c),
		NewJustJoin(c),
		NewNoFluffJobs(c),
		NewWantapply(c),
		NewInfoJobs(c),
		NewJobtech(c),
		NewJobnet(c),
		NewJobdanmark(c),
		NewTyomarkkinatori(c),
		NewLikeit(c),
		NewArbeitsagentur(c),
		// International single-company adapters (boardless).
		NewTelegramCareers(c),
		NewUber(c),
		NewAmazon(c),
		NewGoogle(c),
		NewApple(c),
		NewLumenalta(c),
		NewDataArt(c),
		NewOnstrider(c),
		NewAlignerr(c),
		NewTalentHR(c),
		NewMicro1(c),
		NewBairesDev(c),
		// RU federal open-data aggregator: board-based, sharded per region (board = OKATO code).
		// Concurrency-limited: its gov API degrades under the pipeline's board concurrency (500s +
		// read-timeouts), so its in-flight requests are capped to a gentle few.
		NewTrudvsem(limitedTrudvsemGetter(c)),
		// hh.ru: multi-company aggregator, enumerated by professional_role (board), reading the
		// server-rendered search page's embedded state.
		NewHH(c),
		// RU-domestic single-company adapters (boardless, except Yandex which selects
		// host+language by board).
		NewYandex(c),
		// Yandex Crowd: gig/support catalogue at crowd.yandex.ru, one JSON blob per landing page.
		NewYandexCrowd(c),
		NewOzon(c),
		NewAvito(c),
		NewRWB(c),
		NewSber(c),
		NewAlfaBank(c),
		NewLamoda(c),
		NewKuper(c),
		NewAviasales(c),
		NewDodo(c),
		NewDomclick(c),
		NewMtslink(c),
		NewTBank(c),
		NewMTS(c),
		NewVK(c),
		NewTwoGIS(c),
	)
	// USAJobs and Reed are the keyed sources: register each only when its API key is
	// configured, so unconfigured environments (tests, local dev) leave it absent rather than
	// listing a provider that cannot crawl. The keys are secrets, read from the environment.
	if key := os.Getenv("USAJOBS_API_KEY"); key != "" {
		registry["usajobs"] = NewUSAJobs(c, key)
	}
	if key := os.Getenv("REED_API_KEY"); key != "" {
		registry["reed"] = NewReed(c, key)
	}
	// taleo needs a cookie-persisting client (its searchjobs POST requires the session cookie
	// a careersection GET sets), so it cannot use the shared jar-less client. Build a dedicated
	// one for a real crawl; on the transport-free listing path (c == nil) register a bare adapter
	// — Provider()/marker assertions never touch the transport.
	if c == nil {
		registry["taleo"] = NewTaleo(nil)
	} else {
		registry["taleo"] = NewTaleo(newCookieClient())
	}
	// meta is NOT served by the shared client: Meta's edge 400s the default Go TLS+HTTP/2
	// fingerprint, so it needs the shared Chrome-fingerprint transport (fingerprintHTTP, also used
	// by the bayt/gulftalent aggregators). Build it only when there is a real client to serve (the
	// c == nil marker/listing path — e.g. FilterableProviders — must stay transport-free, so meta
	// registers with a nil client there; Provider()/boardless() never touch it). If the
	// deterministic transport build ever fails, meta is left unregistered so config validation
	// fails fast on the "meta" entry, rather than registering a client guaranteed to be rejected by
	// Meta's edge.
	if c == nil {
		registry["meta"] = NewMetaCareers(nil)
		registry["bayt"] = NewBayt(nil)
		registry["gulftalent"] = NewGulfTalent(nil)
	} else if fp, err := newFingerprintHTTP(); err == nil {
		registry["meta"] = NewMetaCareers(fp)
		registry["bayt"] = NewBayt(fp)
		registry["gulftalent"] = NewGulfTalent(fp)
	}
	return registry
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
