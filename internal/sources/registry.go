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

// SweepGraceWindows returns the post-run sweep window each adapter in reg declares that is wider
// than the default (see sweepGrace). cmd/ingest consults this when computing a provider's cutoff;
// a provider absent from the map is swept on the default window, which is every provider but the
// slice-crawled few.
func SweepGraceWindows(reg map[string]Source) map[string]time.Duration {
	out := make(map[string]time.Duration)
	for name, src := range reg {
		if s, ok := src.(sweepGrace); ok {
			out[name] = s.sweepGrace()
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
		NewOpencats(c),
		NewOdoo(c),
		NewTalentLyft(c),
		NewPinpoint(c),
		NewRippling(c),
		NewBambooHR(c),
		NewWorkday(c),
		NewHuntflow(c),
		NewInhire(c),
		NewHiBob(c),
		NewGem(c),
		NewSuccessFactors(c),
		// Paced: its per-posting detail fan-out fired ~37k requests in 10 minutes and Teamtailor
		// 403'd nearly half the fleet (see teamtailorRequestInterval).
		NewTeamtailor(pacedHTMLGetter(c, teamtailorRequestInterval, teamtailorRequestBurst)),
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
		// Detail hydration is rate-paced (pacedHTMLGetter) to hold the run's request rate
		// under ClinchTalent's per-IP AWS-WAF challenge window; the sitemap fetch is a single
		// request and needs no pacing.
		NewClinch(c, pacedHTMLGetter(c, clinchRequestInterval, clinchRequestBurst)),
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
		// VC talent network, board = portfolio company slug: listed only for the early-stage
		// companies that host their application there instead of on an ATS of their own.
		NewSpeedrun(c),
		// Multi-company aggregators (boardless): one global feed, company per posting.
		NewTecla(c),
		NewTeamex(c),
		NewTopco(c),
		NewGetmatch(c),
		NewGetmanfred(c),
		NewEchoJobs(c),
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
		NewNoDesk(c),
		NewCryptocurrencyJobs(c),
		NewJobspresso(c),
		NewStartupAndVC(c),
		NewFourDayWeek(c),
		NewFunctionalWorks(c),
		NewTheHub(c),
		NewCompleo(c),
		NewInstaffo(c),
		NewGetonbrd(c),
		NewVagas(c),
		NewWantedKR(c),
		NewMyCareersFuture(c),
		NewWorkingNomads(c),
		NewPowerToFly(c),
		NewHimalayas(c),
		NewRemotive(c),
		NewRemotli(c),
		NewLandingJobs(c),
		NewTheMuse(c),
		NewJustJoin(c),
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
		// SolidJobs: Polish job board, multi-company aggregator enumerated by division (board),
		// same keyword-as-board shape as whatjobs.
		NewSolidJobs(c),
		// SEEK: the dominant AU/NZ job board, multi-company aggregator enumerated by ICT
		// subclassification (board) with the market in region, reading its frontend search API and
		// hydrating descriptions from its GraphQL endpoint. Keyless.
		NewSeek(c),
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
	aijobsMaxNewPerRun := aijobsMaxNewPerRunFromEnv()
	registry["aijobs"] = cookieSessionSource[aijobsHTTP](c, func(h aijobsHTTP) Source {
		return NewAijobs(h, aijobsMaxNewPerRun)
	})
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
