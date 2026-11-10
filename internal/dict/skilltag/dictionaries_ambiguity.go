package skilltag

// ambiguousWords marks the wordAliases keys whose word-pass match is "weak": Parse
// tags it ONLY when the same text carries at least one strong tech token
// (corroboration). Three groups qualify:
//
//   - English-word collisions — a common word that doubles as a tech name
//     (react/swift/spring/rust/ruby/…). Alone it is noise: a cook's "must react to
//     changes", a maintenance post's "inspect for rust".
//   - Broad concepts — the batch-3 generic tags (ai/automation/cloud/api/crm/erp/
//     saas/analytics/seo/networking/…). They appear on non-tech posts too ("AI-powered
//     marketing", a salesperson's "CRM experience", "strong networking skills"), so
//     they are too low-precision to stand alone AND too low-precision to corroborate
//     one another — only a concrete named technology (python, kubernetes, typescript,
//     a phrase, an acronym) is a trustworthy corroborator.
//   - Job-posting boilerplate — words that recur in the prose of NON-technical
//     postings in particular, listed below.
//
// The unambiguous alias forms of the collision canonicals stay strong (reactjs,
// "react native", "spring boot", expressjs, "ruby on rails"), so a genuinely-named
// stack is never gated. Membership is per-alias, not per-canonical; value is unused.
var ambiguousWords = map[string]bool{
	// Polish "konteneryzacja" containerizes freight as well as software ("konteneryzacja
	// ładunków"), so it needs a technical token beside it.
	"konteneryzacja": true, "konteneryzacji": true, "konteneryzacją": true, "konteneryzację": true,
	// English-word collisions
	"react":     true,
	"swift":     true,
	"rust":      true,
	"spring":    true,
	"express":   true,
	"ruby":      true,
	"dart":      true,
	"gin":       true,
	"fiber":     true,
	"ember":     true,
	"elixir":    true,
	"rails":     true,
	"groovy":    true,
	"gorilla":   true,
	"koa":       true,
	"remix":     true,
	"astro":     true,
	"shell":     true,
	"apex":      true,
	"julia":     true,
	"maven":     true,
	"1c":        true,
	"amplitude": true,
	"soap":      true,
	"s3":        true,
	// "clio" is a common first name, the Clio Awards (advertising/marketing), and
	// the Renault Clio, at least as often as it names the legal practice
	// management platform.
	"clio": true,
	// broad concepts (batch 3) — tag only in a concrete tech context
	"ai":         true,
	"automation": true,
	"crm":        true,
	"erp":        true,
	"saas":       true,
	"analytics":  true,
	"api":        true,
	"apis":       true,
	"cloud":      true,
	"networking": true,
	"statistics": true,
	"seo":        true,
	"ecommerce":  true,
	"fintech":    true,
	// job-posting boilerplate — the word belongs to the prose of NON-tech postings
	// (an "agile" supervisor, the drivers who are "the backbone", a "restful" night in
	// a care home, the "assembly" line, the rugby "scrum", tree "sap", a "sentry" post,
	// a bank "vault", the fire-code "firewall", a "rancher", a "postman", "braze"d
	// copper). As strong aliases they did double damage: each tagged its posting on its
	// own, AND lifted the gate off every weak word beside it — one "sap" on a road-
	// maintenance description turned "guard rails" into Ruby on Rails. The real roles
	// name their stack (SAP→ABAP, REST→the "rest api" phrase), so nothing is lost.
	"agile":     true,
	"backbone":  true,
	"restful":   true,
	"assembly":  true,
	"scrum":     true,
	"sap":       true,
	"sentry":    true,
	"vault":     true,
	"firewall":  true,
	"firewalls": true,
	"rancher":   true,
	"postman":   true,
	"braze":     true,
	// design words whose lowercase form is ordinary English, a person's name, an
	// occupation or a kitchen appliance: a retail posting "sketches out ideas", managers
	// called Maya and Lottie, a line cook's "industrial blender", a building's
	// "accessibility ramp", the "typography of the shelf labels", a book imprint hiring
	// an "Illustrator", a store using "Canva" for posters, a CNC operator reading
	// "wireframes". A real design posting names Figma, Photoshop or a CAD product beside
	// them, so corroboration costs nothing and drops the false ones.
	//
	// The last five matter for a second reason: a STRONG match lifts the gate off every
	// weak word in the same text, so leaving them strong would have made the gate on
	// sketch/maya/blender decorative — "Typography of the shelf labels … sketch out
	// ideas" tagged both.
	"sketch":  true,
	"maya":    true,
	"blender": true,
	// "invision" is how half the postings misspell "envision"; "prototyping" is what a
	// line cook does with a menu.
	"invision":      true,
	"prototyping":   true,
	"accessibility": true,
	"typography":    true,
	"illustrator":   true,
	"canva":         true,
	"lottie":        true,
	"wireframes":    true,
	// The VFX suite shares its name with the escapologist, and "a Houdini act" is how
	// an events posting spells it. A real FX posting names Maya, Nuke's neighbours or
	// a render farm beside it. "c4d" is the transplant-pathology biomarker as often as
	// it is the 3D suite, and only the word pass can gate a single token.
	"houdini": true,
	"c4d":     true,
	// batch 4 — products whose bare token is ordinary English, a mythological name or
	// a body part, in exactly the postings this dictionary runs over: a startup that
	// "bootstraps" itself, a laptop that "hibernates", a culture blurb about "unity",
	// a solar "eclipse", "cut me some slack", "zoom in on the detail", "the notion
	// that", the yoga "asana", a "puppet" show, the "bicep" a trainer works, Athena
	// and Aurora and Loki as names, "parquet flooring" on a construction posting, a
	// school "prefect" and welding "flux core".
	// Every one of them is unmistakable next to a named technology and noise without
	// one, which is precisely what the corroboration gate decides.
	"bootstrap": true,
	"hibernate": true,
	"unity":     true,
	"eclipse":   true,
	"slack":     true,
	"zoom":      true,
	"notion":    true,
	"asana":     true,
	"puppet":    true,
	"bicep":     true,
	"athena":    true,
	"aurora":    true,
	"parquet":   true,
	"loki":      true,
	"prefect":   true,
	"flux":      true,
	// batch 5, gated half — see the matching wordAliases block.
	"stripe":    true,
	"fastlane":  true,
	"perforce":  true,
	"arista":    true,
	"aem":       true,
	"hris":      true,
	"cmms":      true,
	"plm":       true,
	"lims":      true,
	"iaas":      true,
	"gpu":       true,
	"bluetooth": true,
	"adas":      true,
}

