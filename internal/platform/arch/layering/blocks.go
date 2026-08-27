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
		"arch/layering", "backfillpage", "blobstore", "cache", "config", "database", "db",
		"externalid", "flexjson", "htmltext", "isoweek", "linktoken", "llm", "llmschema", "migrate",
		"modroot", "observability", "outbox", "pgconv", "pgerr", "safehttp", "stringset", "testdb",
		"tokencrypt", "tracerlink", "worker",
	},
	"dict": {
		"classify", "companyname", "industrytag", "lang", "location", "normalize",
		"roletag", "roletype", "skilladjacency", "skillbundle", "skilltag",
		// skillvec/gen is the registry generator — a main package that reads skilltag
		// and writes skillvec's source. It never ships in a binary, but it is a package
		// in the repo, so it needs a block like any other.
		"skillvec", "skillvec/gen", "vocab", "wordmatch",
	},
	"ai": {
		"aiarchetype", "assistant", "autofillagent", "browsertools", "credits", "embed",
		"enrich", "llmkey", "speech",
	},
	"identity": {
		"accountdelete", "accounts", "auth", "auth/apple", "auth/applejobs",
		"auth/mobileauth", "auth/oauth", "auth/recentauth", "userprofile",
	},
	"candidate": {
		"atscheck", "cv", "cvedit", "cvmatch", "cvsection", "experience",
		"hardconstraint", "hardconstraint/credentials", "headshot", "jobmatch",
		"matchanalysis", "pii", "resume", "resumeextract",
	},
	"job": {
		"applydate", "collections", "ghost", "ghostreport", "job", "jobdedup",
		"jobderive", "jobfacts", "jobhash", "jobreality", "jobview", "liveness",
		"outboundurl", "privatejob", "silence", "verdict", "ycdir",
	},
	"application": {
		"appevent", "apptimeline", "calmatch", "calsync", "deliverywindow", "followup",
		"gmailsync", "ical", "inbox", "jobtracking", "mailbox", "mailclassify",
		"mailingest", "maillink", "mailmatch", "mailrecall", "mailtpl", "userjob",
		"viewlog",
	},
	"search": {
		"facetsnapshot", "savedsearch", "search", "searchdrain", "searchintent",
		"similarjobs",
	},
	// catalogstats is here, not in job, because it imports nothing from job at all: it
	// takes cache, db and testdb from platform, sources from here, and reaches the
	// catalogue's row counts through an injected Estimator. Its two headline figures are
	// facts about the adapter registry — how many places the crawler can read, and how many
	// of those are ATS platforms — and both are computed in-process on the READ path too
	// (load.go), so they cannot be handed over in the snapshot.
	"ingest": {
		"adzunadesc", "applyform", "atsboard", "atsdetect", "boardresolve", "catalogstats",
		"contribution", "jdresolve", "linkimport", "linksource", "moderation", "pipeline",
		"screeninganswers", "sources", "submission", "telegram",
	},
	"engage": {
		"broadcast", "community", "companyfeedback", "discordbot", "emailnotify",
		"mailpreview", "notify", "nudge", "onboarding", "pushnotify", "referral",
		"reminder", "report", "subscription", "telegramnotify", "vote",
	},
	"api": {"handler", "ogimage", "ratelimit", "realtime"},
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
