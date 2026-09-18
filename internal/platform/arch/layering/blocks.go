package layering

import (
	"maps"
	"slices"
)

// Layers places each block on a layer, counting up from 1 at the bottom. A block may
// import only blocks on a strictly lower layer.
//
// ai sits low because what remains in it — enrich, embed, llmkey — reaches no further than
// platform and dict; ingest sits high because submission/moderation reach into job and
// linkimport reaches into search and enrich. The order was derived from the graph — see
// docs/superpowers/specs/2026-08-22-internal-module-split-design.md.
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
// parent (auth/oauth, hardconstraint/credentials) is named in full; it takes its parent's
// block, because a sub-package on a different layer from its parent would be a lie about
// where the concept lives.
var blocks = map[string][]string{
	// llm and llmschema are here, not in ai, because neither knows anything about the
	// domain: llm wraps an OpenAI-compatible endpoint and llmschema derives a JSON Schema
	// from a Go type. They import nothing of ours (llm imports only llmschema), which puts
	// them in the same category as safehttp and blobstore. Placing them in ai instead would
	// have made config -> llm an upward edge, and the only ways out were to scatter the
	// config-to-Settings conversion across eight cmd/ entrypoints or to add a package
	// holding one function. The classification was wrong, not the code.
	"platform": {
		// aigateway is here for the same reason as llm: it is the HTTP half of talking to
		// the OpenAI-compatible gateway and knows nothing about the domain. Its two callers
		// sit in different blocks (ai/speech and api/realtime), so anywhere else it would be
		// an upward edge for one of them.
		"aigateway",
		"arch", "arch/layering", "backfillpage", "blobstore",
		// browser is the same "transport, not domain" category one step lower down: how this
		// repository LAUNCHES a local headless Chrome and fetches a URL through one. It is
		// not browseruse below, which is an HTTP client for somebody else's hosted browser —
		// this one is a process on our own host. It is here rather than in either caller
		// because it has two, in different blocks: api/atsapply (fills an application form)
		// and ingest/sources (reads a source gated behind a JavaScript challenge), so in
		// either one it would be an upward edge for the other.
		"browser",
		// firecrawl is the browseruse category, not the browser one, and the distinction is
		// economic rather than technical: it is an HTTP client for a METERED third-party
		// scraping API, where platform/browser launches a free process on our own host. Its
		// caller is ingest/sources, for the two providers that refuse every address we own.
		"firecrawl",
		// browseruse is the HTTP half of talking to the browser-use.com cloud agent API —
		// create/poll/fetch one run — and knows nothing about ATS forms or resolved
		// application plans, the same "transport, not domain" category as llm and
		// aigateway. Its one caller sits in api/atsapply.
		"browseruse",
		"cache", "config", "database", "db",
		"externalid", "flexjson", "htmltext", "isoweek", "linktoken", "llm", "llmschema", "migrate",
		"modroot", "observability", "outbox", "pgconv", "pgerr", "safehttp", "stringset", "testdb",
		"tokencrypt",
		// tracerlink names a CV and is still here, because what it holds is not the CV: link
		// normalisation, an opaque token, a user-agent/device classifier and a salted visitor
		// hash — the same category as linktoken and tokencrypt, and like them it imports
		// nothing of ours. Its only domain contact is Section, a two-value vocabulary saying
		// WHERE in a document a link sat (header.links / projects) so the renderer can put
		// each href back where it found it. That is a coordinate, not a model of a candidate.
		//
		// Unlike aigateway above, no layer forces it: candidate/cv renders with it and
		// api/handler mints and resolves clicks, and both may import candidate. The choice is
		// what the package IS. Moving it would carry a bot classifier and an HMAC of a
		// visitor's IP into the block about a person's CV, where nothing else has a use for
		// either, and it would not remove the one real cost — Hrefs mirrors cv.LinkHrefs
		// because cv imports this package, and the dependency cannot point the other way.
		"tracerlink",
		"worker",
	},
	"dict": {
		"answertopic", "certification", "classify", "companyname", "edulevel", "industrytag",
		"lang", "location", "normalize", "roletype", "skilladjacency", "skillbundle", "skilltag",
		"slugmint",
		// skillvec/gen is the registry generator — a main package that reads skilltag
		// and writes skillvec's source. It never ships in a binary, but it is a package
		// in the repo, so it needs a block like any other.
		"skillvec", "skillvec/gen", "vocab", "wordmatch",
	},
	"ai": {
		"aiarchetype", "assistant", "autofillagent", "browsertools", "embed",
		"enrich", "llmkey",
		// plan holds the plan a user is on and the per-day allowance each metered feature
		// draws from. It reads users.pro_until, which argues for identity — but identity
		// sits on this same layer, so a package there could not be called from assistant
		// or speech, where two of the metered features live. The rule would be satisfied
		// and the code impossible. It reads one column through platform/db rather than
		// using identity's behaviour, which is what makes the placement honest as well as
		// necessary.
		"plan",
		"speech",
	},
	"identity": {
		"accountdelete", "accounts", "auth", "auth/apple", "auth/applejobs",
		"auth/mobileauth", "auth/oauth", "auth/recentauth",
		// billing is here and not in ai, where plan lives, because a subscription is an
		// attribute of the ACCOUNT. The constraint that pushed plan out of this block —
		// ai and identity share a layer, so ai/assistant could not import it — does not
		// reach billing: plan reads users.pro_until through platform/db and never imports
		// this package, and billing's only callers are the webhook handler in api and a
		// binary in cmd.
		"billing",
		// promo decides what discount an account is owed — a promo code it redeemed, the
		// first-month discount an invited account is offered, the credit a referrer earned.
		// It sits beside billing rather than inside it because billing's scope is the
		// subscription itself, and neither imports the other: promo reads its own tables
		// through platform/db and returns a plain value, and the handler and the worker that
		// hold both are in api and cmd, which may import anything.
		"promo",
		"userprofile", "username",
	},
	"candidate": {
		"answerbank",
		"atscheck",
		// coverletter drafts from the experience bank against a vacancy the caller supplies
		// as a db.Job — the letter is about the candidate's evidence, so it sits here and
		// not in job, which this block may not import.
		"coverletter",
		"cv", "cvedit", "cvmatch", "cvsection", "experience",
		// fitanalysis orchestrates matchanalysis (cache, staleness, the credit rule,
		// coalescing); matchanalysis stays the prompt chain and the Analysis type.
		"fitanalysis",
		"hardconstraint", "hardconstraint/credentials", "headshot", "jobmatch",
		"matchanalysis", "pii", "perioddate", "resume", "resumeextract",
		// survey holds the candidate's self-reported segmentation answers (job-search
		// stage, biggest challenge, current income). It sits here rather than in engage,
		// whose digests are its most likely future reader, because it states what a
		// candidate IS — engage is layer 7 and may import this, but not the reverse.
		"survey",
		// talentnetwork is the public catalogue of candidates who opted into being found:
		// membership, the minted handle, the projection snapshot and the filters over it.
		// It sits here and not in engage — which is where the recruiter-facing half will
		// go — because what it publishes is a projection of the candidate's own CV, and
		// candidate is the block that owns what a candidate is made of. engage is layer 7
		// and may import this; the reverse would be the inversion the guard exists to stop.
		"talentnetwork",
	},
	"job": {
		"applydate", "collections",
		// dictgap turns LLM enrichment facts already in the catalogue into ranked
		// candidate gaps for the deterministic dict/skilltag and dict/classify
		// dictionaries — a fact about postings' recorded facets, not an AI/enrichment
		// concern, the same footing reqextract and wikicompany take below.
		"dictgap",
		"ghost", "ghostreport", "job", "jobdedup",
		"jobderive", "jobfacts", "jobhash", "jobreality", "jobview", "liveness",
		// logodomain builds the company-name-to-domain map the logo proxy consults. It
		// is here and not in dict because it is not a dictionary: it reads the stored
		// company website and whatever spellings the catalogue happens to hold, which
		// are facts about companies and postings — the same footing as wikicompany
		// below.
		"logodomain",
		"outboundurl", "privatejob",
		// recentfeed polls recent_feed_outbox and groups the batch by
		// jobhash.NormalizedRoleTitle for the homepage's live "recently added"
		// feed. It lives here rather than in api because grouping recently
		// ingested postings by role is a fact about postings, not a transport
		// concern - api (layer 8) imports it to start the poller and serve the
		// SSE endpoint, the same relationship it has with jobview.
		"recentfeed",
		// reqextract reads a posting's requirements out of its own description markup
		// and returns them in the enrichment contract's shape, so it takes enrich the
		// way jobview does — the block below it, not the model.
		"reqextract",
		// searchping announces a posting's public URL to the external search engines
		// that accept being told (Google's Indexing API, IndexNow). It is here and not
		// in search because search is OUR index — Meilisearch, the drain, saved
		// searches — while this is a fact about a posting's public address and reaches
		// no further than platform. Which postings are eligible lives in the SQL beside
		// the query, so nothing above needs to be imported to decide it.
		"searchping",
		"silence", "verdict", "ycdir",
		// wikicompany resolves a company name against Wikidata/Wikipedia's public APIs
		// for the company-info-wikipedia-backfill worker — a fact-lookup about a
		// company, the same footing as ycdir, not an AI/enrichment concern.
		"wikicompany",
	},
	"application": {
		"appevent", "apptimeline", "autoapply", "autoapplyorchestrate", "calmatch", "calsync",
		"deliverywindow", "followup", "gmailsync", "ical", "inbox", "jobtracking", "joblists",
		"mailbox", "mailclassify", "mailingest", "maillink", "mailmatch", "mailrecall", "mailtpl",
		"userjob", "viewlog",
	},
	"search": {
		"facetsnapshot", "savedsearch", "search", "searchdrain", "searchintent",
		"similarjobs", "suggest",
	},
	// catalogstats is here, not in job, because it imports nothing from job at all: it
	// takes cache, db and testdb from platform, sources from here, and reaches the
	// catalogue's row counts through an injected Estimator. Its two headline figures are
	// facts about the adapter registry — how many places the crawler can read, and how many
	// of those are ATS platforms — and both are computed in-process on the READ path too
	// (load.go), so they cannot be handed over in the snapshot.
	"ingest": {
		"adzunadesc", "applyform", "atsboard", "atsdetect", "boardcatalog", "boardresolve",
		"catalogstats", "contribution", "ingestsched", "jdresolve", "linkimport", "linksource",
		"moderation", "pipeline", "screeninganswers", "sources", "sourcestats", "submission",
		"telegram",
	},
	// socialdigest is here and not in ingest because it is outbound engagement — the
	// same shape as broadcast and notify, differing only in that its audience is the
	// public rather than an account. It reads the catalogue (job) and the view rollup
	// (application), both below it.
	// discordlink is here rather than beside billing for the same reason: what it does is
	// outbound engagement (a role on a community server), and it reaches identity/billing
	// only to ask which tier an account holds. Placing it in identity would invert that —
	// billing would import a community integration — and the guard would say so.
	"engage": {
		"broadcast", "community", "companyfeedback", "discordlink", "emailnotify", "emailprefs",
		"linkedinauth", "mailpreview", "mentorship",
		// mentorship/busysync is named in full, per the auth/oauth convention: it is a
		// sub-package of mentorship and takes its parent's block, but the sync worker
		// reaches into application (gmailsync) the way mentorship itself does for the
		// calendar-write consent, so it is listed rather than left implicit.
		"mentorship/busysync",
		"notify", "nudge", "onboarding",
		// processreport holds candidate-reported facts about how a company hires (today:
		// that it screens with an AI interviewer). It sits beside companyfeedback and not
		// in job, because what it stores is what a PERSON reported, not a property the
		// catalogue derived — the same reason report and vote are here.
		"processreport", "pushnotify",
		// prowelcome is here rather than beside billing for the same reason discordlink
		// is: it is outbound engagement (a one-time email), not subscription logic. It
		// reads a tier resolved elsewhere (plan.TierOf, same as discordlink) and never
		// imports identity/billing at all — the reconciling worker that calls it reads
		// the entitlement columns directly.
		"prowelcome",
		"referral", "reminder", "report", "socialdigest", "subscription",
		"telegramnotify", "vote", "webhooknotify",
	},
	// atsapply and candidateprofile sit here, not lower, because both need to reach
	// ingest (applyform, screeninganswers) as well as candidate (experience, cv,
	// resumeextract) — the only layer above ingest is api. candidateprofile was carved
	// out of handler itself (the extension-autofill path and cmd/auto-apply share one
	// assembler), so it keeps handler's own reach; atsapply is cmd/auto-apply's
	// counterpart to handler — the orchestration layer a cron entrypoint composes
	// ingest+candidate+ai through, the same role handler plays for an HTTP request.
	// ojcp is the projection into the Open Job Context Protocol's wire shapes. It sits in
	// api rather than in job beside jobview because it is a foreign schema's rendering of
	// our catalogue, not a shape the catalogue itself owns — and because it reads job,
	// ingest (the captured apply form) and search together, which only api may do.
	// mcpapp is the OTHER MCP server — the one the ChatGPT app is built on. It is not a
	// variant of ojcpmcp and does not import it: that one renders OJCP's schema for agents
	// that branch on error codes, this one renders ours for a language model that reads
	// prose. Both are adapters over the same Reader the handlers satisfy, which is why they
	// sit in the same block rather than one reaching for the other.
	"api": {"atsapply", "candidateprofile", "handler", "mcpapp", "ogimage", "ojcp", "ojcpmcp", "ratelimit", "realtime"},
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