// properNounWords are the ambiguousWords whose technology is a proper noun — "React",
// "Swift", "Rust", "Spring", "Express" — while the English word they collide with is
// written in lowercase mid-sentence: "ability to react quickly", "a swift turnaround",
// "by spring 2027", "inspect for rust", "express interest". Parse counts such a word
// only when the original text writes it with a leading capital (React, REACT), on top
// of the corroboration every ambiguous word needs. Corroboration alone was not enough:
// a support posting that says "react quickly" and names Zendesk, or a data director's
// "rather than react to decisions" beside "machine learning", has a strong match and
// so kept react.
//
// Measured on a 1% sample of open prod postings (2026-10-29), among those tagged with
// the word, the share whose text never capitalizes it: react 101/1150, express
// 185/348, spring 39/757, swift 32/168, rust 6/177 — and reading them, essentially
// every one was the English word (the lowercase framework spellings react.js, reactjs
// and "react native" are separate strong aliases and unaffected). Membership is
// measured, not assumed: a collision word that is routinely written lowercase as the
// technology (gin, fiber in Go code) does not belong here.
var properNounWords = map[string]bool{
	"react":   true,
	"swift":   true,
	"rust":    true,
	"spring":  true,
	"express": true,
}

// phraseAlias is a punctuated or multi-word term matched against the normalized
// text with whole-term boundaries (the phrase pass). Canonicals are facet-safe
// slugs (cpp, csharp, ci-cd) rather than the raw punctuated form.
