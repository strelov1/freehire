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
// roles (business/real-estate developer). So every term here is SOFTWARE-ANCHORED
// — no bare "engineer"/"developer"/"analyst"/"architect"/"administrator" — and a
// non-software "…Engineer" stays `unknown` rather than being mislabelled tech.
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
	// The IT service desk. These are the one family here whose CATEGORY cannot say
	// what they are: they resolve to `support`, a vocab.NonTechCategories member, so
	// without an entry here the derivation reads them as confidently non-technical
	// and the enrichment gate skips them. That category is right about the function
	// — reactive, ticket-driven — and wrong about the craft: a help desk runs an IT
	// estate. The bare nouns stay out. "support" alone is the whole customer-service
	// population (226k open postings), and only "desk" and the IT-anchored analyst
	// form are specific enough to carry the claim.
	"service desk", "help desk", "helpdesk", "technical support analyst",
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
