package classify

import (
	"strings"

	"github.com/strelov1/freehire/internal/dict/wordmatch"
)

// techTitleTerms is a curated set of confidently technical (software/IT) role
// terms found in job titles, the positive counterpart to nonTechTitleTerms. It
// exists because is_tech's `true` was otherwise set only by a recognized tech
// CATEGORY, so generic software titles that resolve no sub-discipline ("Software
// Engineer", "Web3 Developer", "System Administrator") fell into `unknown`,
// undercounting tech.
//
// Same doctrine as the rest of classify: whole-word match, never guess. The
// governing rule is the "engineer"/"developer" trap: prod titles show bare
// "engineer" is dominated by NON-software roles (mechanical, manufacturing,
// civil, drainage, optical, project) and bare "developer" also names non-tech
// roles (business/real-estate developer). So every term here is ANCHORED to the
// discipline — no bare "engineer"/"developer"/"analyst"/"architect"/"administrator"
// — and a non-software "…Engineer" stays `unknown` rather than being mislabelled
// tech. The anchor is usually the word "software" or a language name; IT support
// below is the one family anchored on its own noun instead, and it carries the
// argument for that beside the terms.
var techTitleTerms = []string{
	// Software engineer forms (never bare "engineer")
	"software engineer", "software development engineer", "devops engineer",
	// "software design engineer" (and the SDET form spelled out) is not adjacent to
	// "software engineer", so it needs its own term: the design-category split made the
	// title category-less, leaving this detector as the only thing that reads it as
	// technical. The "-ing" spelling needs a term of its own too — boundaries mean
	// "engineer" cannot see "engineering", and "Software Design Engineering Manager"
	// would otherwise be unknown, which is prune's ruleUnknown bucket.
	"software design engineer", "software design engineering",
	"site reliability engineer", "platform engineer",
	"backend engineer", "back-end engineer", "frontend engineer", "front-end engineer",
	"fullstack engineer", "full stack engineer", "full-stack engineer",
	"data engineer", "machine learning engineer", "ml engineer", "ai engineer",
	"cloud engineer", "security engineer", "qa engineer", "test engineer",
	"network engineer", "mobile engineer", "web engineer", "firmware engineer",
	"embedded engineer", "infrastructure engineer", "systems software engineer",
	// "Go Engineer"/"Golang Engineer" is the engineer-titled twin of "go developer"/
	// "golang developer" below — the same role, the same unambiguous language anchor.
	"go engineer", "golang engineer",
	// Developer forms (never bare "developer")
	"software developer", "web developer", "backend developer", "back-end developer",
	"frontend developer", "front-end developer", "fullstack developer",
	"full stack developer", "full-stack developer", "mobile developer",
	"app developer", "application developer", "game developer", "salesforce developer",
	"sharepoint developer", "web3 developer", "blockchain developer",
	"smart contract developer", "ios developer", "android developer",
	"python developer", "java developer", "javascript developer", "typescript developer",
	"golang developer", "go developer", ".net developer", "dotnet developer",
	"php developer", "ruby developer", "rails developer", "c# developer",
	"c++ developer", "node developer", "nodejs developer", "node.js developer",
	"react developer", "react native developer", "angular developer", "vue developer",
	"wordpress developer", "drupal developer", "magento developer", "shopify developer",
	"database developer", "etl developer", "bi developer", "power bi developer",
	"rpa developer", "erp developer", "sap developer", "oracle developer", "abap developer",
	// Administration / operations (never bare "administrator")
	"system administrator", "systems administrator", "sysadmin", "network administrator",
	"database administrator", "linux administrator", "windows administrator",
	"it administrator", "devsecops",
	// IT support. These are the one family here whose CATEGORY cannot say what they
	// are: they resolve to `support`, a vocab.NonTechCategories member, so without an
	// entry here the derivation reads them as confidently non-technical and the
	// enrichment gate skips them. That category is right about the function —
	// reactive, ticket-driven — and wrong about the craft: a help desk runs an IT
	// estate.
	//
	// What makes a term safe here is an anchor that names the estate, not the mood of
	// the work. "it support", "desktop support" and "deskside support" are IT by
	// definition; sampled against live titles they are unanimous. Bare "technical
	// support" is NOT and gets no entry: the same sample gives AGV, automotive,
	// controls, logistics and instructional support, so only the analyst form —
	// already the office-IT title in practice — carries the claim. Bare "support" is
	// the whole customer-service population (226k open postings) and stays out too.
	//
	// "it supporter" is the German/Nordic surface form ("1st Level IT Supporter") and
	// needs its own term: the trailing "er" breaks the word boundary, the same trap
	// the "system administrator"/"systems administrator" pair above guards against.
	// "end user" is an IT/product term of art and its titles read the same way
	// ("Analyst, End User Support", "Desktop / End User Support Technician"); spaced
	// and hyphenated are separate terms because a hyphen is a word boundary.
	"service desk", "help desk", "helpdesk", "technical support analyst",
	"it support", "it supporter", "desktop support", "deskside support",
	"end user support", "end-user support",
	// Architects (never bare "architect")
	"software architect", "solutions architect", "cloud architect", "data architect",
	"security architect", "enterprise architect", "technical architect",
	// Data / ML / security specialisms
	"data scientist", "machine learning", "deep learning", "computer vision engineer",
	"nlp engineer", "penetration tester", "pentester", "sdet", "software tester",
	// Generalist software titles that name no sub-discipline, so classify assigns
	// them no category and only this list can lift them out of `unknown`. Each is
	// anchored the same way as the forms above — never a bare "engineer".
	//
	// "Member of Technical Staff" reads as software on the evidence: of 300 sampled
	// prod postings, 294 are software or AI (xAI, Perplexity, Pure Storage, Cockroach
	// Labs, Microsoft) and 6 are semiconductor fab work, which carries its own
	// suffixes ("Process Development", "NAND", "Dry Etch").
	//
	// "Product Engineer" is deliberately ABSENT despite belonging to the same family:
	// the same sample splits 142 software against 64 manufacturing (ABB, Howmet
	// Aerospace, Texas Instruments, Flextronics), so it is not software-anchored and
	// stays unknown rather than dragging plant engineers into tech.
	"member of technical staff", "member of the technical staff", "founding engineer",
	// "AI-native"/"AI-enabled" describe the toolchain the engineer works with, not
	// the discipline, so they claim no category — but the role is still software.
	// Hyphen and space are separate aliases: a hyphen is a word boundary here.
	"ai-native engineer", "ai native engineer",
	// Unambiguous single words
	"programmer", "sre", "devops",

	// --- The forms prod said were missing -------------------------------------
	//
	// Measured 2026-09-23: 2,232,773 open canonical postings carried NO is_tech
	// signal, and 71,314 of them read as software or IT outright. A posting in that
	// state is absent three times over — search.CategoryUnresolved keeps it out of
	// the index, jobSitemapFilter out of every sitemap, and EnqueuePendingJobs' is_tech
	// gate out of enrichment, so nothing downstream can rescue it. The terms below
	// are the four families the 160 commonest lost titles fell into, and every
	// counterexample they raised is a negative case in tech_lost_test.go.
	//
	// Vendor platforms. A platform names the discipline as surely as a language does,
	// which is why these are anchored and bare "developer" still is not.
	"mulesoft developer", "pega developer", "appian developer",
	"power platform developer", "guidewire developer", "zoho developer",
	"netsuite developer", "peoplesoft developer", "sitecore developer",
	"aem developer", "informatica developer", "hadoop developer",
	"unity developer", "laravel developer", "rust developer", "flutter developer",
	"mendix developer",
	// "servicenow developer" is already covered by the existing servicenow terms;
	// the SPACED spelling is not, and prod carries it ("Service Now Developer").
	"service now developer",
	"sql developer", "cobol developer", "api developer", "gis developer",
	//
	// Level-qualified developer. These are admissible for a reason that is easy to
	// miss: the match is a PHRASE on word boundaries, so "senior developer" cannot
	// occur inside "Senior Business Developer" — the two words are not adjacent
	// there. The seniority word is therefore as real an anchor as a language name.
	// Checked against prod: of the twenty commonest open titles carrying one of
	// these phrases, all twenty are software roles, and the corpus's own
	// non-software "…Developer" titles (Business, Job, Product, Project) carry a
	// different qualifier and stay out.
	"senior developer", "lead developer", "junior developer",
	//
	// IT-anchored roles, always TWO words. Bare "it" is the English pronoun once
	// lowercased, so it can never be a term — "Make It Happen Coordinator" would
	// match. These name a role against the IT estate; the craft-not-industry rule
	// that keeps bare "technical support" out is satisfied because the estate IS
	// what the role runs.
	"it officer", "it supervisor", "it assistant", "it associate", "it trainer",
	"it auditor", "it coordinator", "it consultant", "it technician",
	"it operations", "it intern", "it director", "it executive",
	"head of it", "director of it", "it-projektmanager", "it projektmanager",
	// A word between "IT" and the role breaks adjacency, so the qualified spellings
	// the catalogue carries need their own entries — the same trap the spaced
	// "service now developer" above needed one for. These two are what prod actually
	// says: "IT Field Technician" and "Senior IT Internal Auditor".
	"it field technician", "it internal auditor",
	//
	// Surface forms. Word boundaries cannot see past themselves, so a plural and an
	// abbreviation are each their own entry — the same argument "software design
	// engineering" already carries beside its singular.
	//
	// "software engr", never bare "engr": the same corpus carries "Project Engr II"
	// and "Field Service Engr II", which are construction and field service. The
	// anchor is "software", not the abbreviation.
	"software engr", "software eng", "software engineers", "data engineers",
	// "software engineering" for the same boundary reason its "design" sibling above
	// needed one: it reaches "Software Engineering Intern" and "Associate Director,
	// Software Engineering", which "software engineer" cannot see.
	"software engineering", "software development",
	//
	// Spellings the terms already here cannot reach, because a hyphen, a slash, a
	// space or a comma is a word boundary and wordmatch requires adjacency. Each of
	// these has a sibling above that looks like it should already cover it, and does
	// not — which is why they were found by probing the corpus rather than by reading
	// the list.
	"it-administrator", "it-specialist", "it-специалист", "it специалист",
	"it specialist", "dot net developer", "j2ee developer", "react js developer",
	"website developer", "engineer, software",
	//
	// More anchored developer forms the corpus carries in volume.
	"quantitative developer", "quant developer", "automation developer",
	"applications developer", "cloud developer", "data developer",
	"ai developer", "ai agent developer", "crm developer",
	"integration developer", "big data developer", "azure developer",
	"ab initio developer", "mern stack developer",
	//
	// Engineer forms whose anchor is a named technology.
	"kubernetes engineer", "react engineer", "react native engineer",
	//
	// Other anchored software roles.
	"software consultant", "software team lead", "sql dba", "database analyst",
	"cyber intelligence",
	//
	// Third pass, from re-probing the corpus after the two above. The list stops
	// here on purpose: the catalogue holds 1,563,954 distinct unrecognised titles,
	// so the tail is endless and a dictionary chasing it would never converge. What
	// is worth adding is what the corpus shows in VOLUME, and the way to find the
	// next batch is to re-probe, not to keep guessing.
	"it business partner", "it-supporter", "it apprentice", "it lead",
	"cyber sec", "software intern", "software verification",
	"principal engineer software", "algorithm developer",
	"database reliability engineer", "scala developer", "teradata developer",
	"power apps developer", "as400 developer", "cloud consultant",
	"java developers",
	//
	// NOT here, deliberately: the "Systems Engr" abbreviation. "Systems Engineer" is
	// resolved by the CATEGORY dictionary, which carries a blindness rule holding back
	// "Control Systems Engineer", "Power Systems Engineer" and the rest of the
	// industrial family (openspec/specs/it-title-coverage). A term here would reach
	// past that rule and sweep them in, because this detector wins outright over any
	// category. The abbreviation belongs beside its full spelling, not here.
}

// IsTech reports whether a job title states a confidently technical (software/IT)
// role, matching any techTitleTerms term on word boundaries. It never guesses: a
// title it cannot confidently place as technical returns false. It resolves ONLY
// technical roles — a non-software "…Engineer" (mechanical, drainage, …) yields
// false — so it can feed the is_tech derivation as an additional `true` source
// without risking a non-tech job being mislabelled.
func IsTech(title string) bool {
	lower := strings.ToLower(title)
	for _, term := range techTitleTerms {
		if wordmatch.Contains(lower, term, wordmatch.UnicodeBoundary) {
			return true
		}
	}
	return false
}
