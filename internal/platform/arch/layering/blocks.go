package layering

import (
	"maps"
	"slices"
)

// Layers places each block on a layer, counting up from 1 at the bottom. A block may
// import only blocks on a strictly lower layer.
var Layers = map[string]int{
	"platform":    1,
	"dict":        2,
	"ai":          3,
	"identity":    3,
	"candidate":   4,
	"job":         5,
	"application": 6,
	"search":      6,
	"engage":      7,
	"ingest":      7,
	"api":         8,
}

// blocks lists the packages each block owns, named relative to internal/. A package with a
// parent (auth/oauth) is named in full and takes its parent's block.
var blocks = map[string][]string{
	// llm and llmschema are here, not in ai: neither knows anything about the domain, and
	// they import nothing of ours (llm imports only llmschema), like safehttp and blobstore.
	// In ai they would make config -> llm an upward edge.
	"platform": {
		// aigateway is domain-free HTTP transport like llm. Its callers (ai/speech, api/realtime)
		// sit in different blocks, so anywhere else it is an upward edge for one.
		"aigateway",
		"arch", "arch/layering", "blobstore",
		// browser launches a local headless Chrome (a process on our host), unlike browseruse,
		// which is an HTTP client for a hosted browser. Its one caller is api/atsapply.
		"browser",
		// scraplingfetch is the http.RoundTripper to services/scrapling-fetch, the safehttp
		// category. Its caller is ingest/sources.
		"scraplingfetch",
		// browseruse is the HTTP client for the browser-use.com cloud agent API and knows nothing
		// about ATS forms, like llm and aigateway. Its one caller is api/atsapply.
		"browseruse",
		"cache", "config", "database", "db",
		"externalid", "flexjson", "htmltext", "isoweek", "jev", "llm", "llmschema", "migrate",
		"mailmeta", "modroot", "observability", "outbox", "pgconv", "pgerr", "safehttp", "stringset", "testdb",
		"tokencrypt",
		// tracerlink names a CV but holds none of it: link normalisation, an opaque token, a
		// user-agent classifier and a salted visitor hash, like tokencrypt, importing nothing of
		// ours. Section is only a coordinate (header.links / projects) for the renderer. No layer
		// forces the placement; moving it would carry a bot classifier and an IP HMAC into the
		// CV block. Hrefs mirrors cv.LinkHrefs because cv imports this package.
		"tracerlink",
		"worker",
	},
	"dict": {
		"answertopic", "certification", "classify", "companyname", "edulevel", "engagement", "industrytag",
		"lang", "location", "normalize", "perks", "roletype", "skilladjacency", "skillbundle", "skilltag",
		// skillvec/gen is the registry generator, a main package that reads skilltag and writes
		// skillvec's source. It never ships but needs a block like any package.
		"skillvec", "skillvec/gen", "vocab", "wordmatch",
	},
	"ai": {
		"aiarchetype", "assistant", "autofillagent", "browsertools", "embed",
		"enrich", "llmkey",
		// plan holds the user's plan and the per-day allowance of each metered feature. It reads
		// users.pro_until, which argues for identity, but identity shares this layer and
		// assistant and speech could not call it. It reads that column through platform/db.
		"plan",
		"speech",
	},
	"identity": {
		"accountdelete", "accounts", "auth", "auth/apple", "auth/applejobs",
		"apiusage", "auth/mobileauth", "auth/oauth", "auth/oauth2server", "auth/recentauth",
		// billing is here, not in ai: a subscription belongs to the ACCOUNT. plan reads
		// users.pro_until through platform/db and never imports billing, so ai/identity sharing a
		// layer is no constraint. Callers are the webhook handler in api and a cmd binary.
		"billing",
		// promo decides what discount an account is owed (promo code, invited first-month
		// discount, referrer credit). It sits beside billing, not inside it; neither imports the
		// other, and promo reads its own tables through platform/db.
		"promo",
		"userprofile", "username",
	},
	"candidate": {
		"answerbank",
		"atscheck",
		// coverletter drafts from the experience bank against a db.Job the caller supplies; it
		// sits here because job is not importable from this block.
		"coverletter",
		"cv", "cvedit", "cvmatch", "cvsection", "experience",
		// fitanalysis orchestrates matchanalysis (cache, staleness, the credit rule,
		// coalescing); matchanalysis stays the prompt chain and the Analysis type.
		"fitanalysis",
		"hardconstraint", "hardconstraint/credentials", "headshot", "jobmatch",
		"matchanalysis", "perioddate", "resume", "resumeextract",
		// survey holds the candidate's self-reported segmentation answers. It states what a
		// candidate IS, so it sits here; engage (layer 7) may import it, not the reverse.
		"survey",
		// talentnetwork is the public catalogue of opted-in candidates: membership, handle,
		// projection snapshot and filters. It publishes a projection of the candidate's own CV,
		// so it sits here and not in engage.
		"talentnetwork",
	},
	"job": {
		"applydate", "collections",
		// companymerge is the write side of retiring one company slug into another, shared by
		// cmd/merge-companies and companyrelation. Company identity, beside formeremployer.
		"companymerge", "companyrelation",
		// dictgap turns LLM enrichment facts in the catalogue into ranked candidate gaps for the
		// skilltag and classify dictionaries: a fact about recorded facets, not an AI concern.
		"dictgap",
		// formeremployer resolves experience-bank company names to catalogue slugs for the
		// "Hiring again" filter. It reads the bank through platform/db only; the decision is
		// company identity, beside logodomain.
		"formeremployer",
		"ghost", "ghostreport", "job", "jobdedup",
		"jobderive", "jobfacts", "jobhash", "jobreality", "jobview", "liveness",
		// qualityaudit samples the catalogue against the world (is a posting live, is a remote
		// tag honest) and only measures; beside liveness and ghost.
		"qualityaudit",
		// logodomain builds the company-name-to-domain map the logo proxy uses. It is not a
		// dictionary: it reads stored company websites and catalogue spellings, like wikicompany.
		"logodomain",
		"outboundurl", "privatejob",
		// recentfeed polls recent_feed_outbox and groups the batch by
		// jobhash.NormalizedRoleTitle for the homepage feed. Grouping postings by role is a fact
		// about postings, not transport; api imports it to start the poller and serve SSE.
		"recentfeed",
		// reqextract reads a posting's requirements from its description markup into the
		// enrichment contract's shape, so it takes enrich the way jobview does.
		"reqextract",
		// salaryextract reads a labeled salary line from the description, the read-path fill for
		// a posting the model has not priced; a fact the posting states, like reqextract.
		"salaryextract",
		// searchping tells external engines (Google Indexing API, IndexNow) a posting's public
		// URL. It is not in search, which is OUR index (Meilisearch); it reaches no further than
		// platform. Eligibility lives in the SQL beside the query.
		"searchping",
		"silence", "verdict", "ycdir",
		// wikicompany resolves a company name against Wikidata/Wikipedia for the
		// company-info-wikipedia-backfill worker: a fact lookup, like ycdir, not an AI concern.
		"wikicompany",
	},
	"application": {
		"agentview", "appevent", "apptimeline", "autoapply", "autoapplyorchestrate", "calmatch", "calsync",
		"deliverywindow", "followup", "gmailsync", "ical", "inbox", "jobtracking",
		"mailbox", "mailclassify", "mailingest", "maillink", "mailmatch", "mailrecall", "mailtpl",
		"userjob", "viewlog",
	},
	"search": {
		"embedprogress", "facetidx", "facetsnapshot", "savedsearch", "search", "searchdrain", "searchintent",
		"serveprobe", "suggest",
	},
	// catalogstats is here, not in job, because it imports nothing from job: it takes cache,
	// db and testdb from platform, sources from here, and row counts through an injected
	// Estimator. Its headline figures are facts about the adapter registry and are also
	// computed in-process on the READ path (load.go), so the snapshot cannot carry them.
	"ingest": {
		"adzunadesc", "applyform", "atsboard", "atsdetect", "boardcatalog", "boardresolve",
		"catalogstats", "contribution", "directapply", "employer", "ingestsched", "jdresolve", "linkimport",
		"linksource", "moderation", "pipeline", "rederive", "screeninganswers", "sources", "sourcestats",
		"submission", "sweep", "telegram",
	},
	// socialdigest is outbound engagement like broadcast and notify, with a public audience.
	// It reads the catalogue (job) and the view rollup (application).
	// discordlink is outbound engagement (a community-server role) and reaches
	// identity/billing only to ask an account's tier; in identity it would invert that.
	"engage": {
		"broadcast", "buddy", "changehook", "community", "companyfeedback", "companywatch", "discordlink", "emailevents", "emailnotify", "emailprefs",
		// limitnudge is outbound engagement like prowelcome. The trigger (a free-tier ceiling
		// refusal) is written by ai/plan.Store into plan_limit_hits, so it never imports ai/plan.
		"limitnudge",
		// maildaily is the one-optional-letter-a-day claim shared by the four optional senders
		// (onboarding, limitnudge, broadcast, cvlifecycle); cvlifecycle is the CV letters
		// sequence. Both exist to reach the user.
		"maildaily", "cvlifecycle",
		"linkedinauth", "mailpreview", "mentorship", "messaging",
		// mentorship/busysync is named in full, per the auth/oauth convention; its sync worker
		// reaches into application (gmailsync) as mentorship does for calendar-write consent.
		"mentorship/busysync",
		"inboxdigest", "notify", "nudge", "onboarding", "problemreport",
		// ga4mp sends one sign-up event to Google Analytics, imports only the standard library,
		// and is called by the auth handlers; a growth measurement, so engage.
		"ga4mp",
		// processreport holds candidate-reported facts about how a company hires (today: an AI
		// interviewer). It sits beside companyfeedback, not in job: it stores what a PERSON
		// reported, not a derived property.
		"processreport", "pushnotify",
		// prowelcome is outbound engagement (a one-time email), not subscription logic. It reads
		// a tier resolved by plan.TierOf and never imports identity/billing.
		"prowelcome",
		"referral", "reminder", "report",
		// slots is the pure availability engine (rules, expansion, slicing) shared by mentorship
		// and buddy mock interviews; standard library only.
		"slots",
		"socialdigest", "subscription",
		"telegramnotify", "vote", "webhooknotify",
	},
	// atsapply and candidateprofile sit here because both reach ingest (applyform,
	// screeninganswers) and candidate; only api is above ingest. candidateprofile was carved
	// out of handler (extension autofill and cmd/auto-apply share it); atsapply is cmd/auto-apply's
	// counterpart to handler.
	// ojcp renders our catalogue in the Open Job Context Protocol's wire shapes; it reads job,
	// ingest and search together, which only api may do.
	// mcpapp is the ChatGPT app's MCP server. It does not import ojcpmcp: that renders OJCP's
	// schema for agents that branch on error codes, this one renders ours for a model that
	// reads prose. Both adapt the same Reader.
	// apigen is generated from typespec/ by ogen (`make api-contract`) and implemented by
	// handler. It imports nothing of ours.
	"api": {"apigen", "atsapply", "candidateprofile", "handler", "mcpapp", "ogimage", "ojcp", "ojcpmcp", "realtime"},
}

// Assignment is the flattened package → block view the move script drives from.
var Assignment = func() map[string]string {
	m := make(map[string]string)
	for block, pkgs := range blocks {
		for _, p := range pkgs {
			m[p] = block
		}
	}
	return m
}()

// BlockNames returns the block names in sorted order.
func BlockNames() []string { return slices.Sorted(maps.Keys(blocks)) }

// PackagesIn returns the packages a block owns, named relative to internal/.
func PackagesIn(block string) []string { return slices.Clone(blocks[block]) }
