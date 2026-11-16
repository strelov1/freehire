package handler

import (
	"cmp"
	"context"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/atsapply"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/mailrecall"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/candidate/survey"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/engage/changehook"
	"github.com/strelov1/freehire/internal/engage/companyfeedback"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/problemreport"
	"github.com/strelov1/freehire/internal/engage/processreport"
	"github.com/strelov1/freehire/internal/engage/referral"
	"github.com/strelov1/freehire/internal/engage/reminder"
	"github.com/strelov1/freehire/internal/engage/report"
	"github.com/strelov1/freehire/internal/engage/webhooknotify"
	"github.com/strelov1/freehire/internal/identity/accountdelete"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/identity/promo"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/ingest/boardresolve"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/ingest/jdresolve"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/companyrelation"
	"github.com/strelov1/freehire/internal/job/privatejob"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Register wires all routes onto the application from cfg. Auth is same-origin only, so
// the cookie rides along with no CORS; the CORS allowlist is not credentialed and permits
// only cross-origin reads of the public endpoints.
func Register(app *fiber.App, cfg Config) {
	queries := db.New(cfg.Pool)
	a := &API{
		pool:         cfg.Pool,
		queries:      queries,
		issuer:       auth.NewIssuer(cfg.JWTSecret, cfg.JWTTTL),
		browserTools: browsertools.New(),
	}
	app.Use(externalCaller(cfg.AuthIntrospectionSecret))
	if cfg.APIUsage != nil {
		app.Use(countAPIUsage(cfg.APIUsage))
	}
	app.Get("/internal/auth/introspect", newAuthIntrospection(queries, a.issuer, cfg.AuthIntrospectionSecret))

	authH := newConfiguredAuthHandlers(cfg, queries, a.issuer)
	// Shared by the jobs surface and the submission queue, so built here.
	moderationSvc := moderation.New(moderation.NewQueriesRepository(queries, cfg.Pool, enrich.Version))
	// One SSRF-guarded client for everything that fetches a user-supplied page: imports,
	// the ingest registry's board coverage reads (so an import and a crawl of one board share
	// transport and rate limits) and the posting-URL resolver.
	ingestClient := sources.NewClient()
	// Turns a link into the posting it is: offline for nearly every ATS, one platform call
	// for shapes that hide the posting behind a second id. Shared by /jobs/find and the link
	// intake, which must agree on what a page is.
	postingURLs := sources.NewPostingURLResolver(ingestClient)
	jobsH := newJobsHandlers(queries, moderationSvc, postingURLs, cfg.Cache)
	statsH := newStatsHandlers(queries, cfg.Cache, cfg.Pool)
	recentFeedH := newRecentFeedHandlers(cfg.RecentJobsFeed)
	// A nil *Sampler in an interface is not a nil interface, so the 503 path needs the
	// conversion skipped, not done.
	var embedProgressSrc embedProgressSource
	if cfg.EmbedProgress != nil {
		embedProgressSrc = cfg.EmbedProgress
	}
	embedProgressH := newEmbedProgressHandlers(embedProgressSrc)
	ogH := newOGHandlers(queries, cfg.Cache)
	votesH := newVoteHandlers(queries, cfg.Pool)
	companyWatchH := newCompanyWatchHandlers(queries)
	communityH := newCommunityHandlers(queries)
	// Feedback reuses communityH's persona minting (via communityPersonas) so a pseudonym
	// matches the one discussion threads show; hence constructed after communityH.
	companyProcessReportH := newCompanyProcessReportHandlers(processreport.New(queries, cfg.Pool))
	companyFeedbackH := newCompanyFeedbackHandlers(companyfeedback.New(queries, cfg.Pool, communityPersonas{svc: communityH.community}))
	// Contributions detect the ATS board from the URL alone (network-free), with a network
	// fallback (boardresolve) that finds an ATS embedded in a company careers page.
	contributionSvc := contribution.New(contribution.NewQueriesRepository(queries), boardresolve.New())
	reportsH := newReportHandlers(queries)
	ghostReportsH := newGhostReportHandlers(queries)
	savedSearchH := newSavedSearchHandlers(queries)
	subscriptionH := newSubscriptionHandlers(queries)
	webhookH := newWebhookHandlers(queries, webhooknotify.NewNotifier())
	changeWebhookH := newChangeWebhookHandlers(queries, changehook.NewSender())
	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	// The candidate's own screening answers (visa, salary, notice period, relocation, ...),
	// distinct from profileSvc's search preferences (see internal/ingest/screeninganswers/AGENTS.md).
	screeningAnswersSvc := screeninganswers.New(screeninganswers.NewQueriesRepository(queries, cfg.Pool))
	screeningAnswersH := newScreeningAnswersHandlers(screeningAnswersSvc)
	// The accumulating bank of open-ended screening-question answers, keyed by the question's
	// topic: a third store, distinct from screeningAnswersSvc's typed facts. Feeds the autofill
	// assembler below.
	answerBank := answerbank.NewStore(answerbank.NewQueriesRepository(queries))
	// The bank's own routes over the same store, so autofill and the review screen never see two copies.
	answerBankH := &answerBankHandlers{bank: answerBank}
	// The onboarding survey: segmentation answers plus the wizard-done marker. Distinct from
	// profileSvc (search filter) and screeningAnswersSvc (what an employer sees): these
	// describe the candidate to us alone.
	surveyH := newSurveyHandlers(survey.New(survey.NewQueriesRepository(queries)), queries)
	// CV storage is nil-safe: a nil Blob (S3 unconfigured) yields a disabled service, so
	// upload/verdict paths degrade to in-request parsing. The pool serialises one user's
	// uploads, so add_cv's first-CV check holds.
	resumeStore := resume.New(cfg.Blob, resume.NewQueriesRepository(queries).WithUploadLocks(cfg.Pool))
	// The headshot rides the same bucket and nil-degradation: no Blob means the photo
	// endpoints 501 while photo CV templates render a placeholder.
	photoStore := headshot.New(cfg.Blob, headshot.NewQueriesRepository(queries))
	// The profile read serves the structured CV beside the profile, so it follows resumeStore.
	profileH := newProfileHandlers(profileSvc, resumeStore, newCandidateProfiler(queries))
	// Personal skill-demand trend: the caller's profile skills against the weekly
	// insights_skill_history snapshots (see me_market_pulse.go).
	marketPulseH := newMarketPulseHandlers(profileSvc, queries)
	// The Talent Network visibility toggle is a singleton on `users`, not part of profileHandlers.
	talentNetworkH := newTalentNetworkHandlers(queries)
	// The public catalogue. One Catalogue per process: it holds the snapshot every request is
	// served from, and one per request would read the whole membership each time. The
	// one-minute TTL lets someone who just joined see themselves listed; leaving takes effect
	// immediately because the card route re-reads the database.
	talentCatalogH := newTalentCatalogHandlers(talentnetwork.NewCatalogue(queries, time.Minute, time.Now), photoStore)
	// One bank for the whole surface. The single value keeps the evidence gate from being
	// attachable later: the CV editor takes it below, the assistant does not hand it over.
	// SetProfileSkills folds every written atom into the owner's search profile.
	bank := experience.NewStore(experience.NewQueriesRepository(queries)).SetProfileSkills(profileSvc)
	experienceH := newExperienceHandlers(bank).withRequireContext(queries)
	// Nil-safe: NewAnalyzer(nil) is a no-op, so the ATS report works without an LLM.
	atsAnalyzer := atscheck.NewAnalyzer(cfg.LLM)
	// The fit analysis uses the same LLM client with a longer per-call timeout: its reasoning
	// model takes tens of seconds per stage. A nil client stays nil (Analyze is a no-op).
	matchAnalyzer := matchanalysis.NewAnalyzer(cfg.LLM.WithTimeout(matchAnalysisLLMTimeout))
	structuredExtractor := resumeextract.NewExtractor(cmp.Or(cfg.ResumeExtractLLM, cfg.LLM).WithTimeout(resumeExtractLLMTimeout))
	plans := plan.NewStore(queries, cfg.Pool, cfg.Plan)
	// Imports fetch a user-supplied page, so they dial through ingestClient. A nil cfg.Search
	// only skips the index push.
	importer := linkimport.New(cfg.Pool, queries, cfg.Search, ingestClient, intakeSources(sources.All(ingestClient)), boardresolve.New())
	contributionsH := newContributionHandlers(contributionSvc, queries, importer, postingURLs)
	// Prefill reuses the SAME importer (its non-writing Resolve half); see submissionHandlers.PrefillSubmission.
	submissionsH := newSubmissionHandlers(queries, cfg.Pool, moderationSvc, importer, postingURLs)
	// jd-tailor-intake reuses the contribution importer (shared SSRF-guarded transport) for its
	// recognized-ATS branch, and internal/job/privatejob for the generic-scrape/pasted-text branch.
	jdResolveH := newJDResolveHandlers(jdresolve.New(queries, importer, privatejob.NewWriter(queries)))
	planH := newPlanHandlers(plans, queries)
	catalogueH := newCatalogueHandlers(queries, cfg.Export)
	billingSvc := billing.New(cfg.Billing, queries)
	// The store provider reads its own environment: it is enabled and credentialed
	// independently of Stripe.
	storeSvc := billing.NewRevenueCat(billing.RevenueCatConfigFromEnv(), queries)
	// Discounts read their own environment too: an invite link works whether or not anything
	// is for sale, and the reward ceiling is operational.
	promoSvc := promo.New(promo.NewQueriesRepository(queries), promo.ConfigFromEnv())
	billingH := newBillingHandlers(billingSvc, storeSvc, promoSvc)
	promoH := newPromoHandlers(promoSvc)
	// Both registration paths attribute a new account to the inviting link; set here because
	// the constructor already takes ten arguments.
	authH.withInvites(promoSvc)
	// Public: a pricing page that needs an account cannot do its job.
	plansH := newPlansHandlers(cfg.Plan, billingSvc)
	fitSvc := fitanalysis.New(queries, meterOrNil(plans), matchAnalyzer)
	matchH := newMatchHandlers(queries, profileSvc, resumeStore, matchAnalyzer, fitSvc, plans)
	// The CV store is shared: the CV surface owns the write path, referrals render from it and
	// autofill reads the base CV's contact header from it.
	cvStore := cv.NewStore(cv.NewQueriesRepository(queries))
	// The conversation store is shared: the assistant runs turns in it and the tailoring
	// bootstrap mints a session in it. Both get the same one.
	assistantStore := assistant.NewStore(queries)
	// The renderer is shared with referrals and enabled only when a typst binary was resolved.
	// Assign only a NON-NIL renderer: a typed nil would defeat the 501 gate.
	var cvRenderer cv.Renderer
	if r := cv.NewTypstRenderer(cfg.TypstBin); r != nil {
		cvRenderer = r
	}
	// Who each model call is spent as: one resolver for every per-user surface. A nil gateway
	// client leaves it inert and every call on the service credential.
	llmKeys := llmkey.NewResolver(queries, cfg.LLMKeys)
	// Same repository as the tracking surface: tailor bootstrap places the vacancy on the Kanban.
	trackingJobs := trackingBoarder{repo: jobtracking.NewQueriesRepository(queries, cfg.Pool)}
	// One store, chain and bank shared by the endpoint and cover_letter_draft, so the button
	// and chat paths cannot write two different letters for one pair.
	letterDeps := coverLetterDeps{
		letters: coverletter.NewStore(coverletter.NewQueriesRepository(queries)),
		chain:   coverletter.NewAnalyzer(cfg.LLM),
		bank:    bank,
	}
	// How a CV is rendered when its owner sends it somewhere (CV download, on-site application).
	cvPDF := newOwnedCVPDF(queries, cvStore, cvRenderer, photoStore, cfg.FrontendOrigin, servedHostsOrDefault(cfg.ServedHosts, cfg.FrontendOrigin))
	cvH := newCVHandlers(cfg.Pool, queries, cvStore, assistantStore, cvRenderer, cvPDF, cfg.TracerLinkSalt, cfg.FrontendOrigin, resumeStore, photoStore, plans, matchH, bankGate{bank: bank}, trackingJobs, letterDeps, !cfg.CVEditAllowBulletTruncation)

	cvH.llm = llmBinding{client: cfg.LLM, keys: llmKeys}
	referralMessageH := newReferralMessageHandlers(queries, bank, resumeStore, fitSvc, letterDeps.chain, llmBinding{client: cfg.LLM, keys: llmKeys}, plans)
	telegramH := newTelegramHandlers(queries, cfg.JWTSecret, cfg.TelegramBotToken, cfg.TelegramBotUsername, cfg.TelegramWebhookSecret, cfg.FrontendOrigin, contributionsH.intake)
	inboxH := newInboxHandlers(queries, cfg.Pool, cfg.GmailConnector, cfg.GmailCipher, cfg.FrontendOrigin, cfg.CookieSecure, cfg.MailboxDomain)
	discordH := newDiscordHandlers(cfg.DiscordLinker, cfg.DiscordClientID, cfg.FrontendOrigin, cfg.CookieSecure)
	// The pull direction is wired only where there is a model to ask; left nil, its endpoint
	// reports the feature off.
	if cfg.LLM != nil {
		// The mailbox factory exists only where a Gmail client and token cipher both do (the
		// cmd/gmail-sync condition); otherwise callers fall back to stored mail.
		inboxH = inboxH.withRecall(
			mailrecall.New(mailrecall.NewDBStore(queries), cfg.LLM),
			llmBinding{client: cfg.LLM, keys: llmKeys},
			newGmailMailboxes(queries, cfg.GmailCipher, cfg.GmailConnector), plans)
	}
	// Account deletion reaches past the FK cascade: cfg.Blob is nil when storage is
	// unconfigured and the revoker is nil when Gmail is, and neither may stop a member from
	// leaving. The gateway credential is the third external system; attached below.
	accountDeletion := accountdelete.New(accountdelete.NewQueriesRepository(queries), cfg.Blob, inboxH.revokeGmailGrant).
		WithAppleGrants(authH.mobileAuth.ReleaseAppleGrantsForDeletion)
	authH.withAccountDeletion(accountDeletion, queries)
	// Assign only when configured: a nil *search.Client in an interface is non-nil and defeats the nil check.
	var jobSearch searcher
	var facets facetCounter
	var companySearch companySearcher
	// Both sitemaps page an index, so they are wired here.
	var sitemapJobs, sitemapCompanies sitemapLister
	var sitemapFreshJobs freshSitemapLister
	if cfg.Search != nil {
		jobSearch = cfg.Search
		facets = cfg.Search
		companySearch = cfg.Search
		sitemapJobs = cfg.Search
		sitemapCompanies = companySitemapIndex{c: cfg.Search}
		sitemapFreshJobs = cfg.Search
	}
	sitemapH := newSitemapHandlers(sitemapJobs, sitemapCompanies, sitemapFreshJobs)
	searchH := newSearchHandlers(jobSearch, facets, queries, cfg.Cache, profileSvc)
	searchH.agentViews = cfg.AgentViews
	// The OJCP read surface shares the search backend and store, so an agent's question and a
	// browser's reach the same catalogue.
	ojcpH := newOJCPHandlers(jobSearch, queries, cfg.FrontendOrigin, atsapply.SubmittableProviders())
	ojcpH.signer = cfg.OJCPSigner
	ojcpH.agentViews = cfg.AgentViews
	// The ChatGPT app's MCP surface: a SECOND MCP server beside OJCP, over the same search
	// backend and store. OJCP renders a foreign schema for agents that branch on error codes;
	// this one renders ours for a model reading prose.
	mcpappH := newMCPAppHandlers(
		jobSearch,
		companySearch,
		queries,
		cfg.FrontendOrigin,
	)
	mcpappH.facets = facets
	mcpappH.cache = cfg.Cache
	mcpappH.agentViews = cfg.AgentViews
	// The completion dictionary. Left nil when search is unconfigured: a nil *suggest.Service
	// in the interface would pass the handler's "not configured" check and then dereference nil.
	suggestH := &suggestHandlers{}
	if cfg.Suggest != nil {
		suggestH.suggest = cfg.Suggest
	}
	// Reads the same saved profile as the assistant, so a profile-seeded search and a
	// profile-aware conversation agree.
	intentH := newIntentHandlers(llmBinding{client: cmp.Or(cfg.SearchIntentLLM, cfg.LLM), keys: llmKeys})
	companiesH := newCompaniesHandlers(queries, companySearch)
	geoH := newGeoHandlers()
	// One reminder service, handed to tracking as its Reminders port and kept by the tracking
	// handlers for the notification-settings endpoints. Save/unsave/apply schedule and cancel
	// inside tracking, so every caller gets the same side effect.
	reminders := reminder.New(reminder.NewQueriesRepository(queries))
	tracking := jobtracking.New(jobtracking.NewQueriesRepository(queries, cfg.Pool), jobtracking.WithReminders(reminders))
	trackingH := newTrackingHandlers(queries, tracking, reminders, jobSearch)
	// hide_applied_companies on /jobs/search reads the same set the feed badges cards by.
	searchH.appliedCompanies = tracking
	// hide_seen on /jobs/search reads the same record the swipe deck excludes by.
	searchH.seen = tracking
	// A closed posting stays in the index until the drain removes it; the page hides it meanwhile.
	searchH.removals = queries
	timelineH := newTimelineHandlers(queries)
	resumeH := newResumeHandlers(resumeStore, structuredExtractor, facets, profileSvc, atsAnalyzer, queries)
	photoH := newPhotoHandlers(photoStore)
	// Same nil-interface trap as jobSearch: assign only when non-nil, or the handler's "no
	// gateway here" check passes into a nil dereference.
	var stt transcriber
	if cfg.Speech != nil {
		stt = cfg.Speech
	}
	speechH := newSpeechHandlers(stt, plans)
	// The in-app agent is a facade over the feature handlers above: its tools call the same
	// services as their endpoints, so a tool result and the API cannot disagree. The tailoring
	// bootstrap mints conversations through the same store. It also takes the browser-tool hub.
	dictionaryH := &dictionaryHandlers{queries: queries}
	assistantH := newAssistantHandlers(
		queries,
		assistantModels{
			// Its own per-call timeout: a turn's rounds are slower than a one-shot extraction, and
			// a call cut short takes the whole run with it. Nil-safe like the fit analysis.
			Agent: cfg.AssistantLLM.WithTimeout(assistantLLMTimeout), Keys: llmKeys,
			MaxSteps: cfg.AssistantMaxSteps, MaxPrompt: cfg.AssistantMaxPrompt,
		},
		assistantStore,
		searchH,
		resumeH,
		trackingH,
		cvH,
		profileH,
		dictionaryH,
		problemreport.New(queries),
		a.browserTools,
		inboxH,
		bank,
		screeningAnswersSvc,
		plans,
		letterDeps,
		billingSvc.PublicPrices,
	)
	// Same nil-interface trap as stt: assign only when cfg.Realtime is non-nil, or "no voice
	// mode" becomes a panic on the first mint.
	if cfg.Realtime != nil {
		assistantH.realtime = cfg.Realtime
	}
	if p := newInngestEventPublisher(cfg.InngestEventAPIURL, cfg.InngestEventKey); p != nil {
		assistantH.events = p
	}
	assistantH.answerBank = answerBank
	assistantH.support.pool = cfg.Pool
	if cfg.SupportLLM != nil {
		supportLLM := cfg.SupportLLM.WithTimeout(assistantLLMTimeout)
		assistantH.support.llm = supportLLM
		assistantH.support.runner = assistant.NewRunner(supportLLM, assistantStore, assistant.RunnerConfig{MaxSteps: supportMaxSteps})
	}
	assistantH.reports = reportsH
	resumeH.llm = llmBinding{client: cfg.LLM, keys: llmKeys}
	matchH.llm = llmBinding{client: cfg.LLM, keys: llmKeys}
	// The autofill planner is one cheap structured call per run, so it keeps the shared
	// client's default timeout. Its contact block comes from the base CV, then the structured
	// CV (see autofillHandlers.autofillProfile).
	autofillH := newAutofillHandlers(cvStore, resumeStore, queries, screeningAnswersSvc, answerBank, a.browserTools, llmBinding{client: cfg.LLM, keys: llmKeys}, cfg.TypesafeAPIKey)
	autofillH.profiles.WithProfileLocation(profileSvc)
	usageH := newUsageHandlers(cfg.LLMKeys, llmKeys)
	accountDeletion.WithGatewayKeys(llmKeys.Revoke)

	// Referral notifications reuse the SES transport and the Telegram bot when linked. Each
	// channel is wrapped only when configured (typed-nil trap, see above); a referrer with no
	// reachable channel still sees the request in-cabinet.
	var referralEmail emailnotify.Sender
	// mailLinks signs the unsubscribe URL every non-essential mail carries; one instance for
	// every API sender keeps footer tokens the same shape.
	mailLinks := emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin)
	// mailPrefs serves the preference routes and answers "has this account turned that group
	// off" for the report notice and referral ping, which have no selection query to gate.
	// One instance, so the footer switch and the sender's check are the same switch.
	mailPrefs := emailprefs.NewService(queries, cfg.JWTSecret)
	// The same client as its CONCRETE type, because mentorship is composed separately below;
	// its own pointer rather than a type assertion on referralEmail (typed-nil trap).
	var mailClient *emailnotify.Client
	// employerH is constructed unconditionally (its routes register either way), but its
	// employer.Service needs a claim mailer that exists only once an SES client does.
	var employerClaimMailer *emailnotify.AuthMailer
	if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		if ec, err := emailnotify.NewClient(context.Background(), cfg.AWSRegion); err != nil {
			log.Printf("referral: email pinger disabled: %v", err)
		} else {
			referralEmail = ec
			mailClient = ec
			// One AuthMailer carries the account mails (verification, password reset) AND the
			// employer-claim code. Without it the accounts service still registers and
			// authenticates (code-backed flows 503), and employer.Service.Claim reports
			// ErrMailUnavailable before reserving a company slug it could not mail a code for.
			employerClaimMailer = emailnotify.NewAuthMailer(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
			authH.accounts.WithCodes(accounts.NewQueriesCodeStore(queries, cfg.Pool), employerClaimMailer)
			// Also tells a reporter what a moderator decided; without it each decision reports
			// that nobody was notified.
			reportsH.report.WithNotifier(report.NewMailNotifier(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin, mailLinks, mailPrefs))
		}
	} else {
		log.Print("accounts: AWS_REGION/NOTIFY_EMAIL_FROM unset — email verification and password reset are unavailable")
	}
	if mailClient != nil && cfg.SupportReplyTo != "" {
		assistantH.support.mail = supportMail{sender: mailClient, from: emailnotify.From("freehire", cfg.NotifyEmailFrom), to: cfg.SupportReplyTo}
	}
	employerSvc, employerH := newEmployerSurfaces(cfg, queries, authH.accounts, employerClaimMailer, mailClient, moderationSvc)
	// Applying on freehire needs a CV renderer and a mail transport; without either the
	// routes answer 501 rather than record an application that is never delivered.
	directApplyH := &directApplyHandlers{}
	var applicationMailer *emailnotify.ApplicationMailer
	if mailClient != nil {
		applicationMailer = emailnotify.NewApplicationMailer(mailClient, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
		// The inbox needs no CV renderer — it serves the CV each application kept — but it
		// does need mail: moving an application may email the candidate.
		employerH.inbox = directapply.NewInbox(
			directapply.NewQueriesRepository(queries),
			employerSvc,
			directApplyStatusMailer{mailer: applicationMailer, origin: cfg.FrontendOrigin},
			directapply.NewApplicationMoves(queries, cfg.Pool),
		)
	}
	if cvRenderer != nil && mailClient != nil {
		directApplyH.svc = directapply.New(
			directapply.NewQueriesRepository(queries),
			employerSvc,
			directApplyCVs{pdf: cvPDF, resumes: resumeStore},
			directApplyMailer{mailer: applicationMailer, origin: cfg.FrontendOrigin},
			directApplyTracker{tracking: tracking},
			answerBank,
			cfg.DirectApplyDailyLimit,
		)
	} else {
		log.Print("directapply: no CV renderer or no mail transport — applications on freehire are unavailable")
	}
	var referralTelegram referral.TelegramSender
	if telegramH.telegramBot != nil {
		referralTelegram = telegramH.telegramBot
	}
	referralPinger := referral.NewChannelPinger(referralEmail, cfg.NotifyEmailFrom, referralTelegram, cfg.FrontendOrigin, mailLinks, mailPrefs)
	referralCabinetURL := strings.TrimRight(cfg.FrontendOrigin, "/") + "/my/referrals/incoming"
	referralSvc := referral.New(referral.NewQueriesRepository(queries), referralPinger, cfg.Blob,
		referral.Config{CabinetURL: referralCabinetURL})
	referralsH := newReferralHandlers(referralSvc, cfg.Blob, cvRenderer, cvStore, photoStore, queries, resumeStore)
	conversationH, buddyH := newBuddySurfaces(cfg, queries, mailClient)
	assistantH.buddies = buddyH

	mentorshipH, mentorSuggestionsH := newMentorshipSurfaces(cfg, queries, mailClient, photoStore, resumeStore, profileSvc, bank)

	a.mountAppRoutes(app, cfg, queries)

	api := app.Group("/api/v1")

	mw := a.newMiddleware(cfg, plans)

	// Before any /api/v1 route: it hands them the rewritten request (signed_in_door.go).
	mountSignedInDoor(app, mw.key)

	// The email preference centre. Its three public routes are unauthenticated on purpose: a
	// person holding one of our mails must be able to turn it off without an account, so the
	// signed token stands in for a session. The two /me routes are the same switches for a
	// cookie-authenticated caller.
	emailPrefsH := newEmailPrefsHandlers(mailPrefs)
	agentPermissionsH := &agentPermissionHandlers{queries: queries}
	userModerationH := newUserModerationHandlers(queries)
	companyRelationH := &companyRelationHandlers{relations: companyrelation.New(cfg.Pool)}
	handlers := contractHandlers{conversationHandlers: conversationH,
		authHandlers:                 authH,
		resumeHandlers:               resumeH,
		matchHandlers:                matchH,
		autofillHandlers:             autofillH,
		intentHandlers:               intentH,
		embedProgressHandlers:        embedProgressH,
		assistantHandlers:            assistantH,
		communityHandlers:            communityH,
		companyFeedbackHandlers:      companyFeedbackH,
		companyRelationHandlers:      companyRelationH,
		reportHandlers:               reportsH,
		emailPrefsHandlers:           emailPrefsH,
		talentCatalogHandlers:        talentCatalogH,
		buddyHandlers:                buddyH,
		companyWatchHandlers:         companyWatchH,
		geoHandlers:                  geoH,
		suggestHandlers:              suggestH,
		agentPermissionHandlers:      agentPermissionsH,
		usageHandlers:                usageH,
		plansHandlers:                plansH,
		dictionaryHandlers:           dictionaryH,
		jdResolveHandlers:            jdResolveH,
		screeningAnswersHandlers:     screeningAnswersH,
		surveyHandlers:               surveyH,
		voteHandlers:                 votesH,
		ghostReportHandlers:          ghostReportsH,
		talentNetworkHandlers:        talentNetworkH,
		answerBankHandlers:           answerBankH,
		changeWebhookHandlers:        changeWebhookH,
		userModerationHandlers:       userModerationH,
		companyProcessReportHandlers: companyProcessReportH,
		contributionHandlers:         contributionsH,
		marketPulseHandlers:          marketPulseH,
		promoHandlers:                promoH,
		mentorSuggestionsHandlers:    mentorSuggestionsH,
		mentorshipHandlers:           mentorshipH,
		savedSearchHandlers:          savedSearchH,
		subscriptionHandlers:         subscriptionH,
		timelineHandlers:             timelineH,
		profileHandlers:              profileH,
		statsHandlers:                statsH,
		submissionHandlers:           submissionsH,
		trackingHandlers:             trackingH,
		experienceHandlers:           experienceH,
		employerHandlers:             employerH,
		referralHandlers:             referralsH,
		billingHandlers:              billingH,
		planHandlers:                 planH,
		catalogueHandlers:            catalogueH,
		directApplyHandlers:          directApplyH,
		jobsHandlers:                 jobsH,
		searchHandlers:               searchH,
		companiesHandlers:            companiesH,
		cvHandlers:                   cvH,
		inboxHandlers:                inboxH}
	contract := newContract(handlers)
	emailPrefsH.register(api, mw, contract)

	a.mountRoutes(app, api, mw, contract, routeHandlers{
		contractHandlers: handlers,
		sitemap:          sitemapH,
		ojcp:             ojcpH,
		mcpapp:           mcpappH,
		recentFeed:       recentFeedH,
		og:               ogH,
		referralMessage:  referralMessageH,
		photo:            photoH,
		speech:           speechH,
		webhook:          webhookH,
		telegram:         telegramH,
		discord:          discordH,
	})
}
